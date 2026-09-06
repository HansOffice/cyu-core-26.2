package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cyu-core-26.2/internal/runtime/tick"
)

type Server struct {
	configMgr    *ConfigManager
	listener     net.Listener
	running      atomic.Bool
	startTime    time.Time
	players      sync.Map
	entitySeq    atomic.Int32
	onlineCount  atomic.Int32
	totalLogins  atomic.Int64
	totalPackets atomic.Int64
	world        *World
	cmdHandler   *CommandHandler
	tickLoop     *tick.Loop
}

func NewServer(configMgr *ConfigManager) *Server {
	cfg := configMgr.Get()
	s := &Server{
		configMgr: configMgr,
		world:     NewWorld(cfg.SpawnX, cfg.SpawnY, cfg.SpawnZ),
	}
	s.cmdHandler = NewCommandHandler(s)
	s.tickLoop = tick.New(tick.DefaultRate, s.tick)
	return s
}

func (s *Server) Start() error {
	cfg := s.configMgr.Get()
	addr := fmt.Sprintf(":%d", cfg.Port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = l
	s.running.Store(true)
	s.startTime = time.Now()

	go s.acceptLoop()
	go s.keepAliveLoop()
	s.tickLoop.Start()

	return nil
}

func (s *Server) Stop() {
	if !s.running.Swap(false) {
		return
	}
	s.tickLoop.Stop()
	s.BroadcastSystemMessage("&c[CyuCore] 服务端正在关闭...")
	s.players.Range(func(key, value interface{}) bool {
		if session, ok := value.(*PlayerSession); ok {
			session.SendPacket(PlayPktClientBoundDisconnect, buildDisconnect("&c服务端正在关闭 (Server Stopped)"))
			session.Close()
		}
		return true
	})
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *Server) acceptLoop() {
	for s.running.Load() {
		conn, err := s.listener.Accept()
		if err != nil {
			if !s.running.Load() {
				break
			}
			continue
		}
		eid := int(s.entitySeq.Add(1))
		session := NewPlayerSession(s, conn, eid)
		go session.Run()
	}
}

func (s *Server) keepAliveLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for s.running.Load() {
		<-ticker.C
		now := time.Now().UnixMilli()
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, now)
		payload := buf.Bytes()

		s.players.Range(func(key, value interface{}) bool {
			if session, ok := value.(*PlayerSession); ok && session.state == StatePlay {
				session.SendPacket(PlayPktClientBoundKeepAlive, payload)
			}
			return true
		})
	}
}

func (s *Server) tick() {
	age, tod := s.world.AdvanceTime(1)
	if age%tick.DefaultRate == 0 {
		s.BroadcastPacket(PlayPktClientBoundSetTime, buildSetTime(age, tod))
	}
}

func (s *Server) TickMetrics() tick.Snapshot {
	return s.tickLoop.Snapshot()
}

func (s *Server) AddPlayer(session *PlayerSession) {
	newPlayerInfo := buildPlayerInfoAdd(session.uuid, session.username, session.gameMode)
	newPlayerEntity := buildAddPlayerEntity(session.entityID, session.uuid, session.x, session.y, session.z, session.yaw, session.pitch)

	s.players.Range(func(key, value interface{}) bool {
		if other, ok := value.(*PlayerSession); ok && other.state == StatePlay {
			other.SendPacket(PlayPktClientBoundPlayerInfoUpdate, newPlayerInfo)
			other.SendPacket(PlayPktClientBoundAddEntity, newPlayerEntity)

			session.SendPacket(PlayPktClientBoundPlayerInfoUpdate, buildPlayerInfoAdd(other.uuid, other.username, other.gameMode))
			session.SendPacket(PlayPktClientBoundAddEntity, buildAddPlayerEntity(other.entityID, other.uuid, other.x, other.y, other.z, other.yaw, other.pitch))
		}
		return true
	})

	s.players.Store(strings.ToLower(session.username), session)
	s.onlineCount.Add(1)
	s.totalLogins.Add(1)

	session.SendPacket(PlayPktClientBoundCommands, buildCommandsPacket())

	age := s.world.worldAge.Load()
	tod := s.world.timeOfDay.Load()
	session.SendPacket(PlayPktClientBoundSetTime, buildSetTime(age, tod))

	s.world.modifiedBlocks.Range(func(key, value interface{}) bool {
		pos := key.(BlockPos)
		blockID := value.(int)
		session.SendPacket(PlayPktClientBoundBlockUpdate, buildBlockUpdate(pos.X, pos.Y, pos.Z, blockID))
		return true
	})

	logInfo("[+] 玩家 %s (UUID: %s) 成功进入世界 (坐标: %.1f, %.1f, %.1f)",
		session.username, formatUUID(session.uuid), session.x, session.y, session.z)
}

func (s *Server) RemovePlayer(session *PlayerSession) {
	if _, loaded := s.players.LoadAndDelete(strings.ToLower(session.username)); loaded {
		s.onlineCount.Add(-1)
		removeEntityPayload := buildRemoveEntities(session.entityID)
		removeInfoPayload := buildPlayerInfoRemove(session.uuid)

		s.players.Range(func(key, value interface{}) bool {
			if other, ok := value.(*PlayerSession); ok && other.state == StatePlay {
				other.SendPacket(PlayPktClientBoundRemoveEntities, removeEntityPayload)
				other.SendPacket(PlayPktClientBoundPlayerInfoUpdate, removeInfoPayload)
			}
			return true
		})

		logInfo("[-] 玩家 %s 离开了世界", session.username)
		s.BroadcastSystemMessage(fmt.Sprintf("&e玩家 &f%s &e退出了服务器。", session.username))
	}
}

func (s *Server) BroadcastEntityMove(sender *PlayerSession) {
	payload := buildEntityPositionSync(sender.entityID, sender.x, sender.y, sender.z, sender.yaw, sender.pitch, sender.onGround)
	s.players.Range(func(key, value interface{}) bool {
		if other, ok := value.(*PlayerSession); ok && other != sender && other.state == StatePlay {
			other.SendPacket(PlayPktClientBoundEntityPositionSync, payload)
		}
		return true
	})
}

func (s *Server) BroadcastAnimation(sender *PlayerSession, anim byte) {
	payload := buildAnimate(sender.entityID, anim)
	s.players.Range(func(key, value interface{}) bool {
		if other, ok := value.(*PlayerSession); ok && other != sender && other.state == StatePlay {
			other.SendPacket(PlayPktClientBoundAnimate, payload)
		}
		return true
	})
}

func (s *Server) BroadcastBlockUpdate(x, y, z int, blockID int) {
	payload := buildBlockUpdate(x, y, z, blockID)
	s.players.Range(func(key, value interface{}) bool {
		if session, ok := value.(*PlayerSession); ok && session.state == StatePlay {
			session.SendPacket(PlayPktClientBoundBlockUpdate, payload)
		}
		return true
	})
}

func (s *Server) BroadcastPacket(packetID int, payload []byte) {
	s.players.Range(func(key, value interface{}) bool {
		if session, ok := value.(*PlayerSession); ok && session.state == StatePlay {
			session.SendPacket(packetID, payload)
		}
		return true
	})
}

func (s *Server) GetOnlineCount() int {
	return int(s.onlineCount.Load())
}

func (s *Server) HandlePlayerChat(sender *PlayerSession, message string) {
	if strings.HasPrefix(message, "/") {
		s.cmdHandler.Handle(sender, message)
		return
	}
	logInfo("<%s> %s", sender.username, message)
	formatted := fmt.Sprintf("&7<%s&7>&f %s", sender.username, message)
	s.BroadcastSystemMessage(formatted)
}

func (s *Server) BroadcastSystemMessage(text string) {
	payload := buildSystemChatMessage(text)
	s.players.Range(func(key, value interface{}) bool {
		if session, ok := value.(*PlayerSession); ok && session.state == StatePlay {
			session.SendPacket(PlayPktClientBoundSystemChat, payload)
		}
		return true
	})
}

func (s *Server) BroadcastMessage(text string, exclude *PlayerSession) {
	payload := buildSystemChatMessage(text)
	s.players.Range(func(key, value interface{}) bool {
		if session, ok := value.(*PlayerSession); ok && session.state == StatePlay {
			if exclude == nil || session != exclude {
				session.SendPacket(PlayPktClientBoundSystemChat, payload)
			}
		}
		return true
	})
}

func (s *Server) FindPlayer(name string) *PlayerSession {
	if val, ok := s.players.Load(strings.ToLower(name)); ok {
		if session, ok := val.(*PlayerSession); ok {
			return session
		}
	}
	return nil
}

func (s *Server) KickPlayer(name, reason string) bool {
	if session := s.FindPlayer(name); session != nil {
		session.SendPacket(PlayPktClientBoundDisconnect, buildDisconnect(reason))
		session.Close()
		return true
	}
	return false
}

func (s *Server) ListPlayers() []string {
	var list []string
	s.players.Range(func(key, value interface{}) bool {
		if session, ok := value.(*PlayerSession); ok && session.state == StatePlay {
			info := fmt.Sprintf("%s (pos: %.1f, %.1f, %.1f)", session.username, session.x, session.y, session.z)
			list = append(list, info)
		}
		return true
	})
	return list
}

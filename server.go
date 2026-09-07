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

	"cyu-core-26.2/internal/runtime/mailbox"
	"cyu-core-26.2/internal/runtime/tick"
)

type Server struct {
	configMgr      *ConfigManager
	configuration  *vanillaConfiguration
	listener       net.Listener
	running        atomic.Bool
	startTime      time.Time
	players        sync.Map
	entitySeq      atomic.Int32
	onlineCount    atomic.Int32
	totalLogins    atomic.Int64
	totalPackets   atomic.Int64
	world          *World
	cmdHandler     *CommandHandler
	runtimeMailbox *mailbox.Queue
	tickLoop       *tick.Loop
}

func NewServer(configMgr *ConfigManager, configuration *vanillaConfiguration) (*Server, error) {
	if configMgr == nil {
		return nil, fmt.Errorf("server: nil config manager")
	}
	if configuration == nil {
		return nil, fmt.Errorf("server: nil vanilla configuration")
	}

	cfg := configMgr.Get()
	s := &Server{
		configMgr:      configMgr,
		configuration:  configuration,
		world:          NewWorld(cfg.SpawnX, cfg.SpawnY, cfg.SpawnZ),
		runtimeMailbox: mailbox.New(runtimeMailboxCapacity),
	}
	s.cmdHandler = NewCommandHandler(s)
	s.tickLoop = tick.New(tick.DefaultRate, s.tick)
	return s, nil
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
	s.players.Range(func(_, value any) bool {
		if session, ok := value.(*PlayerSession); ok {
			session.SendPacket(PlayPktClientBoundDisconnect, buildDisconnect("&c服务端正在关闭 (Server Stopped)"))
			session.Close()
		}
		return true
	})
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

func (s *Server) acceptLoop() {
	for s.running.Load() {
		conn, err := s.listener.Accept()
		if err != nil {
			if !s.running.Load() {
				return
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
		_ = binary.Write(&buf, binary.BigEndian, now)
		payload := buf.Bytes()

		s.players.Range(func(_, value any) bool {
			if session, ok := value.(*PlayerSession); ok && session.State() == StatePlay {
				session.SendPacket(PlayPktClientBoundKeepAlive, payload)
			}
			return true
		})
	}
}

func (s *Server) tick() {
	s.runtimeMailbox.Drain(maxRuntimeTasksPerTick)

	age, tod := s.world.AdvanceTime(1)
	if age%tick.DefaultRate == 0 {
		s.BroadcastPacket(PlayPktClientBoundSetTime, buildSetTime(age, tod))
	}
}

func (s *Server) TickMetrics() tick.Snapshot {
	return s.tickLoop.Snapshot()
}

// sendConfiguration sends immutable, startup-validated Minecraft 26.2
// Configuration data. Sessions only decide when protocol state permits it.
func (s *Server) sendConfiguration(session *PlayerSession) bool {
	if session == nil || s.configuration == nil {
		return false
	}
	for _, payload := range s.configuration.registryData {
		if !session.SendPacket(ConfigPktClientBoundRegistryData, payload) {
			return false
		}
	}
	if !session.SendPacket(ConfigPktClientBoundUpdateTags, s.configuration.updateTags) {
		return false
	}
	return session.SendPacket(ConfigPktClientBoundFinishConfig, nil)
}

// AddPlayer runs on the tick owner. registered is set before publishing the
// session in the player map so a concurrent Close cannot be lost between the
// state check and registration.
func (s *Server) AddPlayer(session *PlayerSession) {
	if session == nil || session.State() != StatePlay || session.closeOnce.Load() {
		return
	}
	if !session.registered.CompareAndSwap(false, true) {
		return
	}

	key := strings.ToLower(session.username)
	if existing, loaded := s.players.LoadOrStore(key, session); loaded {
		session.registered.Store(false)
		if existing != session {
			session.SendPacket(PlayPktClientBoundDisconnect, buildDisconnect("&c已有同名玩家在线"))
			session.Close()
		}
		return
	}

	// Close may race after registered becomes true but before LoadOrStore.
	// RemovePlayer will queue an idempotent critical cleanup, while this check
	// prevents the new player from becoming visible for a whole tick.
	if session.State() != StatePlay || session.closeOnce.Load() {
		s.players.CompareAndDelete(key, session)
		session.registered.Store(false)
		return
	}

	s.onlineCount.Add(1)
	s.totalLogins.Add(1)

	newPlayerInfo := buildPlayerInfoAdd(session.uuid, session.username, session.gameMode)
	newPlayerEntity := buildAddPlayerEntity(session.entityID, session.uuid, session.x, session.y, session.z, session.yaw, session.pitch)
	s.players.Range(func(_, value any) bool {
		other, ok := value.(*PlayerSession)
		if !ok || other == session || other.State() != StatePlay {
			return true
		}
		other.SendPacket(PlayPktClientBoundPlayerInfoUpdate, newPlayerInfo)
		other.SendPacket(PlayPktClientBoundAddEntity, newPlayerEntity)

		session.SendPacket(PlayPktClientBoundPlayerInfoUpdate, buildPlayerInfoAdd(other.uuid, other.username, other.gameMode))
		session.SendPacket(PlayPktClientBoundAddEntity, buildAddPlayerEntity(other.entityID, other.uuid, other.x, other.y, other.z, other.yaw, other.pitch))
		return true
	})

	session.SendPacket(PlayPktClientBoundCommands, buildCommandsPacket())
	age, tod := s.world.Time()
	session.SendPacket(PlayPktClientBoundSetTime, buildSetTime(age, tod))

	s.world.RangeModifiedBlocks(func(pos BlockPos, blockID int) bool {
		session.SendPacket(PlayPktClientBoundBlockUpdate, buildBlockUpdate(pos.X, pos.Y, pos.Z, blockID))
		return true
	})

	logInfo("[+] 玩家 %s (UUID: %s) 成功进入世界 (坐标: %.1f, %.1f, %.1f)",
		session.username, formatUUID(session.uuid), session.x, session.y, session.z)
}

// RemovePlayer may be called by network/write goroutines. Only sessions that
// were actually published by AddPlayer can enqueue critical lifecycle work.
func (s *Server) RemovePlayer(session *PlayerSession) {
	if s == nil || session == nil || !session.registered.Load() {
		return
	}
	if s.tickLoop != nil && s.tickLoop.Running() {
		if s.postRuntimeCritical(func() { s.removePlayerOwned(session) }) {
			return
		}
	}
	s.removePlayerOwned(session)
}

func (s *Server) removePlayerOwned(session *PlayerSession) {
	if session == nil || !session.registered.Swap(false) {
		return
	}

	key := strings.ToLower(session.username)
	if !s.players.CompareAndDelete(key, session) {
		return
	}
	s.onlineCount.Add(-1)

	removeEntityPayload := buildRemoveEntities(session.entityID)
	removeInfoPayload := buildPlayerInfoRemove(session.uuid)
	s.players.Range(func(_, value any) bool {
		if other, ok := value.(*PlayerSession); ok && other.State() == StatePlay {
			other.SendPacket(PlayPktClientBoundRemoveEntities, removeEntityPayload)
			other.SendPacket(PlayPktClientBoundPlayerInfoUpdate, removeInfoPayload)
		}
		return true
	})

	logInfo("[-] 玩家 %s 离开了世界", session.username)
	s.BroadcastSystemMessage(fmt.Sprintf("&e玩家 &f%s &e退出了服务器。", session.username))
}

// BroadcastEntityMove must run on the runtime owner because it reads the
// sender's authoritative gameplay fields.
func (s *Server) BroadcastEntityMove(sender *PlayerSession) {
	payload := buildEntityPositionSync(sender.entityID, sender.x, sender.y, sender.z, sender.yaw, sender.pitch, sender.onGround)
	s.players.Range(func(_, value any) bool {
		if other, ok := value.(*PlayerSession); ok && other != sender && other.State() == StatePlay {
			other.SendPacket(PlayPktClientBoundEntityPositionSync, payload)
		}
		return true
	})
}

func (s *Server) BroadcastAnimation(sender *PlayerSession, anim byte) {
	payload := buildAnimate(sender.entityID, anim)
	s.players.Range(func(_, value any) bool {
		if other, ok := value.(*PlayerSession); ok && other != sender && other.State() == StatePlay {
			other.SendPacket(PlayPktClientBoundAnimate, payload)
		}
		return true
	})
}

func (s *Server) BroadcastBlockUpdate(x, y, z int, blockID int) {
	payload := buildBlockUpdate(x, y, z, blockID)
	s.players.Range(func(_, value any) bool {
		if session, ok := value.(*PlayerSession); ok && session.State() == StatePlay {
			session.SendPacket(PlayPktClientBoundBlockUpdate, payload)
		}
		return true
	})
}

func (s *Server) BroadcastPacket(packetID int, payload []byte) {
	s.players.Range(func(_, value any) bool {
		if session, ok := value.(*PlayerSession); ok && session.State() == StatePlay {
			session.SendPacket(packetID, payload)
		}
		return true
	})
}

func (s *Server) GetOnlineCount() int {
	return int(s.onlineCount.Load())
}

// HandlePlayerChat runs on the runtime owner. Commands may therefore mutate
// tick-owned player/world state without introducing a second owner.
func (s *Server) HandlePlayerChat(sender *PlayerSession, message string) {
	if strings.HasPrefix(message, "/") {
		s.cmdHandler.Handle(sender, message)
		return
	}
	logInfo("<%s> %s", sender.username, message)
	s.BroadcastSystemMessage(fmt.Sprintf("&7<%s&7>&f %s", sender.username, message))
}

func (s *Server) BroadcastSystemMessage(text string) {
	payload := buildSystemChatMessage(text)
	s.players.Range(func(_, value any) bool {
		if session, ok := value.(*PlayerSession); ok && session.State() == StatePlay {
			session.SendPacket(PlayPktClientBoundSystemChat, payload)
		}
		return true
	})
}

func (s *Server) BroadcastMessage(text string, exclude *PlayerSession) {
	payload := buildSystemChatMessage(text)
	s.players.Range(func(_, value any) bool {
		if session, ok := value.(*PlayerSession); ok && session.State() == StatePlay {
			if exclude == nil || session != exclude {
				session.SendPacket(PlayPktClientBoundSystemChat, payload)
			}
		}
		return true
	})
}

func (s *Server) FindPlayer(name string) *PlayerSession {
	value, ok := s.players.Load(strings.ToLower(name))
	if !ok {
		return nil
	}
	session, ok := value.(*PlayerSession)
	if !ok || session.State() != StatePlay || !session.registered.Load() {
		return nil
	}
	return session
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
	s.players.Range(func(_, value any) bool {
		if session, ok := value.(*PlayerSession); ok && session.State() == StatePlay && session.registered.Load() {
			list = append(list, s.playerListLine(session))
		}
		return true
	})
	return list
}

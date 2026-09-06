package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type SessionState int

const (
	StateHandshake SessionState = 0
	StateStatus    SessionState = 1
	StateLogin     SessionState = 2
	StateConfig    SessionState = 3
	StatePlay      SessionState = 4
	StateClosed    SessionState = 5
)

type PacketOut struct {
	ID      int
	Payload []byte
}

type PlayerSession struct {
	server       *Server
	conn         net.Conn
	state        SessionState
	entityID     int
	username     string
	uuid         [16]byte
	clientProto  int
	gameMode     int
	x, y, z      float64
	yaw, pitch   float32
	onGround     bool
	teleportSeq  int
	sendChan     chan PacketOut
	closeOnce    atomic.Bool
	lastPingTime int64
}

func NewPlayerSession(server *Server, conn net.Conn, entityID int) *PlayerSession {
	return &PlayerSession{
		server:   server,
		conn:     conn,
		state:    StateHandshake,
		entityID: entityID,
		sendChan: make(chan PacketOut, 128),
	}
}

func (s *PlayerSession) Run() {
	go s.writeLoop()
	s.readLoop()
}

func (s *PlayerSession) SendPacket(packetID int, payload []byte) {
	if s.state == StateClosed {
		return
	}
	select {
	case s.sendChan <- PacketOut{ID: packetID, Payload: payload}:
	default:
	}
}

func (s *PlayerSession) SendSystemMessage(text string) {
	s.SendPacket(PlayPktClientBoundSystemChat, buildSystemChatMessage(text))
}

func (s *PlayerSession) Teleport(x, y, z float64, yaw, pitch float32) {
	s.teleportSeq++
	s.x = x
	s.y = y
	s.z = z
	s.yaw = yaw
	s.pitch = pitch
	s.SendPacket(PlayPktClientBoundPlayerPosition, buildPlayerPositionSync(s.teleportSeq, x, y, z, yaw, pitch))
	s.server.BroadcastEntityMove(s)
}

func (s *PlayerSession) writeLoop() {
	for p := range s.sendChan {
		if err := writePacket(s.conn, p.ID, p.Payload); err != nil {
			break
		}
	}
	s.Close()
}

func (s *PlayerSession) Close() {
	if !s.closeOnce.Swap(true) {
		s.state = StateClosed
		s.conn.Close()
		close(s.sendChan)
		if s.username != "" {
			s.server.RemovePlayer(s)
		}
	}
}

func (s *PlayerSession) readLoop() {
	defer s.Close()

	for {
		s.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		packetID, payload, err := readPacket(s.conn)
		if err != nil {
			break
		}

		switch s.state {
		case StateHandshake:
			s.handleHandshake(packetID, payload)
		case StateStatus:
			s.handleStatus(packetID, payload)
		case StateLogin:
			s.handleLogin(packetID, payload)
		case StateConfig:
			s.handleConfig(packetID, payload)
		case StatePlay:
			s.handlePlay(packetID, payload)
		}
	}
}

func (s *PlayerSession) handleHandshake(packetID int, payload []byte) {
	if packetID != 0x00 {
		return
	}
	buf := bytes.NewReader(payload)
	proto, _ := readVarInt(buf)
	host, _ := readString(buf)
	var port uint16
	_ = binary.Read(buf, binary.BigEndian, &port)
	nextState, _ := readVarInt(buf)

	s.clientProto = proto
	logInfo("[握手] 客户端发起连接: 地址=%s, 声明协议版本=%d, 意图状态=%d (Host=%s:%d)",
		s.conn.RemoteAddr().String(), proto, nextState, host, port)

	if nextState == 1 {
		s.state = StateStatus
	} else if nextState == 2 {
		s.state = StateLogin
	}
}

func (s *PlayerSession) handleStatus(packetID int, payload []byte) {
	cfg := s.server.configMgr.Get()
	if packetID == 0x00 {
		online := s.server.GetOnlineCount()
		resp := buildStatusResponse(cfg.VersionName, ProtocolVersion26_2, cfg.MaxPlayers, online, cfg.MotdLine1, cfg.MotdLine2)
		s.SendPacket(0x00, resp)
	} else if packetID == 0x01 {
		s.SendPacket(0x01, payload)
	}
}

func (s *PlayerSession) handleLogin(packetID int, payload []byte) {
	if packetID == 0x00 {
		buf := bytes.NewReader(payload)
		name, err := readString(buf)
		if err != nil || name == "" {
			name = "Player"
		}
		s.username = name
		var clientUUID [16]byte
		if buf.Len() >= 16 {
			_, _ = buf.Read(clientUUID[:])
			s.uuid = clientUUID
		} else {
			s.uuid = makeOfflineUUID(name)
		}

		logInfo("[登录] 玩家 %s (协议: %d) 请求进入, 下发登录确认帧...", s.username, s.clientProto)
		s.SendPacket(0x02, buildLoginSuccess(s.clientProto, s.uuid, s.username))
	} else if packetID == 0x03 {
		s.state = StateConfig
		s.SendPacket(ConfigPktClientBoundKnownPacks, buildKnownPacks("26.2"))
	}
}

func (s *PlayerSession) handleConfig(packetID int, payload []byte) {
	switch packetID {
	case ConfigPktServerBoundClientInfo:
	case ConfigPktServerBoundKnownPacks:
		for _, pkt := range cachedRegistryPackets {
			s.SendPacket(ConfigPktClientBoundRegistryData, pkt)
		}
		s.SendPacket(ConfigPktClientBoundUpdateTags, cachedUpdateTagsPacket)
		s.SendPacket(ConfigPktClientBoundFinishConfig, []byte{})
	case ConfigPktServerBoundFinishConfig:
		s.state = StatePlay
		s.enterPlay()
	case ConfigPktServerBoundKeepAlive:
	case ConfigPktServerBoundPong:
	}
}

func (s *PlayerSession) enterPlay() {
	cfg := s.server.configMgr.Get()
	s.gameMode = cfg.GameMode
	s.x = cfg.SpawnX
	s.y = cfg.SpawnY
	s.z = cfg.SpawnZ

	s.SendPacket(PlayPktClientBoundLogin, buildPlayLogin(s.entityID, cfg.MaxPlayers, s.gameMode, true))
	s.teleportSeq = 1
	s.SendPacket(PlayPktClientBoundPlayerPosition, buildPlayerPositionSync(s.teleportSeq, s.x, s.y, s.z, s.yaw, s.pitch))
	s.SendPacket(PlayPktClientBoundSetCenterChunk, buildCenterChunk(0, 0))

	s.SendPacket(PlayPktClientBoundChunkBatchStart, []byte{})
	for cx := -1; cx <= 1; cx++ {
		for cz := -1; cz <= 1; cz++ {
			s.SendPacket(PlayPktClientBoundLevelChunk, buildChunkPacket(cx, cz, cfg.PlatformBlock))
		}
	}
	buf := bytes.NewBuffer(writeVarInt(9))
	s.SendPacket(PlayPktClientBoundChunkBatchFinished, buf.Bytes())

	s.SendPacket(PlayPktClientBoundPlayerInfoUpdate, buildPlayerInfoAdd(s.uuid, s.username, s.gameMode))
	s.SendPacket(PlayPktClientBoundGameEvent, buildGameEventWaitingChunks())

	s.server.AddPlayer(s)

	welcome := fmt.Sprintf("&b&l[ CyuCore ] &a欢迎 &f%s &a接入 Minecraft 26.2 原生游戏核心！", s.username)
	s.SendSystemMessage(welcome)
	tip := "&7[提示] 输入 &e/help &7查看可用指令，按 Tab 可查看在线玩家列表。"
	s.SendSystemMessage(tip)

	s.server.BroadcastMessage(fmt.Sprintf("&e玩家 &f%s &e加入了服务器！", s.username), s)
}

func (s *PlayerSession) handlePlay(packetID int, payload []byte) {
	switch packetID {
	case PlayPktServerBoundAcceptTeleport:
	case PlayPktServerBoundChunkBatchRecv:
	case PlayPktServerBoundPlayerLoaded:
	case PlayPktServerBoundKeepAlive:
	case PlayPktServerBoundMovePos:
		buf := bytes.NewReader(payload)
		binary.Read(buf, binary.BigEndian, &s.x)
		binary.Read(buf, binary.BigEndian, &s.y)
		binary.Read(buf, binary.BigEndian, &s.z)
		if buf.Len() > 0 {
			var og byte
			binary.Read(buf, binary.BigEndian, &og)
			s.onGround = og == 1
		}
		s.checkVoidFall()
		s.server.BroadcastEntityMove(s)
	case PlayPktServerBoundMovePosRot:
		buf := bytes.NewReader(payload)
		binary.Read(buf, binary.BigEndian, &s.x)
		binary.Read(buf, binary.BigEndian, &s.y)
		binary.Read(buf, binary.BigEndian, &s.z)
		binary.Read(buf, binary.BigEndian, &s.yaw)
		binary.Read(buf, binary.BigEndian, &s.pitch)
		if buf.Len() > 0 {
			var og byte
			binary.Read(buf, binary.BigEndian, &og)
			s.onGround = og == 1
		}
		s.checkVoidFall()
		s.server.BroadcastEntityMove(s)
	case PlayPktServerBoundMoveRot:
		buf := bytes.NewReader(payload)
		binary.Read(buf, binary.BigEndian, &s.yaw)
		binary.Read(buf, binary.BigEndian, &s.pitch)
		if buf.Len() > 0 {
			var og byte
			binary.Read(buf, binary.BigEndian, &og)
			s.onGround = og == 1
		}
		s.server.BroadcastEntityMove(s)
	case PlayPktServerBoundMoveStatus:
		buf := bytes.NewReader(payload)
		if buf.Len() > 0 {
			var og byte
			binary.Read(buf, binary.BigEndian, &og)
			s.onGround = og == 1
		}
	case PlayPktServerBoundChat:
		buf := bytes.NewReader(payload)
		msg, err := readString(buf)
		if err == nil && msg != "" {
			s.server.HandlePlayerChat(s, msg)
		}
	case PlayPktServerBoundChatCommand:
		buf := bytes.NewReader(payload)
		cmd, err := readString(buf)
		if err == nil && cmd != "" {
			s.server.HandlePlayerChat(s, "/"+cmd)
		}
	case PlayPktServerBoundSwing:
		s.server.BroadcastAnimation(s, 0)
	case PlayPktServerBoundPlayerAction:
		s.handlePlayerAction(payload)
	case PlayPktServerBoundUseItemOn:
		s.handleUseItemOn(payload)
	}
}

func (s *PlayerSession) checkVoidFall() {
	if s.y < -10.0 {
		w := s.server.world
		s.Teleport(w.spawnX, w.spawnY, w.spawnZ, s.yaw, s.pitch)
		s.SendSystemMessage("&e[保护] 你已坠入虚空，已自动将你拉回出生点平台！")
	}
}

func (s *PlayerSession) handlePlayerAction(payload []byte) {
	buf := bytes.NewReader(payload)
	status, err1 := readVarInt(buf)
	var posVal uint64
	err2 := binary.Read(buf, binary.BigEndian, &posVal)
	var face byte
	_ = binary.Read(buf, binary.BigEndian, &face)
	seq, _ := readVarInt(buf)

	if err1 != nil || err2 != nil {
		return
	}

	if status == 0 || status == 2 {
		x, y, z := unpackPosition(posVal)
		s.server.world.SetBlock(x, y, z, BlockAir)
		s.server.BroadcastBlockUpdate(x, y, z, BlockAir)
		if seq >= 0 {
			s.SendPacket(PlayPktClientBoundBlockChangedAck, buildBlockChangedAck(seq))
		}
	}
}

func (s *PlayerSession) handleUseItemOn(payload []byte) {
	buf := bytes.NewReader(payload)
	_, _ = readVarInt(buf)
	var posVal uint64
	if err := binary.Read(buf, binary.BigEndian, &posVal); err != nil {
		return
	}
	face, _ := readVarInt(buf)
	var cx, cy, cz float32
	binary.Read(buf, binary.BigEndian, &cx)
	binary.Read(buf, binary.BigEndian, &cy)
	binary.Read(buf, binary.BigEndian, &cz)
	var inside byte
	binary.Read(buf, binary.BigEndian, &inside)
	if buf.Len() > 0 {
		var wb byte
		binary.Read(buf, binary.BigEndian, &wb)
	}
	seq, _ := readVarInt(buf)

	bx, by, bz := unpackPosition(posVal)
	tx, ty, tz := bx, by, bz
	switch face {
	case 0:
		ty--
	case 1:
		ty++
	case 2:
		tz--
	case 3:
		tz++
	case 4:
		tx--
	case 5:
		tx++
	}

	blockID := BlockStoneBricks
	s.server.world.SetBlock(tx, ty, tz, blockID)
	s.server.BroadcastBlockUpdate(tx, ty, tz, blockID)
	if seq >= 0 {
		s.SendPacket(PlayPktClientBoundBlockChangedAck, buildBlockChangedAck(seq))
	}
}

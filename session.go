package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type SessionState int32

const (
	StateHandshake   SessionState = 0
	StateStatus      SessionState = 1
	StateLogin       SessionState = 2
	StateConfig      SessionState = 3
	StatePlay        SessionState = 4
	StateClosed      SessionState = 5
	StatePlayPending SessionState = 6
)

const outboundQueueCapacity = 128

type PacketOut struct {
	ID      int
	Payload []byte
}

type PlayerSession struct {
	server            *Server
	conn              net.Conn
	state             atomic.Int32
	entityID          int
	username          string
	uuid              [16]byte
	clientProto       int
	configurationSent bool
	gameMode          int
	x, y, z           float64
	yaw, pitch        float32
	onGround          bool
	teleportSeq       int
	playerSnapshot    atomic.Pointer[playerSnapshot]
	sendChan          chan PacketOut
	done              chan struct{}
	closeOnce         atomic.Bool
	lastPingTime      int64
}

func NewPlayerSession(server *Server, conn net.Conn, entityID int) *PlayerSession {
	s := &PlayerSession{
		server:   server,
		conn:     conn,
		entityID: entityID,
		sendChan: make(chan PacketOut, outboundQueueCapacity),
		done:     make(chan struct{}),
	}
	s.setState(StateHandshake)
	return s
}

func (s *PlayerSession) State() SessionState {
	return SessionState(s.state.Load())
}

func (s *PlayerSession) setState(state SessionState) {
	s.state.Store(int32(state))
}

func (s *PlayerSession) Run() {
	go s.writeLoop()
	s.readLoop()
}

func (s *PlayerSession) SendPacket(packetID int, payload []byte) bool {
	if s.State() == StateClosed {
		return false
	}

	packet := PacketOut{ID: packetID, Payload: payload}
	select {
	case <-s.done:
		return false
	case s.sendChan <- packet:
		return true
	default:
		logWarn("[network] outbound queue overflow for %s; disconnecting slow client", s.conn.RemoteAddr())
		s.Close()
		return false
	}
}

func (s *PlayerSession) SendSystemMessage(text string) {
	s.SendPacket(PlayPktClientBoundSystemChat, buildSystemChatMessage(text))
}

func (s *PlayerSession) writeLoop() {
	for {
		select {
		case <-s.done:
			return
		case p := <-s.sendChan:
			if err := writePacket(s.conn, p.ID, p.Payload); err != nil {
				s.Close()
				return
			}
		}
	}
}

func (s *PlayerSession) Close() {
	if !s.closeOnce.Swap(true) {
		s.setState(StateClosed)
		close(s.done)
		s.conn.Close()
		if s.username != "" && s.server != nil {
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

		switch s.State() {
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
		case StatePlayPending:
			// The tick owner has accepted FinishConfiguration but has not yet
			// published the initialized Play state. No serverbound Play packet
			// is valid until the first clientbound Play packet is emitted.
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
		s.setState(StateStatus)
	} else if nextState == 2 {
		s.setState(StateLogin)
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
		s.setState(StateConfig)
		s.SendPacket(ConfigPktClientBoundKnownPacks, buildKnownPacks("26.2"))
	}
}

func (s *PlayerSession) handleConfig(packetID int, payload []byte) {
	switch packetID {
	case ConfigPktServerBoundClientInfo:
	case ConfigPktServerBoundKnownPacks:
		if s.configurationSent {
			logWarn("[protocol] duplicate KnownPacks from %s during Configuration", s.conn.RemoteAddr())
			s.Close()
			return
		}
		s.configurationSent = true
		if s.server == nil || !s.server.sendConfiguration(s) {
			s.Close()
		}
	case ConfigPktServerBoundFinishConfig:
		if !s.configurationSent {
			logWarn("[protocol] premature FinishConfiguration from %s", s.conn.RemoteAddr())
			s.Close()
			return
		}
		if s.server == nil {
			s.Close()
			return
		}
		s.setState(StatePlayPending)
		if !s.server.postRuntime(func() {
			if s.State() != StatePlayPending {
				return
			}
			s.enterPlay()
		}) {
			logWarn("[runtime] unable to enqueue Play initialization for %s", s.conn.RemoteAddr())
			s.Close()
		}
	case ConfigPktServerBoundKeepAlive:
	case ConfigPktServerBoundPong:
	}
}

// enterPlay runs on the tick owner. From this point onward the gameplay fields
// on PlayerSession are authoritative tick-owned state.
func (s *PlayerSession) enterPlay() {
	if s.State() == StateClosed {
		return
	}

	cfg := s.server.configMgr.Get()
	s.gameMode = cfg.GameMode
	s.x = cfg.SpawnX
	s.y = cfg.SpawnY
	s.z = cfg.SpawnZ
	s.yaw = 0
	s.pitch = 0
	s.onGround = false
	s.teleportSeq = 1
	s.publishPlayerSnapshot()

	// Publish Play only after all gameplay fields are initialized. Any incoming
	// Play packet can then only enqueue work for a later tick.
	s.setState(StatePlay)

	s.SendPacket(PlayPktClientBoundLogin, buildPlayLogin(s.entityID, cfg.MaxPlayers, s.gameMode, true))
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
		var move playerMove
		move.hasPosition = true
		if binary.Read(buf, binary.BigEndian, &move.x) != nil ||
			binary.Read(buf, binary.BigEndian, &move.y) != nil ||
			binary.Read(buf, binary.BigEndian, &move.z) != nil {
			return
		}
		var onGround byte
		if binary.Read(buf, binary.BigEndian, &onGround) != nil {
			return
		}
		move.onGround = onGround != 0
		s.server.postPlayerRuntime(s, func() { s.server.applyPlayerMove(s, move) })
	case PlayPktServerBoundMovePosRot:
		buf := bytes.NewReader(payload)
		var move playerMove
		move.hasPosition = true
		move.hasRotation = true
		if binary.Read(buf, binary.BigEndian, &move.x) != nil ||
			binary.Read(buf, binary.BigEndian, &move.y) != nil ||
			binary.Read(buf, binary.BigEndian, &move.z) != nil ||
			binary.Read(buf, binary.BigEndian, &move.yaw) != nil ||
			binary.Read(buf, binary.BigEndian, &move.pitch) != nil {
			return
		}
		var onGround byte
		if binary.Read(buf, binary.BigEndian, &onGround) != nil {
			return
		}
		move.onGround = onGround != 0
		s.server.postPlayerRuntime(s, func() { s.server.applyPlayerMove(s, move) })
	case PlayPktServerBoundMoveRot:
		buf := bytes.NewReader(payload)
		var move playerMove
		move.hasRotation = true
		if binary.Read(buf, binary.BigEndian, &move.yaw) != nil ||
			binary.Read(buf, binary.BigEndian, &move.pitch) != nil {
			return
		}
		var onGround byte
		if binary.Read(buf, binary.BigEndian, &onGround) != nil {
			return
		}
		move.onGround = onGround != 0
		s.server.postPlayerRuntime(s, func() { s.server.applyPlayerMove(s, move) })
	case PlayPktServerBoundMoveStatus:
		buf := bytes.NewReader(payload)
		var onGround byte
		if binary.Read(buf, binary.BigEndian, &onGround) != nil {
			return
		}
		move := playerMove{onGround: onGround != 0}
		s.server.postPlayerRuntime(s, func() { s.server.applyPlayerMove(s, move) })
	case PlayPktServerBoundChat:
		buf := bytes.NewReader(payload)
		msg, err := readString(buf)
		if err == nil && msg != "" {
			s.server.postPlayerRuntime(s, func() { s.server.HandlePlayerChat(s, msg) })
		}
	case PlayPktServerBoundChatCommand:
		buf := bytes.NewReader(payload)
		cmd, err := readString(buf)
		if err == nil && cmd != "" {
			message := "/" + cmd
			s.server.postPlayerRuntime(s, func() { s.server.HandlePlayerChat(s, message) })
		}
	case PlayPktServerBoundSwing:
		s.server.postPlayerRuntime(s, func() { s.server.BroadcastAnimation(s, 0) })
	case PlayPktServerBoundPlayerAction:
		s.handlePlayerAction(payload)
	case PlayPktServerBoundUseItemOn:
		s.handleUseItemOn(payload)
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

	if err1 != nil || err2 != nil || (status != 0 && status != 2) {
		return
	}

	x, y, z := unpackPosition(posVal)
	s.server.postPlayerRuntime(s, func() {
		s.server.world.SetBlock(x, y, z, BlockAir)
		s.server.BroadcastBlockUpdate(x, y, z, BlockAir)
		if seq >= 0 {
			s.SendPacket(PlayPktClientBoundBlockChangedAck, buildBlockChangedAck(seq))
		}
	})
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
	if binary.Read(buf, binary.BigEndian, &cx) != nil ||
		binary.Read(buf, binary.BigEndian, &cy) != nil ||
		binary.Read(buf, binary.BigEndian, &cz) != nil {
		return
	}
	var inside byte
	if binary.Read(buf, binary.BigEndian, &inside) != nil {
		return
	}
	if buf.Len() > 0 {
		var worldBorderHit byte
		if binary.Read(buf, binary.BigEndian, &worldBorderHit) != nil {
			return
		}
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
	default:
		return
	}

	s.server.postPlayerRuntime(s, func() {
		blockID := BlockStoneBricks
		s.server.world.SetBlock(tx, ty, tz, blockID)
		s.server.BroadcastBlockUpdate(tx, ty, tz, blockID)
		if seq >= 0 {
			s.SendPacket(PlayPktClientBoundBlockChangedAck, buildBlockChangedAck(seq))
		}
	})
}

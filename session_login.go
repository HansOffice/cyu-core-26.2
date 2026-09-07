package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func (s *PlayerSession) handleHandshake(packetID int, payload []byte) {
	if packetID != 0x00 {
		return
	}

	buf := bytes.NewReader(payload)
	proto, err := readVarInt(buf)
	if err != nil {
		return
	}
	host, err := readString(buf)
	if err != nil {
		return
	}
	var port uint16
	if err := binary.Read(buf, binary.BigEndian, &port); err != nil {
		return
	}
	nextState, err := readVarInt(buf)
	if err != nil {
		return
	}

	s.clientProto = proto
	logInfo("[握手] 客户端发起连接: 地址=%s, 声明协议版本=%d, 意图状态=%d (Host=%s:%d)",
		s.remoteAddress(), proto, nextState, host, port)

	switch nextState {
	case 1:
		s.transitionState(StateHandshake, StateStatus)
	case 2:
		s.transitionState(StateHandshake, StateLogin)
	}
}

func (s *PlayerSession) handleStatus(packetID int, payload []byte) {
	cfg := s.server.configMgr.Get()
	switch packetID {
	case 0x00:
		online := s.server.GetOnlineCount()
		resp := buildStatusResponse(cfg.VersionName, ProtocolVersion26_2, cfg.MaxPlayers, online, cfg.MotdLine1, cfg.MotdLine2)
		s.SendPacket(0x00, resp)
	case 0x01:
		s.SendPacket(0x01, payload)
	}
}

func (s *PlayerSession) handleLogin(packetID int, payload []byte) {
	switch packetID {
	case 0x00:
		buf := bytes.NewReader(payload)
		name, err := readString(buf)
		if err != nil || name == "" {
			name = "Player"
		}
		s.username = name

		var clientUUID [16]byte
		if buf.Len() >= len(clientUUID) {
			_, _ = buf.Read(clientUUID[:])
			s.uuid = clientUUID
		} else {
			s.uuid = makeOfflineUUID(name)
		}

		logInfo("[登录] 玩家 %s (协议: %d) 请求进入, 下发登录确认帧...", s.username, s.clientProto)
		s.SendPacket(0x02, buildLoginSuccess(s.clientProto, s.uuid, s.username))
	case 0x03:
		if !s.transitionState(StateLogin, StateConfig) {
			return
		}
		s.SendPacket(ConfigPktClientBoundKnownPacks, buildKnownPacks("26.2"))
	}
}

func (s *PlayerSession) handleConfig(packetID int, payload []byte) {
	switch packetID {
	case ConfigPktServerBoundClientInfo:
	case ConfigPktServerBoundKnownPacks:
		if s.configurationSent {
			logWarn("[protocol] duplicate KnownPacks from %s during Configuration", s.remoteAddress())
			s.Close()
			return
		}
		s.configurationSent = true
		if s.server == nil || !s.server.sendConfiguration(s) {
			s.Close()
		}
	case ConfigPktServerBoundFinishConfig:
		if !s.configurationSent {
			logWarn("[protocol] premature FinishConfiguration from %s", s.remoteAddress())
			s.Close()
			return
		}
		if s.server == nil {
			s.Close()
			return
		}
		if !s.transitionState(StateConfig, StatePlayPending) {
			return
		}
		if !s.server.postRuntime(func() {
			s.enterPlay()
		}) {
			logWarn("[runtime] unable to enqueue Play initialization for %s", s.remoteAddress())
			s.Close()
		}
	case ConfigPktServerBoundKeepAlive:
	case ConfigPktServerBoundPong:
	}
}

// enterPlay runs only on the tick owner. It initializes authoritative gameplay
// state, atomically publishes the Pending -> Play transition, and only then
// emits Play packets. A concurrent Close wins the CAS and cannot be resurrected.
func (s *PlayerSession) enterPlay() {
	if s == nil || s.server == nil || s.State() != StatePlayPending || s.closeOnce.Load() {
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

	if !s.transitionState(StatePlayPending, StatePlay) {
		return
	}

	if !s.sendInitialPlayPackets(cfg) {
		s.Close()
		return
	}
	if s.State() != StatePlay || s.closeOnce.Load() {
		return
	}

	s.server.AddPlayer(s)
	if !s.registered.Load() {
		return
	}

	welcome := fmt.Sprintf("&b&l[ CyuCore ] &a欢迎 &f%s &a接入 Minecraft 26.2 原生游戏核心！", s.username)
	s.SendSystemMessage(welcome)
	s.SendSystemMessage("&7[提示] 输入 &e/help &7查看可用指令，按 Tab 可查看在线玩家列表。")
	s.server.BroadcastMessage(fmt.Sprintf("&e玩家 &f%s &e加入了服务器！", s.username), s)
}

func (s *PlayerSession) sendInitialPlayPackets(cfg *ServerConfig) bool {
	if !s.SendPacket(PlayPktClientBoundLogin, buildPlayLogin(s.entityID, cfg.MaxPlayers, s.gameMode, true)) {
		return false
	}
	if !s.SendPacket(PlayPktClientBoundPlayerPosition, buildPlayerPositionSync(s.teleportSeq, s.x, s.y, s.z, s.yaw, s.pitch)) {
		return false
	}
	if !s.SendPacket(PlayPktClientBoundSetCenterChunk, buildCenterChunk(0, 0)) {
		return false
	}
	if !s.SendPacket(PlayPktClientBoundChunkBatchStart, nil) {
		return false
	}
	for cx := -1; cx <= 1; cx++ {
		for cz := -1; cz <= 1; cz++ {
			if !s.SendPacket(PlayPktClientBoundLevelChunk, buildChunkPacket(cx, cz, cfg.PlatformBlock)) {
				return false
			}
		}
	}
	buf := bytes.NewBuffer(writeVarInt(9))
	if !s.SendPacket(PlayPktClientBoundChunkBatchFinished, buf.Bytes()) {
		return false
	}
	if !s.SendPacket(PlayPktClientBoundPlayerInfoUpdate, buildPlayerInfoAdd(s.uuid, s.username, s.gameMode)) {
		return false
	}
	return s.SendPacket(PlayPktClientBoundGameEvent, buildGameEventWaitingChunks())
}

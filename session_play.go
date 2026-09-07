package main

import (
	"bytes"
	"encoding/binary"
)

func (s *PlayerSession) handlePlay(packetID int, payload []byte) {
	switch packetID {
	case PlayPktServerBoundAcceptTeleport:
	case PlayPktServerBoundChunkBatchRecv:
	case PlayPktServerBoundPlayerLoaded:
	case PlayPktServerBoundKeepAlive:
	case PlayPktServerBoundMovePos:
		buf := bytes.NewReader(payload)
		move := playerMove{hasPosition: true}
		if binary.Read(buf, binary.BigEndian, &move.x) != nil ||
			binary.Read(buf, binary.BigEndian, &move.y) != nil ||
			binary.Read(buf, binary.BigEndian, &move.z) != nil ||
			!readMoveGrounded(buf, &move) {
			return
		}
		s.server.postPlayerRuntime(s, func() { s.server.applyPlayerMove(s, move) })
	case PlayPktServerBoundMovePosRot:
		buf := bytes.NewReader(payload)
		move := playerMove{hasPosition: true, hasRotation: true}
		if binary.Read(buf, binary.BigEndian, &move.x) != nil ||
			binary.Read(buf, binary.BigEndian, &move.y) != nil ||
			binary.Read(buf, binary.BigEndian, &move.z) != nil ||
			binary.Read(buf, binary.BigEndian, &move.yaw) != nil ||
			binary.Read(buf, binary.BigEndian, &move.pitch) != nil ||
			!readMoveGrounded(buf, &move) {
			return
		}
		s.server.postPlayerRuntime(s, func() { s.server.applyPlayerMove(s, move) })
	case PlayPktServerBoundMoveRot:
		buf := bytes.NewReader(payload)
		move := playerMove{hasRotation: true}
		if binary.Read(buf, binary.BigEndian, &move.yaw) != nil ||
			binary.Read(buf, binary.BigEndian, &move.pitch) != nil ||
			!readMoveGrounded(buf, &move) {
			return
		}
		s.server.postPlayerRuntime(s, func() { s.server.applyPlayerMove(s, move) })
	case PlayPktServerBoundMoveStatus:
		buf := bytes.NewReader(payload)
		var move playerMove
		if !readMoveGrounded(buf, &move) {
			return
		}
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

func readMoveGrounded(buf *bytes.Reader, move *playerMove) bool {
	var onGround byte
	if binary.Read(buf, binary.BigEndian, &onGround) != nil {
		return false
	}
	move.onGround = onGround != 0
	return true
}

func (s *PlayerSession) handlePlayerAction(payload []byte) {
	buf := bytes.NewReader(payload)
	status, err := readVarInt(buf)
	if err != nil {
		return
	}
	var posVal uint64
	if err := binary.Read(buf, binary.BigEndian, &posVal); err != nil {
		return
	}
	var face byte
	if err := binary.Read(buf, binary.BigEndian, &face); err != nil {
		return
	}
	seq, err := readVarInt(buf)
	if err != nil || (status != 0 && status != 2) {
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
	if _, err := readVarInt(buf); err != nil {
		return
	}
	var posVal uint64
	if err := binary.Read(buf, binary.BigEndian, &posVal); err != nil {
		return
	}
	face, err := readVarInt(buf)
	if err != nil {
		return
	}
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
	seq, err := readVarInt(buf)
	if err != nil {
		return
	}

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

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
)

const (
	ProtocolVersion26_2 = 776
	EntityTypePlayer    = 128
)

const (
	BlockAir         = 0
	BlockStone       = 1
	BlockGrass       = 9
	BlockBedrock     = 33
	BlockStoneBricks = 330
	BlockGlowstone   = 296
)

const (
	ConfigPktClientBoundFinishConfig = 0x03
	ConfigPktClientBoundKeepAlive    = 0x04
	ConfigPktClientBoundPing         = 0x05
	ConfigPktClientBoundRegistryData = 0x07
	ConfigPktClientBoundUpdateTags   = 0x0d
	ConfigPktClientBoundKnownPacks   = 0x0e

	ConfigPktServerBoundClientInfo   = 0x00
	ConfigPktServerBoundFinishConfig = 0x03
	ConfigPktServerBoundKeepAlive    = 0x04
	ConfigPktServerBoundPong         = 0x05
	ConfigPktServerBoundKnownPacks   = 0x07
)

const (
	PlayPktClientBoundAddEntity          = 0x01
	PlayPktClientBoundAnimate            = 0x02
	PlayPktClientBoundBlockChangedAck    = 0x04
	PlayPktClientBoundBlockUpdate        = 0x08
	PlayPktClientBoundChunkBatchFinished = 0x0b
	PlayPktClientBoundChunkBatchStart    = 0x0c
	PlayPktClientBoundCommands           = 0x10
	PlayPktClientBoundDisconnect         = 0x20
	PlayPktClientBoundEntityPositionSync = 0x23
	PlayPktClientBoundGameEvent          = 0x26
	PlayPktClientBoundKeepAlive          = 0x2c
	PlayPktClientBoundLevelChunk         = 0x2d
	PlayPktClientBoundLogin              = 0x31
	PlayPktClientBoundPlayerInfoUpdate   = 0x46
	PlayPktClientBoundPlayerPosition     = 0x48
	PlayPktClientBoundRemoveEntities     = 0x4d
	PlayPktClientBoundSetCenterChunk     = 0x5e
	PlayPktClientBoundSetTime            = 0x71
	PlayPktClientBoundSystemChat         = 0x79

	PlayPktServerBoundAcceptTeleport = 0x00
	PlayPktServerBoundChatCommand    = 0x07
	PlayPktServerBoundChat           = 0x09
	PlayPktServerBoundChunkBatchRecv = 0x0b
	PlayPktServerBoundKeepAlive      = 0x1c
	PlayPktServerBoundMovePos        = 0x1e
	PlayPktServerBoundMovePosRot     = 0x1f
	PlayPktServerBoundMoveRot        = 0x20
	PlayPktServerBoundMoveStatus     = 0x21
	PlayPktServerBoundPlayerAction   = 0x29
	PlayPktServerBoundPlayerCommand  = 0x2a
	PlayPktServerBoundPlayerLoaded   = 0x2c
	PlayPktServerBoundSwing          = 0x3f
	PlayPktServerBoundUseItemOn      = 0x42
)

func packPosition(x, y, z int) uint64 {
	ux := uint64(x) & 0x3FFFFFF
	uz := uint64(z) & 0x3FFFFFF
	uy := uint64(y) & 0xFFF
	return (ux << 38) | (uz << 12) | uy
}

func unpackPosition(val uint64) (int, int, int) {
	x := int(int64(val) >> 38)
	y := int(int64(val<<52) >> 52)
	z := int(int64(val<<26) >> 38)
	return x, y, z
}

func buildStatusResponse(version string, proto int, max, online int, motd1, motd2 string) []byte {
	m := map[string]interface{}{
		"version": map[string]interface{}{
			"name":     version,
			"protocol": proto,
		},
		"players": map[string]interface{}{
			"max":    max,
			"online": online,
			"sample": []interface{}{},
		},
		"description": map[string]string{
			"text": translateColorCodes(motd1 + "\n" + motd2),
		},
		"enforcesSecureChat": false,
	}
	raw, _ := json.Marshal(m)
	return writeString(string(raw))
}

func buildLoginSuccess(proto int, uuid [16]byte, name string) []byte {
	var buf bytes.Buffer
	buf.Write(uuid[:])
	buf.Write(writeString(name))
	buf.Write(writeVarInt(0))
	if proto == 766 || proto == 767 {
		buf.WriteByte(0)
	} else {
		buf.Write(uuid[:])
	}
	return buf.Bytes()
}

func buildKnownPacks(version string) []byte {
	var buf bytes.Buffer
	buf.Write(writeVarInt(1))
	buf.Write(writeString("minecraft"))
	buf.Write(writeString("core"))
	buf.Write(writeString(version))
	return buf.Bytes()
}

func buildPlayLogin(entityID int, maxPlayers int, gameMode int, isFlat bool) []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(entityID))
	buf.WriteByte(0)

	buf.Write(writeVarInt(1))
	buf.Write(writeString("minecraft:overworld"))

	buf.Write(writeVarInt(maxPlayers))
	buf.Write(writeVarInt(8))
	buf.Write(writeVarInt(8))
	buf.WriteByte(0)
	buf.WriteByte(1)
	buf.WriteByte(0)

	buf.Write(writeVarInt(0))
	buf.Write(writeString("minecraft:overworld"))
	binary.Write(&buf, binary.BigEndian, int64(123456789))
	buf.WriteByte(byte(gameMode))
	buf.WriteByte(0xFF)
	buf.WriteByte(0)
	if isFlat {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	buf.WriteByte(0)
	buf.Write(writeVarInt(0))
	buf.Write(writeVarInt(63))

	buf.WriteByte(0)
	buf.WriteByte(0)

	return buf.Bytes()
}

func buildPlayerPositionSync(teleportID int, x, y, z float64, yaw, pitch float32) []byte {
	var buf bytes.Buffer
	buf.Write(writeVarInt(teleportID))
	binary.Write(&buf, binary.BigEndian, x)
	binary.Write(&buf, binary.BigEndian, y)
	binary.Write(&buf, binary.BigEndian, z)
	binary.Write(&buf, binary.BigEndian, float64(0))
	binary.Write(&buf, binary.BigEndian, float64(0))
	binary.Write(&buf, binary.BigEndian, float64(0))
	binary.Write(&buf, binary.BigEndian, yaw)
	binary.Write(&buf, binary.BigEndian, pitch)
	binary.Write(&buf, binary.BigEndian, int32(0))
	return buf.Bytes()
}

func buildCenterChunk(cx, cz int) []byte {
	var buf bytes.Buffer
	buf.Write(writeVarInt(cx))
	buf.Write(writeVarInt(cz))
	return buf.Bytes()
}

func buildGameEventWaitingChunks() []byte {
	var buf bytes.Buffer
	buf.WriteByte(13)
	binary.Write(&buf, binary.BigEndian, float32(0.0))
	return buf.Bytes()
}

func buildGameEventGameMode(mode int) []byte {
	var buf bytes.Buffer
	buf.WriteByte(3)
	binary.Write(&buf, binary.BigEndian, float32(mode))
	return buf.Bytes()
}

func buildPlayerInfoAdd(uuid [16]byte, name string, gameMode int) []byte {
	var buf bytes.Buffer
	buf.WriteByte(0x01 | 0x04 | 0x08 | 0x10 | 0x20)
	buf.Write(writeVarInt(1))
	buf.Write(uuid[:])

	buf.Write(writeString(name))
	buf.Write(writeVarInt(0))

	buf.Write(writeVarInt(gameMode))

	buf.WriteByte(1)

	buf.Write(writeVarInt(1))

	buf.WriteByte(0)

	return buf.Bytes()
}

func buildPlayerInfoRemove(uuid [16]byte) []byte {
	var buf bytes.Buffer
	buf.Write(writeVarInt(1))
	buf.Write(uuid[:])
	return buf.Bytes()
}

func buildAddPlayerEntity(entityID int, uuid [16]byte, x, y, z float64, yaw, pitch float32) []byte {
	var buf bytes.Buffer
	buf.Write(writeVarInt(entityID))
	buf.Write(uuid[:])
	buf.Write(writeVarInt(EntityTypePlayer))
	binary.Write(&buf, binary.BigEndian, x)
	binary.Write(&buf, binary.BigEndian, y)
	binary.Write(&buf, binary.BigEndian, z)
	buf.WriteByte(byte(pitch * 256.0 / 360.0))
	buf.WriteByte(byte(yaw * 256.0 / 360.0))
	buf.WriteByte(byte(yaw * 256.0 / 360.0))
	buf.Write(writeVarInt(0))
	binary.Write(&buf, binary.BigEndian, int16(0))
	binary.Write(&buf, binary.BigEndian, int16(0))
	binary.Write(&buf, binary.BigEndian, int16(0))
	return buf.Bytes()
}

func buildEntityPositionSync(entityID int, x, y, z float64, yaw, pitch float32, onGround bool) []byte {
	var buf bytes.Buffer
	buf.Write(writeVarInt(entityID))
	binary.Write(&buf, binary.BigEndian, x)
	binary.Write(&buf, binary.BigEndian, y)
	binary.Write(&buf, binary.BigEndian, z)
	binary.Write(&buf, binary.BigEndian, float64(0))
	binary.Write(&buf, binary.BigEndian, float64(0))
	binary.Write(&buf, binary.BigEndian, float64(0))
	binary.Write(&buf, binary.BigEndian, yaw)
	binary.Write(&buf, binary.BigEndian, pitch)
	if onGround {
		buf.WriteByte(1)
	} else {
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

func buildRemoveEntities(entityIDs ...int) []byte {
	var buf bytes.Buffer
	buf.Write(writeVarInt(len(entityIDs)))
	for _, id := range entityIDs {
		buf.Write(writeVarInt(id))
	}
	return buf.Bytes()
}

func buildAnimate(entityID int, animation byte) []byte {
	var buf bytes.Buffer
	buf.Write(writeVarInt(entityID))
	buf.WriteByte(animation)
	return buf.Bytes()
}

func buildBlockUpdate(x, y, z int, blockStateID int) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, packPosition(x, y, z))
	buf.Write(writeVarInt(blockStateID))
	return buf.Bytes()
}

func buildBlockChangedAck(sequence int) []byte {
	var buf bytes.Buffer
	buf.Write(writeVarInt(sequence))
	return buf.Bytes()
}

func buildSetTime(worldAge, timeOfDay int64) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, worldAge)
	binary.Write(&buf, binary.BigEndian, timeOfDay)
	return buf.Bytes()
}

func buildSystemChatMessage(text string) []byte {
	var buf bytes.Buffer
	formatted := translateColorCodes(text)
	buf.WriteByte(0x08)
	binary.Write(&buf, binary.BigEndian, uint16(len(formatted)))
	buf.WriteString(formatted)
	buf.WriteByte(0)
	return buf.Bytes()
}

func buildDisconnect(reason string) []byte {
	var buf bytes.Buffer
	formatted := translateColorCodes(reason)
	buf.WriteByte(0x08)
	binary.Write(&buf, binary.BigEndian, uint16(len(formatted)))
	buf.WriteString(formatted)
	return buf.Bytes()
}

func buildCommandsPacket() []byte {
	var buf bytes.Buffer
	cmds := []string{"help", "gamemode", "tp", "time", "say", "spawn", "clear", "ping"}

	nodeCount := 1 + len(cmds)
	buf.Write(writeVarInt(nodeCount))

	buf.WriteByte(0x00)
	buf.Write(writeVarInt(len(cmds)))
	for i := 1; i <= len(cmds); i++ {
		buf.Write(writeVarInt(i))
	}

	for _, cmd := range cmds {
		buf.WriteByte(0x01)
		buf.Write(writeVarInt(0))
		buf.Write(writeString(cmd))
	}

	buf.Write(writeVarInt(0))

	return buf.Bytes()
}

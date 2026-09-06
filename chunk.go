package main

import (
	"bytes"
	"encoding/binary"
)

func buildChunkPacket(chunkX, chunkZ int, platformBlockID int) []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(chunkX))
	binary.Write(&buf, binary.BigEndian, int32(chunkZ))

	buf.Write(writeVarInt(1))
	buf.Write(writeVarInt(4))
	buf.Write(writeVarInt(37))
	for i := 0; i < 37; i++ {
		binary.Write(&buf, binary.BigEndian, uint64(0))
	}

	var dataBuf bytes.Buffer
	for s := 0; s < 24; s++ {
		if (s == 4 || s == 8 || s == 12) && chunkX == 0 && chunkZ == 0 {
			dataBuf.Write(buildPlatformSection(platformBlockID))
		} else {
			dataBuf.Write(buildEmptySection())
		}
	}

	sectionBytes := dataBuf.Bytes()
	buf.Write(writeVarInt(len(sectionBytes)))
	buf.Write(sectionBytes)

	buf.Write(writeVarInt(0))

	buf.WriteByte(0)
	buf.WriteByte(0)
	buf.WriteByte(0)
	buf.WriteByte(0)
	buf.Write(writeVarInt(0))
	buf.Write(writeVarInt(0))

	return buf.Bytes()
}

func buildEmptySection() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, int16(0))
	binary.Write(&buf, binary.BigEndian, int16(0))

	buf.WriteByte(0)
	buf.Write(writeVarInt(0))

	buf.WriteByte(0)
	buf.Write(writeVarInt(1))

	return buf.Bytes()
}

func buildPlatformSection(blockStateID int) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, int16(256))
	binary.Write(&buf, binary.BigEndian, int16(0))

	buf.WriteByte(4)
	buf.Write(writeVarInt(2))
	buf.Write(writeVarInt(0))
	buf.Write(writeVarInt(blockStateID))

	buf.Write(writeVarInt(256))
	for i := 0; i < 16; i++ {
		binary.Write(&buf, binary.BigEndian, uint64(0x1111111111111111))
	}
	for i := 0; i < 240; i++ {
		binary.Write(&buf, binary.BigEndian, uint64(0))
	}

	buf.WriteByte(0)
	buf.Write(writeVarInt(1))

	return buf.Bytes()
}

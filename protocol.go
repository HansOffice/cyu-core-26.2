package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"strings"

	mcproto "cyu-core-26.2/internal/protocol"
)

func readVarInt(r io.Reader) (int, error) {
	value, err := mcproto.ReadVarInt(r)
	return int(value), err
}

func writeVarInt(val int) []byte {
	return mcproto.AppendVarInt(nil, int32(val))
}

func readString(r io.Reader) (string, error) {
	return mcproto.ReadString(r, mcproto.DefaultMaxStringSize)
}

func writeString(s string) []byte {
	return mcproto.AppendString(nil, s)
}

func readPacket(r io.Reader) (int, []byte, error) {
	packetID, payload, err := mcproto.ReadPacket(r, mcproto.DefaultMaxPacketSize)
	return int(packetID), payload, err
}

func writePacket(w io.Writer, packetID int, payload []byte) error {
	return mcproto.WritePacket(w, int32(packetID), payload, mcproto.DefaultMaxPacketSize)
}

func makeOfflineUUID(name string) [16]byte {
	hash := md5.Sum([]byte("OfflinePlayer:" + name))
	hash[6] = (hash[6] & 0x0f) | 0x30
	hash[8] = (hash[8] & 0x3f) | 0x80
	return hash
}

func formatUUID(u [16]byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

func translateColorCodes(s string) string {
	validCodes := "0123456789abcdefklmnorABCDEFKLMNOR"
	var sb strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '&' && i+1 < len(runes) && strings.ContainsRune(validCodes, runes[i+1]) {
			sb.WriteRune('§')
			sb.WriteRune(runes[i+1])
			i++
		} else {
			sb.WriteRune(runes[i])
		}
	}
	return sb.String()
}

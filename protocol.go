package main

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"strings"
)

func readVarInt(r io.Reader) (int, error) {
	var val uint32
	var shift uint
	for {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		val |= uint32(b[0]&0x7F) << shift
		if (b[0] & 0x80) == 0 {
			break
		}
		shift += 7
		if shift >= 35 {
			return 0, fmt.Errorf("varint exceeds 35 bits")
		}
	}
	return int(int32(val)), nil
}

func writeVarInt(val int) []byte {
	uval := uint32(int32(val))
	var buf []byte
	for {
		b := byte(uval & 0x7F)
		uval >>= 7
		if uval != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if uval == 0 {
			break
		}
	}
	return buf
}

func readString(r io.Reader) (string, error) {
	length, err := readVarInt(r)
	if err != nil {
		return "", err
	}
	if length < 0 || length > 32767 {
		return "", fmt.Errorf("invalid string length %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func writeString(s string) []byte {
	b := []byte(s)
	lenBytes := writeVarInt(len(b))
	return append(lenBytes, b...)
}

func readPacket(r io.Reader) (int, []byte, error) {
	length, err := readVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if length <= 0 || length > 2097152 {
		return 0, nil, fmt.Errorf("invalid packet length %d", length)
	}
	raw := make([]byte, length)
	if _, err := io.ReadFull(r, raw); err != nil {
		return 0, nil, err
	}
	buf := bytes.NewReader(raw)
	packetID, err := readVarInt(buf)
	if err != nil {
		return 0, nil, err
	}
	remaining := buf.Len()
	payload := raw[len(raw)-remaining:]
	return packetID, payload, nil
}

func writePacket(w io.Writer, packetID int, payload []byte) error {
	idBytes := writeVarInt(packetID)
	totalLen := len(idBytes) + len(payload)
	lenBytes := writeVarInt(totalLen)

	var packet []byte
	packet = append(packet, lenBytes...)
	packet = append(packet, idBytes...)
	packet = append(packet, payload...)
	_, err := w.Write(packet)
	return err
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

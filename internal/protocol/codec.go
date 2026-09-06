package protocol

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

const (
	MaxVarIntBytes       = 5
	DefaultMaxStringSize = 32767
	DefaultMaxPacketSize = 2 * 1024 * 1024
)

var ErrVarIntTooLong = errors.New("protocol: varint exceeds 5 bytes")

func ReadVarInt(r io.Reader) (int32, error) {
	var value uint32
	for i := 0; i < MaxVarIntBytes; i++ {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		value |= uint32(b[0]&0x7f) << (7 * i)
		if b[0]&0x80 == 0 {
			return int32(value), nil
		}
	}
	return 0, ErrVarIntTooLong
}

func AppendVarInt(dst []byte, value int32) []byte {
	u := uint32(value)
	for {
		b := byte(u & 0x7f)
		u >>= 7
		if u != 0 {
			b |= 0x80
		}
		dst = append(dst, b)
		if u == 0 {
			return dst
		}
	}
}

func ReadString(r io.Reader, maxBytes int) (string, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}
	if length < 0 || int(length) > maxBytes {
		return "", fmt.Errorf("protocol: invalid string length %d (max %d)", length, maxBytes)
	}
	buf := make([]byte, int(length))
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func AppendString(dst []byte, value string) []byte {
	dst = AppendVarInt(dst, int32(len(value)))
	return append(dst, value...)
}

func ReadPacket(r io.Reader, maxBytes int) (int32, []byte, error) {
	length, err := ReadVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if length <= 0 || int(length) > maxBytes {
		return 0, nil, fmt.Errorf("protocol: invalid packet length %d (max %d)", length, maxBytes)
	}

	raw := make([]byte, int(length))
	if _, err := io.ReadFull(r, raw); err != nil {
		return 0, nil, err
	}

	buf := bytes.NewReader(raw)
	packetID, err := ReadVarInt(buf)
	if err != nil {
		return 0, nil, err
	}
	payload := raw[len(raw)-buf.Len():]
	return packetID, payload, nil
}

func WritePacket(w io.Writer, packetID int32, payload []byte, maxBytes int) error {
	header := make([]byte, 0, 10)
	id := AppendVarInt(nil, packetID)
	totalLength := len(id) + len(payload)
	if totalLength <= 0 || totalLength > maxBytes {
		return fmt.Errorf("protocol: invalid packet length %d (max %d)", totalLength, maxBytes)
	}
	header = AppendVarInt(header, int32(totalLength))
	header = append(header, id...)

	if err := writeAll(w, header); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

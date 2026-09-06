package protocol

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestVarIntRoundTrip(t *testing.T) {
	values := []int32{0, 1, 2, 127, 128, 255, 25565, 2097151, 2147483647, -1, -2147483648}
	for _, want := range values {
		encoded := AppendVarInt(nil, want)
		got, err := ReadVarInt(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("ReadVarInt(%d): %v", want, err)
		}
		if got != want {
			t.Fatalf("ReadVarInt round trip: want %d got %d", want, got)
		}
	}
}

func TestReadVarIntRejectsOversizedEncoding(t *testing.T) {
	_, err := ReadVarInt(bytes.NewReader([]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x00}))
	if !errors.Is(err, ErrVarIntTooLong) {
		t.Fatalf("expected ErrVarIntTooLong, got %v", err)
	}
}

func TestStringRoundTripAndLimit(t *testing.T) {
	encoded := AppendString(nil, "CyuCore 世界")
	got, err := ReadString(bytes.NewReader(encoded), DefaultMaxStringSize)
	if err != nil {
		t.Fatal(err)
	}
	if got != "CyuCore 世界" {
		t.Fatalf("unexpected string %q", got)
	}

	_, err = ReadString(bytes.NewReader(AppendString(nil, "abcd")), 3)
	if err == nil {
		t.Fatal("expected string length error")
	}
}

func TestPacketRoundTrip(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5}
	var wire bytes.Buffer
	if err := WritePacket(&wire, 0x2d, payload, DefaultMaxPacketSize); err != nil {
		t.Fatal(err)
	}

	id, got, err := ReadPacket(&wire, DefaultMaxPacketSize)
	if err != nil {
		t.Fatal(err)
	}
	if id != 0x2d {
		t.Fatalf("expected packet ID 0x2d, got %#x", id)
	}
	if !reflect.DeepEqual(got, payload) {
		t.Fatalf("payload mismatch: want %v got %v", payload, got)
	}
}

func TestPacketSizeLimits(t *testing.T) {
	var wire bytes.Buffer
	if err := WritePacket(&wire, 1, make([]byte, 8), 4); err == nil {
		t.Fatal("expected write size error")
	}

	oversized := AppendVarInt(nil, 5)
	oversized = append(oversized, make([]byte, 5)...)
	if _, _, err := ReadPacket(bytes.NewReader(oversized), 4); err == nil {
		t.Fatal("expected read size error")
	}
}

func TestWritePacketHandlesPartialWrites(t *testing.T) {
	var dst bytes.Buffer
	w := &partialWriter{dst: &dst, max: 2}
	payload := []byte{9, 8, 7, 6, 5}
	if err := WritePacket(w, 3, payload, DefaultMaxPacketSize); err != nil {
		t.Fatal(err)
	}

	id, got, err := ReadPacket(&dst, DefaultMaxPacketSize)
	if err != nil {
		t.Fatal(err)
	}
	if id != 3 || !reflect.DeepEqual(got, payload) {
		t.Fatalf("round trip mismatch: id=%d payload=%v", id, got)
	}
}

func TestWritePacketRejectsWriterWithoutProgress(t *testing.T) {
	err := WritePacket(zeroWriter{}, 1, []byte{1}, DefaultMaxPacketSize)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}
}

type partialWriter struct {
	dst *bytes.Buffer
	max int
}

func (w *partialWriter) Write(p []byte) (int, error) {
	if len(p) > w.max {
		p = p[:w.max]
	}
	return w.dst.Write(p)
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }

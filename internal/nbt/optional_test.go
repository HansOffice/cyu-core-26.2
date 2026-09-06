package nbt

import (
	"bytes"
	"testing"
)

func TestWriteEndSentinelOptionalNetworkUsesEndForAbsentValue(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEndSentinelOptionalNetwork(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if got := buf.Bytes(); !bytes.Equal(got, []byte{0}) {
		t.Fatalf("absent end-sentinel NBT = %x, want 00", got)
	}
}

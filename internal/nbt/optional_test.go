package nbt

import (
	"bytes"
	"testing"
)

func TestWriteOptionalNetworkUsesEndForAbsentValue(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteOptionalNetwork(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if got := buf.Bytes(); !bytes.Equal(got, []byte{0}) {
		t.Fatalf("absent optional NBT = %x, want 00", got)
	}
}

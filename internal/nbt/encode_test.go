package nbt

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func TestWriteNetworkOmitsRootName(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteNetwork(&buf, Compound{"a": Int(1)}); err != nil {
		t.Fatal(err)
	}
	assertHex(t, buf.Bytes(), "0a030001610000000100")
}

func TestWriteNetworkUsesJavaModifiedUTF8(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "ASCII", value: "abc", want: "080003616263"},
		{name: "NUL", value: "\x00", want: "080002c080"},
		{name: "supplementary", value: "😀", want: "080006eda0bdedb880"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteNetwork(&buf, String(test.value)); err != nil {
				t.Fatal(err)
			}
			assertHex(t, buf.Bytes(), test.want)
		})
	}
}

func TestCompoundEncodingIsDeterministic(t *testing.T) {
	value := Compound{
		"z": Byte(2),
		"a": Byte(1),
	}

	var first bytes.Buffer
	var second bytes.Buffer
	if err := WriteNetwork(&first, value); err != nil {
		t.Fatal(err)
	}
	if err := WriteNetwork(&second, value); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("compound encoding is not deterministic:\n%x\n%x", first.Bytes(), second.Bytes())
	}
	assertHex(t, first.Bytes(), "0a01000161010100017a0200")
}

func TestListIsHomogeneous(t *testing.T) {
	if _, err := NewList(Int(1), Long(2)); err == nil {
		t.Fatal("expected NewList to reject mixed element types")
	}

	var buf bytes.Buffer
	err := WriteNetwork(&buf, List{ElementType: IntType, Values: []Value{Int(1), Long(2)}})
	if err == nil || !strings.Contains(err.Error(), "heterogeneous list") {
		t.Fatalf("expected encoder to reject mixed element types, got %v", err)
	}
}

func TestWriteNetworkListAndArrays(t *testing.T) {
	list, err := NewList(Int(1), Int(2))
	if err != nil {
		t.Fatal(err)
	}
	var listBuf bytes.Buffer
	if err := WriteNetwork(&listBuf, list); err != nil {
		t.Fatal(err)
	}
	assertHex(t, listBuf.Bytes(), "0903000000020000000100000002")

	var ints bytes.Buffer
	if err := WriteNetwork(&ints, IntArray{1, -1}); err != nil {
		t.Fatal(err)
	}
	assertHex(t, ints.Bytes(), "0b0000000200000001ffffffff")
}

func TestWriteNetworkRejectsInvalidValues(t *testing.T) {
	if err := WriteNetwork(&bytes.Buffer{}, nil); err == nil {
		t.Fatal("expected nil root error")
	}
	if err := WriteNetwork(&bytes.Buffer{}, Compound{"bad": nil}); err == nil {
		t.Fatal("expected nil compound value error")
	}
	if err := WriteNetwork(&bytes.Buffer{}, String(string([]byte{0xff}))); err == nil {
		t.Fatal("expected invalid UTF-8 error")
	}
}

func TestCloneIsDeep(t *testing.T) {
	original := Compound{
		"bytes":  ByteArray{1, 2},
		"nested": Compound{"value": Int(3)},
	}
	cloned := Clone(original).(Compound)
	cloned["bytes"].(ByteArray)[0] = 9
	cloned["nested"].(Compound)["value"] = Int(4)

	if got := original["bytes"].(ByteArray)[0]; got != 1 {
		t.Fatalf("byte array mutated through clone: %d", got)
	}
	if got := original["nested"].(Compound)["value"].(Int); got != 3 {
		t.Fatalf("nested compound mutated through clone: %d", got)
	}
}

func assertHex(t *testing.T, got []byte, want string) {
	t.Helper()
	wantBytes, err := hex.DecodeString(want)
	if err != nil {
		t.Fatalf("invalid expected hex: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Fatalf("wire bytes mismatch:\nwant %x\n got %x", wantBytes, got)
	}
}

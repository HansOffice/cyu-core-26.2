package nbt

import (
	"bytes"
	"reflect"
	"testing"
)

func TestNetworkReadWriteRoundTrip(t *testing.T) {
	list, err := NewList(Int(1), Int(-2), Int(3))
	if err != nil {
		t.Fatal(err)
	}
	value := Compound{
		"byte":       Byte(-7),
		"short":      Short(-300),
		"int":        Int(-123456),
		"long":       Long(-0x102030405060708),
		"float":      Float(1.25),
		"double":     Double(-2.5),
		"bytes":      ByteArray{0, 1, 0xff},
		"string":     String("nul:\x00 snowman:☃ music:𝄞"),
		"list":       list,
		"empty_list": List{ElementType: StringType},
		"ints":       IntArray{-1, 0, 1},
		"longs":      LongArray{-1, 0x102030405060708},
	}

	var encoded bytes.Buffer
	if err := WriteNetwork(&encoded, value); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadNetwork(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, value) {
		t.Fatalf("network round trip mismatch:\nwant %#v\n got %#v", value, decoded)
	}
}

func TestTypedJSONRoundTrip(t *testing.T) {
	value := Compound{
		"long": Long(-1),
		"list": List{ElementType: LongType, Values: []Value{Long(1), Long(-2)}},
		"array": LongArray{1, -2},
		"nested": Compound{
			"enabled": Byte(1),
		},
	}
	encoded, err := EncodeJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, value) {
		t.Fatalf("json round trip mismatch:\njson %s\nwant %#v\n got %#v", encoded, value, decoded)
	}
}

func TestReadNetworkRejectsMalformedData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "end root", data: []byte{0}},
		{name: "unknown root", data: []byte{99}},
		{name: "negative byte array length", data: []byte{byte(ByteArrayType), 0xff, 0xff, 0xff, 0xff}},
		{name: "truncated compound", data: []byte{byte(CompoundType), byte(IntType), 0, 1, 'x'}},
		{name: "invalid modified utf8", data: []byte{byte(StringType), 0, 1, 0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ReadNetwork(bytes.NewReader(test.data)); err == nil {
				t.Fatal("expected malformed NBT error")
			}
		})
	}
}

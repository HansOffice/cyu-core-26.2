package nbt

import (
	"strings"
	"testing"
)

func TestDecodeJSONMatchesPrismarineTypedNBT(t *testing.T) {
	const source = `{
		"type":"compound",
		"value":{
			"natural":{"type":"byte","value":1},
			"height":{"type":"int","value":384},
			"scale":{"type":"float","value":0.1},
			"name":{"type":"string","value":"minecraft:test"},
			"seed":{"type":"long","value":[-1,-1]},
			"values":{"type":"list","value":{"type":"compound","value":[
				{"id":{"type":"string","value":"minecraft:first"}},
				{"id":{"type":"string","value":"minecraft:second"}}
			]}}
		}
	}`

	value, err := DecodeJSON([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	compound := value.(Compound)
	if _, ok := compound["natural"].(Byte); !ok {
		t.Fatalf("natural type = %T, want nbt.Byte", compound["natural"])
	}
	if _, ok := compound["height"].(Int); !ok {
		t.Fatalf("height type = %T, want nbt.Int", compound["height"])
	}
	if _, ok := compound["scale"].(Float); !ok {
		t.Fatalf("scale type = %T, want nbt.Float", compound["scale"])
	}
	if got := compound["seed"].(Long); got != -1 {
		t.Fatalf("seed = %d, want -1", got)
	}
	list := compound["values"].(List)
	if list.ElementType != CompoundType || len(list.Values) != 2 {
		t.Fatalf("unexpected list: %#v", list)
	}
	first := list.Values[0].(Compound)
	if got := first["id"].(String); got != "minecraft:first" {
		t.Fatalf("first list value = %q", got)
	}
}

func TestDecodeJSONPrismarineArrays(t *testing.T) {
	tests := []struct {
		name   string
		source string
		check  func(t *testing.T, value Value)
	}{
		{
			name:   "byte array",
			source: `{"type":"byteArray","value":[-1,0,127]}`,
			check: func(t *testing.T, value Value) {
				got := value.(ByteArray)
				if len(got) != 3 || got[0] != 0xff || got[2] != 0x7f {
					t.Fatalf("byteArray = %v", got)
				}
			},
		},
		{
			name:   "int array",
			source: `{"type":"intArray","value":[1,-2]}`,
			check: func(t *testing.T, value Value) {
				got := value.(IntArray)
				if len(got) != 2 || got[0] != 1 || got[1] != -2 {
					t.Fatalf("intArray = %v", got)
				}
			},
		},
		{
			name:   "long array",
			source: `{"type":"longArray","value":[[0,1],[-1,-1]]}`,
			check: func(t *testing.T, value Value) {
				got := value.(LongArray)
				if len(got) != 2 || got[0] != 1 || got[1] != -1 {
					t.Fatalf("longArray = %v", got)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := DecodeJSON([]byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, value)
		})
	}
}

func TestDecodeJSONPreservesEmptyListElementType(t *testing.T) {
	value, err := DecodeJSON([]byte(`{"type":"list","value":{"type":"compound","value":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	list := value.(List)
	if list.ElementType != CompoundType || len(list.Values) != 0 {
		t.Fatalf("unexpected empty list: %#v", list)
	}
}

func TestDecodeJSONRejectsAmbiguousAndUnsupportedData(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "untyped number", source: `1`, want: "cannot unmarshal"},
		{name: "unknown field", source: `{"type":"int","value":1,"typo":true}`, want: "unknown field"},
		{name: "unknown type", source: `{"type":"integer","value":1}`, want: "unknown type"},
		{name: "overflow", source: `{"type":"byte","value":128}`, want: "does not fit int8"},
		{name: "private list dialect", source: `{"type":"list","value":[{"type":"int","value":1}]}`, want: "cannot unmarshal array"},
		{name: "short array", source: `{"type":"shortArray","value":[1,2]}`, want: "not a Java Edition network NBT tag"},
		{name: "long scalar", source: `{"type":"long","value":1}`, want: "expected [high, low]"},
		{name: "nonempty end list", source: `{"type":"list","value":{"type":"end","value":[1]}}`, want: "requires an empty list"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeJSON([]byte(test.source))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

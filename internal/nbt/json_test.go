package nbt

import (
	"strings"
	"testing"
)

func TestDecodeJSONPreservesExplicitNBTTypes(t *testing.T) {
	const source = `{
		"type":"compound",
		"value":{
			"natural":{"type":"byte","value":1},
			"height":{"type":"int","value":384},
			"scale":{"type":"float","value":0.1},
			"name":{"type":"string","value":"minecraft:test"},
			"values":{"type":"list","value":[{"type":"long","value":1},{"type":"long","value":2}]}
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
	list := compound["values"].(List)
	if list.ElementType != LongType || len(list.Values) != 2 {
		t.Fatalf("unexpected list: %#v", list)
	}
}

func TestDecodeJSONRejectsAmbiguousAndMalformedData(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "untyped number", source: `1`, want: "cannot unmarshal"},
		{name: "unknown field", source: `{"type":"int","value":1,"typo":true}`, want: "unknown field"},
		{name: "unknown type", source: `{"type":"integer","value":1}`, want: "unknown type"},
		{name: "overflow", source: `{"type":"byte","value":128}`, want: "does not fit int8"},
		{name: "mixed list", source: `{"type":"list","value":[{"type":"int","value":1},{"type":"long","value":2}]}`, want: "heterogeneous list"},
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

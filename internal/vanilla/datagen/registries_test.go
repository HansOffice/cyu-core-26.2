package datagen

import (
	"strings"
	"testing"

	"cyu-core-26.2/internal/registry"
)

func reportID(t *testing.T, raw string) registry.Identifier {
	t.Helper()
	value, err := registry.ParseIdentifier(raw)
	if err != nil {
		t.Fatalf("ParseIdentifier(%q): %v", raw, err)
	}
	return value
}

func TestDecodeRegistriesReportUsesProtocolIDsAsOrder(t *testing.T) {
	const source = `{
		"minecraft:item": {
			"protocol_id": 1,
			"entries": {
				"minecraft:stone": {"protocol_id": 1},
				"minecraft:air": {"protocol_id": 0}
			}
		},
		"minecraft:block": {
			"default": "minecraft:air",
			"protocol_id": 0,
			"entries": {
				"minecraft:stone": {"protocol_id": 1},
				"minecraft:air": {"protocol_id": 0}
			}
		},
		"minecraft:worldgen/world_preset": {
			"entries": {
				"minecraft:flat": {},
				"minecraft:normal": {}
			}
		}
	}`

	report, err := DecodeRegistriesReport(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	values := report.Registries()
	if len(values) != 3 {
		t.Fatalf("registry count = %d, want 3", len(values))
	}
	if values[0].Key != reportID(t, "minecraft:block") || values[1].Key != reportID(t, "minecraft:item") {
		t.Fatalf("indexed order = %s, %s", values[0].Key, values[1].Key)
	}
	if values[2].Key != reportID(t, "minecraft:worldgen/world_preset") || values[2].HasProtocolID {
		t.Fatalf("unindexed registry = %#v", values[2])
	}
	if got := values[0].Entries; len(got) != 2 || got[0].Key != reportID(t, "minecraft:air") || got[1].Key != reportID(t, "minecraft:stone") {
		t.Fatalf("block entry order = %#v", got)
	}
	if values[0].Default == nil || *values[0].Default != reportID(t, "minecraft:air") {
		t.Fatalf("block default = %v", values[0].Default)
	}
}

func TestDecodeRegistriesReportRejectsDuplicateProtocolIDs(t *testing.T) {
	t.Run("registries", func(t *testing.T) {
		const source = `{
			"minecraft:block":{"protocol_id":4,"entries":{}},
			"minecraft:item":{"protocol_id":4,"entries":{}}
		}`
		_, err := DecodeRegistriesReport(strings.NewReader(source))
		if err == nil || !strings.Contains(err.Error(), "duplicate registry protocol ID 4") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("entries", func(t *testing.T) {
		const source = `{
			"minecraft:item":{"protocol_id":1,"entries":{
				"minecraft:air":{"protocol_id":0},
				"minecraft:stone":{"protocol_id":0}
			}}
		}`
		_, err := DecodeRegistriesReport(strings.NewReader(source))
		if err == nil || !strings.Contains(err.Error(), "duplicate entry protocol ID 0") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestDecodeRegistriesReportRejectsInvalidStructure(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "unknown registry field",
			src:  `{"minecraft:item":{"protocol_id":1,"entries":{},"typo":true}}`,
			want: `unknown field "typo"`,
		},
		{
			name: "unknown entry field",
			src:  `{"minecraft:item":{"protocol_id":1,"entries":{"minecraft:air":{"protocol_id":0,"name":"air"}}}}`,
			want: `unknown field "name"`,
		},
		{
			name: "negative protocol id",
			src:  `{"minecraft:item":{"protocol_id":-1,"entries":{}}}`,
			want: "invalid non-negative int32",
		},
		{
			name: "missing default entry",
			src:  `{"minecraft:block":{"default":"minecraft:air","protocol_id":0,"entries":{"minecraft:stone":{"protocol_id":1}}}}`,
			want: "default minecraft:air is not an entry",
		},
		{
			name: "invalid resource location",
			src:  `{"Minecraft:item":{"protocol_id":1,"entries":{}}}`,
			want: "invalid namespace character",
		},
		{
			name: "trailing json",
			src:  `{} {}`,
			want: "multiple JSON values",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeRegistriesReport(strings.NewReader(test.src))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRegistriesReportReturnsDefensiveCopies(t *testing.T) {
	const source = `{"minecraft:block":{"default":"minecraft:air","protocol_id":0,"entries":{"minecraft:air":{"protocol_id":0}}}}`
	report, err := DecodeRegistriesReport(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}

	key := reportID(t, "minecraft:block")
	value, ok := report.Registry(key)
	if !ok {
		t.Fatal("block registry missing")
	}
	value.Entries[0].Key = reportID(t, "minecraft:stone")
	*value.Default = reportID(t, "minecraft:stone")

	again, _ := report.Registry(key)
	if again.Entries[0].Key != reportID(t, "minecraft:air") {
		t.Fatalf("entry mutated through lookup: %#v", again.Entries)
	}
	if again.Default == nil || *again.Default != reportID(t, "minecraft:air") {
		t.Fatalf("default mutated through lookup: %v", again.Default)
	}
}

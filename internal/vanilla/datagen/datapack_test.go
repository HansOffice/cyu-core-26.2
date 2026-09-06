package datagen

import (
	"strings"
	"testing"
)

func TestDecodeDatapackReportCapturesDynamicRegistryCapabilities(t *testing.T) {
	const source = `{
		"others": {
			"function": {"elements":true,"format":"mcfunction","stable":true,"tags":true},
			"structure": {"elements":true,"format":"structure","stable":true,"tags":false}
		},
		"registries": {
			"minecraft:worldgen/world_preset": {"elements":true,"stable":false,"tags":true},
			"minecraft:block": {"elements":false,"stable":true,"tags":true},
			"minecraft:damage_type": {"elements":true,"stable":false,"tags":true}
		}
	}`

	report, err := DecodeDatapackReport(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	values := report.Registries()
	if len(values) != 3 {
		t.Fatalf("registry count = %d, want 3", len(values))
	}
	if values[0].Key != reportID(t, "minecraft:block") || values[1].Key != reportID(t, "minecraft:damage_type") || values[2].Key != reportID(t, "minecraft:worldgen/world_preset") {
		t.Fatalf("registry order = %#v", values)
	}

	for _, raw := range []string{"minecraft:damage_type", "minecraft:worldgen/world_preset"} {
		value, ok := report.Registry(reportID(t, raw))
		if !ok {
			t.Fatalf("registry %s missing", raw)
		}
		if !value.Elements || value.Stable || !value.Tags {
			t.Fatalf("registry %s capabilities = %#v", raw, value)
		}
	}
	block, ok := report.Registry(reportID(t, "minecraft:block"))
	if !ok || block.Elements || !block.Stable || !block.Tags {
		t.Fatalf("block capabilities = %#v, ok=%v", block, ok)
	}

	dynamic := report.ElementRegistries()
	if len(dynamic) != 2 || dynamic[0].Key != reportID(t, "minecraft:damage_type") || dynamic[1].Key != reportID(t, "minecraft:worldgen/world_preset") {
		t.Fatalf("element registries = %#v", dynamic)
	}
}

func TestDecodeDatapackReportRejectsSchemaDrift(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "missing others",
			src:  `{"registries":{}}`,
			want: "missing others",
		},
		{
			name: "missing registries",
			src:  `{"others":{}}`,
			want: "missing registries",
		},
		{
			name: "unknown root field",
			src:  `{"others":{},"registries":{},"typo":true}`,
			want: "unknown field",
		},
		{
			name: "missing capability",
			src:  `{"others":{},"registries":{"minecraft:damage_type":{"elements":true,"stable":false}}}`,
			want: "missing tags",
		},
		{
			name: "wrong capability type",
			src:  `{"others":{},"registries":{"minecraft:damage_type":{"elements":"yes","stable":false,"tags":true}}}`,
			want: "elements:",
		},
		{
			name: "unknown registry field",
			src:  `{"others":{},"registries":{"minecraft:damage_type":{"elements":true,"stable":false,"tags":true,"format":"json"}}}`,
			want: `unknown field "format"`,
		},
		{
			name: "unknown other field",
			src:  `{"others":{"function":{"elements":true,"stable":true,"tags":true,"format":"mcfunction","typo":1}},"registries":{}}`,
			want: `unknown field "typo"`,
		},
		{
			name: "empty other format",
			src:  `{"others":{"function":{"elements":true,"stable":true,"tags":true,"format":""}},"registries":{}}`,
			want: "format is empty",
		},
		{
			name: "invalid identifier",
			src:  `{"others":{},"registries":{"Minecraft:damage_type":{"elements":true,"stable":false,"tags":true}}}`,
			want: "invalid namespace character",
		},
		{
			name: "trailing json",
			src:  `{"others":{},"registries":{}} {}`,
			want: "multiple JSON values",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeDatapackReport(strings.NewReader(test.src))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestDatapackReportReturnsDefensiveSlices(t *testing.T) {
	const source = `{"others":{},"registries":{"minecraft:damage_type":{"elements":true,"stable":false,"tags":true}}}`
	report, err := DecodeDatapackReport(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}

	values := report.Registries()
	values[0].Key = reportID(t, "minecraft:block")
	again, ok := report.Registry(reportID(t, "minecraft:damage_type"))
	if !ok || again.Key != reportID(t, "minecraft:damage_type") {
		t.Fatalf("report mutated through slice: %#v, ok=%v", again, ok)
	}
}

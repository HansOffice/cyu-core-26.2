package registry

import (
	"errors"
	"strings"
	"testing"
)

func id(t *testing.T, raw string) Identifier {
	t.Helper()
	identifier, err := ParseIdentifier(raw)
	if err != nil {
		t.Fatalf("ParseIdentifier(%q): %v", raw, err)
	}
	return identifier
}

func TestParseIdentifier(t *testing.T) {
	valid := []string{
		"minecraft:damage_type",
		"minecraft:worldgen/world_preset",
		"cyucore:plugin/example.value",
	}
	for _, raw := range valid {
		if _, err := ParseIdentifier(raw); err != nil {
			t.Fatalf("expected %q to be valid: %v", raw, err)
		}
	}

	invalid := []string{"", "stone", ":stone", "minecraft:", "Minecraft:stone", "minecraft:bad path", "a:b:c"}
	for _, raw := range invalid {
		if _, err := ParseIdentifier(raw); err == nil {
			t.Fatalf("expected %q to be invalid", raw)
		}
	}
}

func TestDecodeDatasetBuildsStableRegistryIDsAndResolvedTags(t *testing.T) {
	const source = `{
		"registries": [{
			"key": "minecraft:damage_type",
			"entries": [
				{"key": "minecraft:in_fire", "data": {"exhaustion": 0.1}},
				{"key": "minecraft:lava", "data": {"exhaustion": 0.2}}
			]
		}],
		"tags": [{
			"registry": "minecraft:damage_type",
			"tags": [{"key": "minecraft:is_fire", "entries": ["minecraft:in_fire", "minecraft:lava"]}]
		}]
	}`

	set, err := DecodeDataset(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	registry, ok := set.Registry(id(t, "minecraft:damage_type"))
	if !ok {
		t.Fatal("damage type registry missing")
	}
	if got, ok := registry.EntryID(id(t, "minecraft:in_fire")); !ok || got != 0 {
		t.Fatalf("in_fire ID: got %d, ok=%v", got, ok)
	}
	if got, ok := registry.EntryID(id(t, "minecraft:lava")); !ok || got != 1 {
		t.Fatalf("lava ID: got %d, ok=%v", got, ok)
	}

	group, ok := set.Tags(id(t, "minecraft:damage_type"))
	if !ok || len(group.Tags) != 1 || group.Tags[0].Key != id(t, "minecraft:is_fire") {
		t.Fatalf("unexpected tag group: %#v", group)
	}
}

func TestDecodeDatasetRejectsTagReferenceToMissingEntry(t *testing.T) {
	const source = `{
		"registries": [{"key":"minecraft:damage_type","entries":[{"key":"minecraft:lava"}]}],
		"tags": [{"registry":"minecraft:damage_type","tags":[{"key":"minecraft:is_fire","entries":["minecraft:in_fire"]}]}]
	}`
	_, err := DecodeDataset(strings.NewReader(source))
	if err == nil || !strings.Contains(err.Error(), "missing entry minecraft:in_fire") {
		t.Fatalf("expected missing tag entry error, got %v", err)
	}
}

func TestRequirementsReportKnownConfigurationCrashClasses(t *testing.T) {
	damageRegistry, err := NewRegistry(id(t, "minecraft:damage_type"), []Entry{{Key: id(t, "minecraft:lava")}})
	if err != nil {
		t.Fatal(err)
	}
	set, err := NewSet([]*Registry{damageRegistry}, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = set.ValidateRequirements(Requirements{
		Registries: []Identifier{id(t, "minecraft:worldgen/world_preset")},
		Tags: map[Identifier][]Identifier{
			id(t, "minecraft:damage_type"): {id(t, "minecraft:is_fire")},
		},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	message := validation.Error()
	if !strings.Contains(message, "missing registry minecraft:worldgen/world_preset") {
		t.Fatalf("world preset regression not reported: %s", message)
	}
	if !strings.Contains(message, "missing tag minecraft:damage_type/minecraft:is_fire") {
		t.Fatalf("damage tag regression not reported: %s", message)
	}
}

func TestDatasetRejectsDuplicatesAndUnknownFields(t *testing.T) {
	t.Run("duplicate registry", func(t *testing.T) {
		const source = `{"registries":[{"key":"minecraft:damage_type"},{"key":"minecraft:damage_type"}],"tags":[]}`
		_, err := DecodeDataset(strings.NewReader(source))
		if err == nil || !strings.Contains(err.Error(), "duplicate registry") {
			t.Fatalf("expected duplicate registry error, got %v", err)
		}
	})

	t.Run("duplicate entry", func(t *testing.T) {
		const source = `{"registries":[{"key":"minecraft:damage_type","entries":[{"key":"minecraft:lava"},{"key":"minecraft:lava"}]}],"tags":[]}`
		_, err := DecodeDataset(strings.NewReader(source))
		if err == nil || !strings.Contains(err.Error(), "duplicate entry") {
			t.Fatalf("expected duplicate entry error, got %v", err)
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		const source = `{"registries":[],"tags":[],"typo":true}`
		_, err := DecodeDataset(strings.NewReader(source))
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("expected unknown field error, got %v", err)
		}
	})
}

func TestRegistryReturnsDefensiveDataCopies(t *testing.T) {
	key := id(t, "minecraft:test")
	entryKey := id(t, "minecraft:value")
	registry, err := NewRegistry(key, []Entry{{Key: entryKey, Data: []byte(`{"value":1}`)}})
	if err != nil {
		t.Fatal(err)
	}

	entry, _, ok := registry.Lookup(entryKey)
	if !ok {
		t.Fatal("entry missing")
	}
	entry.Data[0] = 'x'
	again, _, _ := registry.Lookup(entryKey)
	if string(again.Data) != `{"value":1}` {
		t.Fatalf("registry data was mutated through lookup: %s", again.Data)
	}
}

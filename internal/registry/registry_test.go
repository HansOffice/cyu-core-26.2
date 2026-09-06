package registry

import (
	"errors"
	"strings"
	"testing"

	"cyu-core-26.2/internal/nbt"
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

func TestDecodeDatasetBuildsStableRegistryIDsAndTypedNBT(t *testing.T) {
	const source = `{
		"registries": [{
			"key": "minecraft:damage_type",
			"entries": [
				{"key": "minecraft:in_fire", "data": {"type":"compound","value":{"exhaustion":{"type":"float","value":0.1}}}},
				{"key": "minecraft:lava", "data": {"type":"compound","value":{"exhaustion":{"type":"float","value":0.2}}}}
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
	entry, _, _ := registry.Lookup(id(t, "minecraft:in_fire"))
	compound, ok := entry.Data.(nbt.Compound)
	if !ok {
		t.Fatalf("entry data type = %T, want nbt.Compound", entry.Data)
	}
	if _, ok := compound["exhaustion"].(nbt.Float); !ok {
		t.Fatalf("exhaustion type = %T, want nbt.Float", compound["exhaustion"])
	}

	group, ok := set.Tags(id(t, "minecraft:damage_type"))
	if !ok || len(group.Tags) != 1 || group.Tags[0].Key != id(t, "minecraft:is_fire") {
		t.Fatalf("unexpected tag group: %#v", group)
	}
}

func TestDecodeDatasetRejectsAmbiguousUntypedNBT(t *testing.T) {
	const source = `{
		"registries":[{"key":"minecraft:damage_type","entries":[{"key":"minecraft:lava","data":{"exhaustion":0.1}}]}],
		"tags":[]
	}`
	_, err := DecodeDataset(strings.NewReader(source))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected typed-NBT schema error, got %v", err)
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

func TestRegistryReturnsDefensiveNBTCopies(t *testing.T) {
	key := id(t, "minecraft:test")
	entryKey := id(t, "minecraft:value")
	registry, err := NewRegistry(key, []Entry{{Key: entryKey, Data: nbt.Compound{"value": nbt.Int(1)}}})
	if err != nil {
		t.Fatal(err)
	}

	entry, _, ok := registry.Lookup(entryKey)
	if !ok {
		t.Fatal("entry missing")
	}
	entry.Data.(nbt.Compound)["value"] = nbt.Int(2)
	again, _, _ := registry.Lookup(entryKey)
	if got := again.Data.(nbt.Compound)["value"].(nbt.Int); got != 1 {
		t.Fatalf("registry NBT was mutated through lookup: %d", got)
	}
}

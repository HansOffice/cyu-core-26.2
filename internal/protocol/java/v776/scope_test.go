package v776

import (
	"strings"
	"testing"

	"cyu-core-26.2/internal/registry"
)

func TestSynchronizedRegistryKeysMatch26_2Closure(t *testing.T) {
	keys := SynchronizedRegistryKeys()
	if got, want := len(keys), 29; got != want {
		t.Fatalf("synchronized registry count = %d, want %d", got, want)
	}
	if keys[0] != registry.Identifier("minecraft:worldgen/biome") {
		t.Fatalf("first synchronized registry = %s", keys[0])
	}
	if keys[len(keys)-1] != registry.Identifier("minecraft:timeline") {
		t.Fatalf("last synchronized registry = %s", keys[len(keys)-1])
	}
	if IsSynchronizedRegistry(registry.Identifier("minecraft:worldgen/world_preset")) {
		t.Fatal("world_preset is a WORLDGEN registry, not a v776 synchronized RegistryData registry")
	}
	if !IsSynchronizedRegistry(registry.Identifier("minecraft:damage_type")) {
		t.Fatal("damage_type must be synchronized")
	}

	keys[0] = registry.Identifier("minecraft:mutated")
	if SynchronizedRegistryKeys()[0] != registry.Identifier("minecraft:worldgen/biome") {
		t.Fatal("SynchronizedRegistryKeys exposed mutable internal storage")
	}
}

func TestRegistryDataRegistriesRejectsIncompleteSet(t *testing.T) {
	set, err := registry.NewSet(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RegistryDataRegistries(set)
	if err == nil || !strings.Contains(err.Error(), "minecraft:worldgen/biome") {
		t.Fatalf("expected first missing synchronized registry, got %v", err)
	}
}

func TestValidateConfigurationSetRequiresDamageFireTag(t *testing.T) {
	registries := make([]*registry.Registry, 0, len(synchronizedRegistryKeys))
	for _, key := range synchronizedRegistryKeys {
		entries := []registry.Entry(nil)
		if key == registry.Identifier("minecraft:damage_type") {
			entries = []registry.Entry{{Key: registry.Identifier("minecraft:lava")}}
		}
		reg, err := registry.NewRegistry(key, entries)
		if err != nil {
			t.Fatal(err)
		}
		registries = append(registries, reg)
	}
	set, err := registry.NewSet(registries, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateConfigurationSet(set); err == nil || !strings.Contains(err.Error(), "minecraft:is_fire") {
		t.Fatalf("expected damage fire tag regression, got %v", err)
	}

	set, err = registry.NewSet(registries, []registry.TagGroup{{
		Registry: registry.Identifier("minecraft:damage_type"),
		Tags: []registry.Tag{{
			Key:     registry.Identifier("minecraft:is_fire"),
			Entries: []registry.Identifier{registry.Identifier("minecraft:lava")},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateConfigurationSet(set); err != nil {
		t.Fatalf("valid configuration set rejected: %v", err)
	}
}

package v776

import (
	"fmt"

	"cyu-core-26.2/internal/registry"
)

// synchronizedRegistryKeys is RegistryDataLoader.SYNCHRONIZED_REGISTRIES for
// Minecraft 26.2, in the exact order used by the vanilla server when emitting
// Configuration RegistryData packets.
var synchronizedRegistryKeys = [...]registry.Identifier{
	"minecraft:worldgen/biome",
	"minecraft:chat_type",
	"minecraft:trim_pattern",
	"minecraft:trim_material",
	"minecraft:wolf_variant",
	"minecraft:wolf_sound_variant",
	"minecraft:pig_variant",
	"minecraft:pig_sound_variant",
	"minecraft:frog_variant",
	"minecraft:cat_variant",
	"minecraft:cat_sound_variant",
	"minecraft:cow_sound_variant",
	"minecraft:cow_variant",
	"minecraft:chicken_sound_variant",
	"minecraft:chicken_variant",
	"minecraft:zombie_nautilus_variant",
	"minecraft:painting_variant",
	"minecraft:sulfur_cube_archetype",
	"minecraft:dimension_type",
	"minecraft:damage_type",
	"minecraft:banner_pattern",
	"minecraft:enchantment",
	"minecraft:jukebox_song",
	"minecraft:instrument",
	"minecraft:test_environment",
	"minecraft:test_instance",
	"minecraft:dialog",
	"minecraft:world_clock",
	"minecraft:timeline",
}

var synchronizedRegistrySet = func() map[registry.Identifier]struct{} {
	set := make(map[registry.Identifier]struct{}, len(synchronizedRegistryKeys))
	for _, key := range synchronizedRegistryKeys {
		set[key] = struct{}{}
	}
	return set
}()

// SynchronizedRegistryKeys returns the authoritative protocol-776 RegistryData
// registry order. The returned slice is a defensive copy.
func SynchronizedRegistryKeys() []registry.Identifier {
	keys := make([]registry.Identifier, len(synchronizedRegistryKeys))
	copy(keys, synchronizedRegistryKeys[:])
	return keys
}

// IsSynchronizedRegistry reports whether the registry is carried by the
// Configuration RegistryData stream. This is deliberately narrower than the
// set of data-pack-backed WORLDGEN registries.
func IsSynchronizedRegistry(key registry.Identifier) bool {
	_, ok := synchronizedRegistrySet[key]
	return ok
}

// RegistryDataRegistries resolves the exact v776 RegistryData sequence from a
// structured registry set. Missing entries fail before any partial
// Configuration sequence can be sent.
func RegistryDataRegistries(set *registry.Set) ([]*registry.Registry, error) {
	if set == nil {
		return nil, fmt.Errorf("v776: nil registry set")
	}
	registries := make([]*registry.Registry, 0, len(synchronizedRegistryKeys))
	for _, key := range synchronizedRegistryKeys {
		reg, ok := set.Registry(key)
		if !ok {
			return nil, fmt.Errorf("v776: missing synchronized registry %s", key)
		}
		registries = append(registries, reg)
	}
	return registries, nil
}

// ValidateConfigurationSet checks v776-specific invariants that are not part
// of the generic registry graph. In particular, damage_type/is_fire is a
// network-safe tag regression anchor, while worldgen/world_preset is explicitly
// not synchronized and must not be treated as a RegistryData requirement.
func ValidateConfigurationSet(set *registry.Set) error {
	if _, err := RegistryDataRegistries(set); err != nil {
		return err
	}

	damageType := registry.Identifier("minecraft:damage_type")
	isFire := registry.Identifier("minecraft:is_fire")
	group, ok := set.Tags(damageType)
	if !ok {
		return fmt.Errorf("v776: missing tag %s/%s", damageType, isFire)
	}
	for _, tag := range group.Tags {
		if tag.Key == isFire {
			return nil
		}
	}
	return fmt.Errorf("v776: missing tag %s/%s", damageType, isFire)
}

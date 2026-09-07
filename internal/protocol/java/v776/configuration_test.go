package v776

import (
	"bytes"
	"testing"

	"cyu-core-26.2/internal/nbt"
	"cyu-core-26.2/internal/protocol"
	"cyu-core-26.2/internal/registry"
)

func TestEncodeRegistryDataPreservesOrderAndBooleanOptionalNBT(t *testing.T) {
	reg, err := registry.NewRegistry(mustID(t, "minecraft:test"), []registry.Entry{
		{Key: mustID(t, "minecraft:empty")},
		{Key: mustID(t, "minecraft:value"), Data: nbt.Compound{"x": nbt.Int(1)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := EncodeRegistryData(reg)
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.AppendString(nil, "minecraft:test")
	want = protocol.AppendVarInt(want, 2)
	want = protocol.AppendString(want, "minecraft:empty")
	want = append(want, 0x00) // Optional.empty()
	want = protocol.AppendString(want, "minecraft:value")
	want = append(want, 0x01) // Optional.of(...)
	want = append(want, 0x0a, 0x03, 0x00, 0x01, 'x', 0x00, 0x00, 0x00, 0x01, 0x00)
	if !bytes.Equal(got, want) {
		t.Fatalf("registry payload mismatch:\nwant %x\n got %x", want, got)
	}
}

func TestEncodeUpdateTagsUsesRegistryRuntimeIDs(t *testing.T) {
	reg, err := registry.NewRegistry(mustID(t, "minecraft:damage_type"), []registry.Entry{
		{Key: mustID(t, "minecraft:in_fire")},
		{Key: mustID(t, "minecraft:lava")},
	})
	if err != nil {
		t.Fatal(err)
	}
	untagged, err := registry.NewRegistry(mustID(t, "minecraft:worldgen/world_preset"), nil)
	if err != nil {
		t.Fatal(err)
	}
	set, err := registry.NewSet([]*registry.Registry{reg, untagged}, []registry.TagGroup{{
		Registry: reg.Key(),
		Tags: []registry.Tag{{
			Key:     mustID(t, "minecraft:is_fire"),
			Entries: []registry.Identifier{mustID(t, "minecraft:in_fire"), mustID(t, "minecraft:lava")},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	got, err := EncodeUpdateTags(set)
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.AppendVarInt(nil, 1)
	want = protocol.AppendString(want, "minecraft:damage_type")
	want = protocol.AppendVarInt(want, 1)
	want = protocol.AppendString(want, "minecraft:is_fire")
	want = protocol.AppendVarInt(want, 2)
	want = protocol.AppendVarInt(want, 0)
	want = protocol.AppendVarInt(want, 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("tags payload mismatch:\nwant %x\n got %x", want, got)
	}
}

func TestConfigurationEncodersRejectNilInputs(t *testing.T) {
	if _, err := EncodeRegistryData(nil); err == nil {
		t.Fatal("expected nil registry error")
	}
	if _, err := EncodeUpdateTags(nil); err == nil {
		t.Fatal("expected nil set error")
	}
}

func mustID(t *testing.T, raw string) registry.Identifier {
	t.Helper()
	id, err := registry.ParseIdentifier(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

package capture

import (
	"bytes"
	"strings"
	"testing"

	"cyu-core-26.2/internal/nbt"
	"cyu-core-26.2/internal/protocol"
	"cyu-core-26.2/internal/registry"
	"cyu-core-26.2/internal/vanilla/datagen"
)

func captureID(t *testing.T, raw string) registry.Identifier {
	t.Helper()
	id, err := registry.ParseIdentifier(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDecodeRegistryDataRequiresFullContents(t *testing.T) {
	payload := protocol.AppendString(nil, "minecraft:damage_type")
	payload = protocol.AppendVarInt(payload, 2)
	payload = protocol.AppendString(payload, "minecraft:in_fire")
	payload = append(payload, 1)
	var nbtBytes bytes.Buffer
	if err := nbt.WriteNetwork(&nbtBytes, nbt.Compound{"exhaustion": nbt.Float(0.1)}); err != nil {
		t.Fatal(err)
	}
	payload = append(payload, nbtBytes.Bytes()...)
	payload = protocol.AppendString(payload, "minecraft:lava")
	payload = append(payload, 0)

	if _, err := DecodeRegistryData(payload, true); err == nil || !strings.Contains(err.Error(), "omitted NBT") {
		t.Fatalf("expected omitted-data error, got %v", err)
	}
	reg, err := DecodeRegistryData(payload, false)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Len() != 2 {
		t.Fatalf("registry length = %d", reg.Len())
	}
	entry, id, ok := reg.Lookup(captureID(t, "minecraft:in_fire"))
	if !ok || id != 0 {
		t.Fatalf("in_fire lookup = %#v, %d, %v", entry, id, ok)
	}
	if _, ok := entry.Data.(nbt.Compound); !ok {
		t.Fatalf("in_fire NBT type = %T", entry.Data)
	}
}

func TestDecodeUpdateTagsRejectsTrailingAndNegativeIDs(t *testing.T) {
	payload := protocol.AppendVarInt(nil, 1)
	payload = protocol.AppendString(payload, "minecraft:damage_type")
	payload = protocol.AppendVarInt(payload, 1)
	payload = protocol.AppendString(payload, "minecraft:is_fire")
	payload = protocol.AppendVarInt(payload, 1)
	payload = protocol.AppendVarInt(payload, -1)
	if _, err := DecodeUpdateTags(payload); err == nil || !strings.Contains(err.Error(), "negative registry ID") {
		t.Fatalf("expected negative ID error, got %v", err)
	}

	valid := protocol.AppendVarInt(nil, 1)
	valid = protocol.AppendString(valid, "minecraft:damage_type")
	valid = protocol.AppendVarInt(valid, 1)
	valid = protocol.AppendString(valid, "minecraft:is_fire")
	valid = protocol.AppendVarInt(valid, 1)
	valid = protocol.AppendVarInt(valid, 0)
	valid = append(valid, 0xff)
	if _, err := DecodeUpdateTags(valid); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("expected trailing-data error, got %v", err)
	}
}

func TestBuildDatasetResolvesSynchronizedAndStaticTagIDs(t *testing.T) {
	damage, err := registry.NewRegistry(captureID(t, "minecraft:damage_type"), []registry.Entry{
		{Key: captureID(t, "minecraft:in_fire"), Data: nbt.Compound{"exhaustion": nbt.Float(0.1)}},
		{Key: captureID(t, "minecraft:lava"), Data: nbt.Compound{"exhaustion": nbt.Float(0.2)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	const reportJSON = `{
		"minecraft:block": {
			"protocol_id": 0,
			"entries": {
				"minecraft:air": {"protocol_id": 0},
				"minecraft:stone": {"protocol_id": 1}
			}
		}
	}`
	report, err := datagen.DecodeRegistriesReport(strings.NewReader(reportJSON))
	if err != nil {
		t.Fatal(err)
	}
	groups := []RawTagGroup{
		{
			Registry: captureID(t, "minecraft:block"),
			Tags:     []RawTag{{Key: captureID(t, "minecraft:mineable/pickaxe"), EntryIDs: []int32{1}}},
		},
		{
			Registry: captureID(t, "minecraft:damage_type"),
			Tags:     []RawTag{{Key: captureID(t, "minecraft:is_fire"), EntryIDs: []int32{0, 1}}},
		},
	}

	dataset, err := BuildDataset([]*registry.Registry{damage}, groups, report)
	if err != nil {
		t.Fatal(err)
	}
	encoded, set, err := MarshalDataset(dataset)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"minecraft:stone"`)) {
		t.Fatalf("static registry entries missing from dataset: %s", encoded)
	}

	blockGroup, ok := set.Tags(captureID(t, "minecraft:block"))
	if !ok || len(blockGroup.Tags) != 1 || len(blockGroup.Tags[0].Entries) != 1 || blockGroup.Tags[0].Entries[0] != captureID(t, "minecraft:stone") {
		t.Fatalf("unexpected resolved block tags: %#v", blockGroup)
	}
	damageGroup, ok := set.Tags(captureID(t, "minecraft:damage_type"))
	if !ok || len(damageGroup.Tags) != 1 || len(damageGroup.Tags[0].Entries) != 2 {
		t.Fatalf("unexpected resolved damage tags: %#v", damageGroup)
	}
}

func TestBuildDatasetRejectsNonStaticTagRegistry(t *testing.T) {
	const reportJSON = `{
		"minecraft:worldgen/world_preset": {
			"entries": {"minecraft:normal": {}}
		}
	}`
	report, err := datagen.DecodeRegistriesReport(strings.NewReader(reportJSON))
	if err != nil {
		t.Fatal(err)
	}
	_, err = BuildDataset(nil, []RawTagGroup{{
		Registry: captureID(t, "minecraft:worldgen/world_preset"),
		Tags:     []RawTag{{Key: captureID(t, "minecraft:normal"), EntryIDs: []int32{0}}},
	}}, report)
	if err == nil || !strings.Contains(err.Error(), "has no static protocol ID") {
		t.Fatalf("expected non-static tag registry rejection, got %v", err)
	}
}

func TestValidateRegistrySequence(t *testing.T) {
	one, err := registry.NewRegistry(captureID(t, "minecraft:a"), nil)
	if err != nil {
		t.Fatal(err)
	}
	two, err := registry.NewRegistry(captureID(t, "minecraft:b"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRegistrySequence([]*registry.Registry{one, two}, []registry.Identifier{captureID(t, "minecraft:a"), captureID(t, "minecraft:b")}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRegistrySequence([]*registry.Registry{two, one}, []registry.Identifier{captureID(t, "minecraft:a"), captureID(t, "minecraft:b")}); err == nil {
		t.Fatal("expected sequence mismatch")
	}
}

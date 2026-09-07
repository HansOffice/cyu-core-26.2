package capture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"cyu-core-26.2/internal/nbt"
	"cyu-core-26.2/internal/protocol"
	"cyu-core-26.2/internal/registry"
	"cyu-core-26.2/internal/vanilla/datagen"
)

const maxCollectionEntries = 1 << 20

// RawTag is one tag from a Configuration UpdateTags payload before numeric
// registry IDs are resolved back to resource identifiers.
type RawTag struct {
	Key      registry.Identifier
	EntryIDs []int32
}

// RawTagGroup is one registry's tag payload from Configuration UpdateTags.
type RawTagGroup struct {
	Registry registry.Identifier
	Tags     []RawTag
}

// DecodeRegistryData decodes one protocol-776 Configuration RegistryData
// payload. When requireData is true every entry must carry its NBT contents,
// which is the expected response after the capture client accepts no known
// vanilla packs.
func DecodeRegistryData(payload []byte, requireData bool) (*registry.Registry, error) {
	buf := bytes.NewReader(payload)
	rawKey, err := protocol.ReadString(buf, protocol.DefaultMaxStringSize)
	if err != nil {
		return nil, fmt.Errorf("capture registry data: registry key: %w", err)
	}
	key, err := registry.ParseIdentifier(rawKey)
	if err != nil {
		return nil, fmt.Errorf("capture registry data: %w", err)
	}
	count, err := readCount(buf, "registry entries")
	if err != nil {
		return nil, fmt.Errorf("capture registry data %s: %w", key, err)
	}
	entries := make([]registry.Entry, 0, count)
	seen := make(map[registry.Identifier]struct{}, count)
	for i := 0; i < count; i++ {
		rawEntryKey, err := protocol.ReadString(buf, protocol.DefaultMaxStringSize)
		if err != nil {
			return nil, fmt.Errorf("capture registry data %s entry %d key: %w", key, i, err)
		}
		entryKey, err := registry.ParseIdentifier(rawEntryKey)
		if err != nil {
			return nil, fmt.Errorf("capture registry data %s entry %d: %w", key, i, err)
		}
		if _, duplicate := seen[entryKey]; duplicate {
			return nil, fmt.Errorf("capture registry data %s: duplicate entry %s", key, entryKey)
		}
		seen[entryKey] = struct{}{}

		present, err := buf.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("capture registry data %s entry %s presence: %w", key, entryKey, err)
		}
		if present != 0 && present != 1 {
			return nil, fmt.Errorf("capture registry data %s entry %s: invalid optional marker 0x%02x", key, entryKey, present)
		}
		var data nbt.Value
		if present == 1 {
			data, err = nbt.ReadNetwork(buf)
			if err != nil {
				return nil, fmt.Errorf("capture registry data %s entry %s NBT: %w", key, entryKey, err)
			}
		} else if requireData {
			return nil, fmt.Errorf("capture registry data %s entry %s omitted NBT despite empty known-pack selection", key, entryKey)
		}
		entries = append(entries, registry.Entry{Key: entryKey, Data: data})
	}
	if buf.Len() != 0 {
		return nil, fmt.Errorf("capture registry data %s: %d trailing bytes", key, buf.Len())
	}
	reg, err := registry.NewRegistry(key, entries)
	if err != nil {
		return nil, fmt.Errorf("capture registry data %s: %w", key, err)
	}
	return reg, nil
}

// DecodeUpdateTags decodes one protocol-776 Configuration UpdateTags payload.
// Numeric entry IDs are intentionally left unresolved until the full captured
// registry closure and the official static registry report are available.
func DecodeUpdateTags(payload []byte) ([]RawTagGroup, error) {
	buf := bytes.NewReader(payload)
	groupCount, err := readCount(buf, "tag groups")
	if err != nil {
		return nil, fmt.Errorf("capture update tags: %w", err)
	}
	groups := make([]RawTagGroup, 0, groupCount)
	seenGroups := make(map[registry.Identifier]struct{}, groupCount)
	for groupIndex := 0; groupIndex < groupCount; groupIndex++ {
		rawRegistryKey, err := protocol.ReadString(buf, protocol.DefaultMaxStringSize)
		if err != nil {
			return nil, fmt.Errorf("capture update tags group %d registry: %w", groupIndex, err)
		}
		registryKey, err := registry.ParseIdentifier(rawRegistryKey)
		if err != nil {
			return nil, fmt.Errorf("capture update tags group %d: %w", groupIndex, err)
		}
		if _, duplicate := seenGroups[registryKey]; duplicate {
			return nil, fmt.Errorf("capture update tags: duplicate registry group %s", registryKey)
		}
		seenGroups[registryKey] = struct{}{}
		tagCount, err := readCount(buf, "tags")
		if err != nil {
			return nil, fmt.Errorf("capture update tags %s: %w", registryKey, err)
		}
		if tagCount == 0 {
			return nil, fmt.Errorf("capture update tags %s: empty tag group should not be serialized by vanilla", registryKey)
		}
		group := RawTagGroup{Registry: registryKey, Tags: make([]RawTag, 0, tagCount)}
		seenTags := make(map[registry.Identifier]struct{}, tagCount)
		for tagIndex := 0; tagIndex < tagCount; tagIndex++ {
			rawTagKey, err := protocol.ReadString(buf, protocol.DefaultMaxStringSize)
			if err != nil {
				return nil, fmt.Errorf("capture update tags %s tag %d key: %w", registryKey, tagIndex, err)
			}
			tagKey, err := registry.ParseIdentifier(rawTagKey)
			if err != nil {
				return nil, fmt.Errorf("capture update tags %s tag %d: %w", registryKey, tagIndex, err)
			}
			if _, duplicate := seenTags[tagKey]; duplicate {
				return nil, fmt.Errorf("capture update tags %s: duplicate tag %s", registryKey, tagKey)
			}
			seenTags[tagKey] = struct{}{}
			entryCount, err := readCount(buf, "tag entries")
			if err != nil {
				return nil, fmt.Errorf("capture update tags %s/%s: %w", registryKey, tagKey, err)
			}
			tag := RawTag{Key: tagKey, EntryIDs: make([]int32, entryCount)}
			for entryIndex := range tag.EntryIDs {
				id, err := protocol.ReadVarInt(buf)
				if err != nil {
					return nil, fmt.Errorf("capture update tags %s/%s entry %d: %w", registryKey, tagKey, entryIndex, err)
				}
				if id < 0 {
					return nil, fmt.Errorf("capture update tags %s/%s entry %d: negative registry ID %d", registryKey, tagKey, entryIndex, id)
				}
				tag.EntryIDs[entryIndex] = id
			}
			group.Tags = append(group.Tags, tag)
		}
		groups = append(groups, group)
	}
	if buf.Len() != 0 {
		return nil, fmt.Errorf("capture update tags: %d trailing bytes", buf.Len())
	}
	return groups, nil
}

// ValidateRegistrySequence verifies the captured RegistryData packet order
// against the exact protocol-version closure before any dataset is emitted.
func ValidateRegistrySequence(registries []*registry.Registry, expected []registry.Identifier) error {
	if len(registries) != len(expected) {
		return fmt.Errorf("capture registry sequence: got %d registries, want %d", len(registries), len(expected))
	}
	for i, key := range expected {
		if registries[i] == nil {
			return fmt.Errorf("capture registry sequence: registry %d is nil", i)
		}
		if got := registries[i].Key(); got != key {
			return fmt.Errorf("capture registry sequence: registry %d = %s, want %s", i, got, key)
		}
	}
	return nil
}

// BuildDataset combines captured synchronized registry contents with the exact
// UpdateTags groups vanilla emitted. Static tag registries resolve their
// numeric IDs through the official Mojang registries report; non-synchronized
// data-pack registries therefore cannot accidentally enter the network-safe
// tag closure because they do not have report-backed static IDs.
func BuildDataset(synchronized []*registry.Registry, rawGroups []RawTagGroup, report *datagen.RegistriesReport) (registry.Dataset, error) {
	if report == nil {
		return registry.Dataset{}, fmt.Errorf("capture dataset: nil registries report")
	}

	dataset := registry.Dataset{}
	synchronizedByKey := make(map[registry.Identifier]*registry.Registry, len(synchronized))
	for index, reg := range synchronized {
		if reg == nil {
			return registry.Dataset{}, fmt.Errorf("capture dataset: synchronized registry %d is nil", index)
		}
		if _, duplicate := synchronizedByKey[reg.Key()]; duplicate {
			return registry.Dataset{}, fmt.Errorf("capture dataset: duplicate synchronized registry %s", reg.Key())
		}
		synchronizedByKey[reg.Key()] = reg
		definition, err := registryDefinitionFromRuntime(reg)
		if err != nil {
			return registry.Dataset{}, err
		}
		dataset.Registries = append(dataset.Registries, definition)
	}

	groupsByKey := make(map[registry.Identifier]RawTagGroup, len(rawGroups))
	staticKeys := make([]registry.Identifier, 0)
	for _, group := range rawGroups {
		if _, duplicate := groupsByKey[group.Registry]; duplicate {
			return registry.Dataset{}, fmt.Errorf("capture dataset: duplicate tag group %s", group.Registry)
		}
		groupsByKey[group.Registry] = cloneRawTagGroup(group)
		if _, synchronized := synchronizedByKey[group.Registry]; !synchronized {
			staticKeys = append(staticKeys, group.Registry)
		}
	}
	sort.Slice(staticKeys, func(i, j int) bool { return staticKeys[i].String() < staticKeys[j].String() })

	staticEntries := make(map[registry.Identifier][]registry.Identifier, len(staticKeys))
	for _, key := range staticKeys {
		reported, ok := report.Registry(key)
		if !ok {
			return registry.Dataset{}, fmt.Errorf("capture dataset: tag registry %s is neither synchronized nor present in Mojang registries report", key)
		}
		entries := make([]registry.Identifier, len(reported.Entries))
		definition := registry.RegistryDefinition{Key: key.String(), Entries: make([]registry.EntryDefinition, len(reported.Entries))}
		for i, entry := range reported.Entries {
			if !entry.HasProtocolID {
				return registry.Dataset{}, fmt.Errorf("capture dataset: tag registry %s entry %s has no static protocol ID", key, entry.Key)
			}
			if entry.ProtocolID != int32(i) {
				return registry.Dataset{}, fmt.Errorf("capture dataset: tag registry %s entry %s protocol ID %d, want contiguous ID %d", key, entry.Key, entry.ProtocolID, i)
			}
			entries[i] = entry.Key
			definition.Entries[i] = registry.EntryDefinition{Key: entry.Key.String()}
		}
		staticEntries[key] = entries
		dataset.Registries = append(dataset.Registries, definition)
	}

	groupKeys := make([]registry.Identifier, 0, len(groupsByKey))
	for key := range groupsByKey {
		groupKeys = append(groupKeys, key)
	}
	sort.Slice(groupKeys, func(i, j int) bool { return groupKeys[i].String() < groupKeys[j].String() })
	for _, key := range groupKeys {
		group := groupsByKey[key]
		tags := append([]RawTag(nil), group.Tags...)
		sort.Slice(tags, func(i, j int) bool { return tags[i].Key.String() < tags[j].Key.String() })
		definition := registry.TagGroupDefinition{Registry: key.String(), Tags: make([]registry.TagDefinition, 0, len(tags))}
		for _, tag := range tags {
			resolved := registry.TagDefinition{Key: tag.Key.String(), Entries: make([]string, len(tag.EntryIDs))}
			for i, id := range tag.EntryIDs {
				entryKey, err := resolveEntryID(key, id, synchronizedByKey, staticEntries)
				if err != nil {
					return registry.Dataset{}, fmt.Errorf("capture dataset: tag %s/%s: %w", key, tag.Key, err)
				}
				resolved.Entries[i] = entryKey.String()
			}
			definition.Tags = append(definition.Tags, resolved)
		}
		dataset.Tags = append(dataset.Tags, definition)
	}
	return dataset, nil
}

// MarshalDataset emits deterministic, human-reviewable JSON and immediately
// decodes it through the runtime loader. A generator cannot produce a dataset
// that only its own in-memory representation understands.
func MarshalDataset(dataset registry.Dataset) ([]byte, *registry.Set, error) {
	encoded, err := json.MarshalIndent(dataset, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("capture dataset: marshal: %w", err)
	}
	encoded = append(encoded, '\n')
	set, err := registry.DecodeDataset(bytes.NewReader(encoded))
	if err != nil {
		return nil, nil, fmt.Errorf("capture dataset: runtime decode: %w", err)
	}
	return encoded, set, nil
}

func registryDefinitionFromRuntime(reg *registry.Registry) (registry.RegistryDefinition, error) {
	definition := registry.RegistryDefinition{Key: reg.Key().String(), Entries: make([]registry.EntryDefinition, 0, reg.Len())}
	for _, entry := range reg.Entries() {
		encoded, err := nbt.EncodeJSON(entry.Data)
		if err != nil {
			return registry.RegistryDefinition{}, fmt.Errorf("capture dataset: registry %s entry %s JSON: %w", reg.Key(), entry.Key, err)
		}
		definition.Entries = append(definition.Entries, registry.EntryDefinition{Key: entry.Key.String(), Data: encoded})
	}
	return definition, nil
}

func resolveEntryID(key registry.Identifier, id int32, synchronized map[registry.Identifier]*registry.Registry, static map[registry.Identifier][]registry.Identifier) (registry.Identifier, error) {
	if reg, ok := synchronized[key]; ok {
		entries := reg.Entries()
		if id < 0 || int(id) >= len(entries) {
			return "", fmt.Errorf("registry ID %d outside synchronized registry size %d", id, len(entries))
		}
		return entries[id].Key, nil
	}
	entries, ok := static[key]
	if !ok {
		return "", fmt.Errorf("no registry ID resolver")
	}
	if id < 0 || int(id) >= len(entries) {
		return "", fmt.Errorf("registry ID %d outside static registry size %d", id, len(entries))
	}
	return entries[id], nil
}

func cloneRawTagGroup(group RawTagGroup) RawTagGroup {
	cloned := RawTagGroup{Registry: group.Registry, Tags: make([]RawTag, len(group.Tags))}
	for i, tag := range group.Tags {
		cloned.Tags[i] = RawTag{Key: tag.Key, EntryIDs: append([]int32(nil), tag.EntryIDs...)}
	}
	return cloned
}

func readCount(r io.Reader, name string) (int, error) {
	count, err := protocol.ReadVarInt(r)
	if err != nil {
		return 0, fmt.Errorf("%s count: %w", name, err)
	}
	if count < 0 || count > maxCollectionEntries {
		return 0, fmt.Errorf("invalid %s count %d", name, count)
	}
	return int(count), nil
}

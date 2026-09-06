package v776

import (
	"fmt"
	"math"

	"cyu-core-26.2/internal/nbt"
	"cyu-core-26.2/internal/protocol"
	"cyu-core-26.2/internal/registry"
)

const (
	ProtocolVersion                        int32 = 776
	ConfigurationClientboundRegistryDataID int32 = 0x07
	ConfigurationClientboundUpdateTagsID   int32 = 0x0d
)

// EncodeRegistryData encodes the payload of one clientbound Configuration
// RegistryData packet. Registry entry order is preserved because it defines
// the numeric IDs later referenced by tags and gameplay packets.
func EncodeRegistryData(reg *registry.Registry) ([]byte, error) {
	if reg == nil {
		return nil, fmt.Errorf("v776: registry data: nil registry")
	}
	if reg.Len() > math.MaxInt32 {
		return nil, fmt.Errorf("v776: registry %s has too many entries: %d", reg.Key(), reg.Len())
	}

	payload := protocol.AppendString(nil, reg.Key().String())
	payload = protocol.AppendVarInt(payload, int32(reg.Len()))
	writer := appendWriter{dst: &payload}
	for _, entry := range reg.Entries() {
		payload = protocol.AppendString(payload, entry.Key.String())
		if err := nbt.WriteOptionalNetwork(writer, entry.Data); err != nil {
			return nil, fmt.Errorf("v776: registry %s entry %s: %w", reg.Key(), entry.Key, err)
		}
	}
	return payload, nil
}

// EncodeUpdateTags encodes the clientbound Configuration UpdateTags payload.
// Tag groups follow registry order and tag entries are translated to the
// runtime IDs derived from that same registry ordering.
func EncodeUpdateTags(set *registry.Set) ([]byte, error) {
	if set == nil {
		return nil, fmt.Errorf("v776: update tags: nil registry set")
	}

	registries := set.Registries()
	groupCount := 0
	for _, reg := range registries {
		if _, ok := set.Tags(reg.Key()); ok {
			groupCount++
		}
	}
	payload, err := appendCount(nil, groupCount, "tag groups")
	if err != nil {
		return nil, err
	}

	for _, reg := range registries {
		group, ok := set.Tags(reg.Key())
		if !ok {
			continue
		}
		payload = protocol.AppendString(payload, group.Registry.String())
		payload, err = appendCount(payload, len(group.Tags), "tags")
		if err != nil {
			return nil, fmt.Errorf("v776: tags %s: %w", group.Registry, err)
		}
		for _, tag := range group.Tags {
			payload = protocol.AppendString(payload, tag.Key.String())
			payload, err = appendCount(payload, len(tag.Entries), "tag entries")
			if err != nil {
				return nil, fmt.Errorf("v776: tag %s/%s: %w", group.Registry, tag.Key, err)
			}
			for _, entryKey := range tag.Entries {
				id, exists := reg.EntryID(entryKey)
				if !exists {
					return nil, fmt.Errorf("v776: tag %s/%s references missing entry %s", group.Registry, tag.Key, entryKey)
				}
				payload = protocol.AppendVarInt(payload, id)
			}
		}
	}
	return payload, nil
}

func appendCount(dst []byte, count int, name string) ([]byte, error) {
	if count < 0 || uint64(count) > math.MaxInt32 {
		return nil, fmt.Errorf("v776: %s count %d exceeds VarInt", name, count)
	}
	return protocol.AppendVarInt(dst, int32(count)), nil
}

type appendWriter struct {
	dst *[]byte
}

func (w appendWriter) Write(data []byte) (int, error) {
	*w.dst = append(*w.dst, data...)
	return len(data), nil
}

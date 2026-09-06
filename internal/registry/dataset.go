package registry

import (
	"encoding/json"
	"fmt"
	"io"
)

// Dataset is the stable structured interchange format used by versioned
// vanilla-data generation. Runtime code consumes Set rather than raw JSON.
type Dataset struct {
	Registries []RegistryDefinition `json:"registries"`
	Tags       []TagGroupDefinition `json:"tags"`
}

type RegistryDefinition struct {
	Key     string            `json:"key"`
	Entries []EntryDefinition `json:"entries"`
}

type EntryDefinition struct {
	Key  string          `json:"key"`
	Data json.RawMessage `json:"data,omitempty"`
}

type TagGroupDefinition struct {
	Registry string          `json:"registry"`
	Tags     []TagDefinition `json:"tags"`
}

type TagDefinition struct {
	Key     string   `json:"key"`
	Entries []string `json:"entries"`
}

func DecodeDataset(r io.Reader) (*Set, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	var dataset Dataset
	if err := decoder.Decode(&dataset); err != nil {
		return nil, fmt.Errorf("registry dataset: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("registry dataset: multiple JSON values")
		}
		return nil, fmt.Errorf("registry dataset: trailing data: %w", err)
	}

	registries := make([]*Registry, 0, len(dataset.Registries))
	for registryIndex, definition := range dataset.Registries {
		key, err := ParseIdentifier(definition.Key)
		if err != nil {
			return nil, fmt.Errorf("registry dataset: registry %d: %w", registryIndex, err)
		}
		entries := make([]Entry, 0, len(definition.Entries))
		for entryIndex, entryDefinition := range definition.Entries {
			entryKey, err := ParseIdentifier(entryDefinition.Key)
			if err != nil {
				return nil, fmt.Errorf("registry dataset: registry %s entry %d: %w", key, entryIndex, err)
			}
			entries = append(entries, Entry{Key: entryKey, Data: entryDefinition.Data})
		}
		registry, err := NewRegistry(key, entries)
		if err != nil {
			return nil, fmt.Errorf("registry dataset: %w", err)
		}
		registries = append(registries, registry)
	}

	groups := make([]TagGroup, 0, len(dataset.Tags))
	for groupIndex, definition := range dataset.Tags {
		registryKey, err := ParseIdentifier(definition.Registry)
		if err != nil {
			return nil, fmt.Errorf("registry dataset: tag group %d: %w", groupIndex, err)
		}
		group := TagGroup{Registry: registryKey, Tags: make([]Tag, 0, len(definition.Tags))}
		for tagIndex, tagDefinition := range definition.Tags {
			tagKey, err := ParseIdentifier(tagDefinition.Key)
			if err != nil {
				return nil, fmt.Errorf("registry dataset: tag group %s tag %d: %w", registryKey, tagIndex, err)
			}
			tag := Tag{Key: tagKey, Entries: make([]Identifier, 0, len(tagDefinition.Entries))}
			for entryIndex, rawEntry := range tagDefinition.Entries {
				entryKey, err := ParseIdentifier(rawEntry)
				if err != nil {
					return nil, fmt.Errorf("registry dataset: tag %s/%s entry %d: %w", registryKey, tagKey, entryIndex, err)
				}
				tag.Entries = append(tag.Entries, entryKey)
			}
			group.Tags = append(group.Tags, tag)
		}
		groups = append(groups, group)
	}

	set, err := NewSet(registries, groups)
	if err != nil {
		return nil, fmt.Errorf("registry dataset: validate: %w", err)
	}
	return set, nil
}

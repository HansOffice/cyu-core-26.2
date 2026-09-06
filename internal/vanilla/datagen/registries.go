package datagen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"

	"cyu-core-26.2/internal/registry"
)

// RegistryEntry is one entry from Mojang's generated reports/registries.json.
// ProtocolID is optional because datapack-backed registries are not guaranteed
// to expose numeric IDs in every report surface.
type RegistryEntry struct {
	Key           registry.Identifier
	ProtocolID    int32
	HasProtocolID bool
}

// Registry describes one registry from Mojang's generated registry report.
// The report is an authority for IDs, not for synchronized RegistryData NBT.
type Registry struct {
	Key           registry.Identifier
	ProtocolID    int32
	HasProtocolID bool
	Default       *registry.Identifier
	Entries       []RegistryEntry
}

// RegistriesReport is an immutable normalized view of reports/registries.json.
// Registry and entry maps in the source JSON are normalized into deterministic
// order without inventing IDs.
type RegistriesReport struct {
	ordered []Registry
	byKey   map[registry.Identifier]int
}

// DecodeRegistriesReport parses Mojang server datagen's reports/registries.json.
// Unknown fields are rejected so upstream schema changes cannot silently alter
// CyuCore's interpretation of authoritative protocol IDs.
func DecodeRegistriesReport(r io.Reader) (*RegistriesReport, error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()

	var root map[string]json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("mojang registries report: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("mojang registries report: multiple JSON values")
		}
		return nil, fmt.Errorf("mojang registries report: trailing data: %w", err)
	}

	registries := make([]Registry, 0, len(root))
	for rawKey, rawRegistry := range root {
		key, err := registry.ParseIdentifier(rawKey)
		if err != nil {
			return nil, fmt.Errorf("mojang registries report: registry key %q: %w", rawKey, err)
		}
		parsed, err := decodeRegistry(key, rawRegistry)
		if err != nil {
			return nil, err
		}
		registries = append(registries, parsed)
	}

	sort.Slice(registries, func(i, j int) bool {
		left, right := registries[i], registries[j]
		if left.HasProtocolID != right.HasProtocolID {
			return left.HasProtocolID
		}
		if left.HasProtocolID && left.ProtocolID != right.ProtocolID {
			return left.ProtocolID < right.ProtocolID
		}
		return left.Key.String() < right.Key.String()
	})

	report := &RegistriesReport{
		ordered: make([]Registry, len(registries)),
		byKey:   make(map[registry.Identifier]int, len(registries)),
	}
	registryIDs := make(map[int32]registry.Identifier)
	for i, value := range registries {
		if value.HasProtocolID {
			if previous, exists := registryIDs[value.ProtocolID]; exists {
				return nil, fmt.Errorf("mojang registries report: duplicate registry protocol ID %d for %s and %s", value.ProtocolID, previous, value.Key)
			}
			registryIDs[value.ProtocolID] = value.Key
		}
		report.ordered[i] = cloneRegistry(value)
		report.byKey[value.Key] = i
	}
	return report, nil
}

func decodeRegistry(key registry.Identifier, raw json.RawMessage) (Registry, error) {
	object, err := decodeObject(raw)
	if err != nil {
		return Registry{}, fmt.Errorf("mojang registries report: registry %s: %w", key, err)
	}
	for field := range object {
		switch field {
		case "default", "protocol_id", "entries":
		default:
			return Registry{}, fmt.Errorf("mojang registries report: registry %s: unknown field %q", key, field)
		}
	}

	value := Registry{Key: key}
	if rawID, exists := object["protocol_id"]; exists {
		id, err := decodeProtocolID(rawID)
		if err != nil {
			return Registry{}, fmt.Errorf("mojang registries report: registry %s protocol_id: %w", key, err)
		}
		value.ProtocolID = id
		value.HasProtocolID = true
	}
	if rawDefault, exists := object["default"]; exists {
		var text string
		if err := json.Unmarshal(rawDefault, &text); err != nil {
			return Registry{}, fmt.Errorf("mojang registries report: registry %s default: %w", key, err)
		}
		identifier, err := registry.ParseIdentifier(text)
		if err != nil {
			return Registry{}, fmt.Errorf("mojang registries report: registry %s default: %w", key, err)
		}
		value.Default = &identifier
	}

	entries := map[string]json.RawMessage{}
	if rawEntries, exists := object["entries"]; exists {
		if err := json.Unmarshal(rawEntries, &entries); err != nil {
			return Registry{}, fmt.Errorf("mojang registries report: registry %s entries: %w", key, err)
		}
	}
	value.Entries = make([]RegistryEntry, 0, len(entries))
	entryIDs := make(map[int32]registry.Identifier)
	entryKeys := make(map[registry.Identifier]struct{}, len(entries))
	for rawEntryKey, rawEntry := range entries {
		entryKey, err := registry.ParseIdentifier(rawEntryKey)
		if err != nil {
			return Registry{}, fmt.Errorf("mojang registries report: registry %s entry key %q: %w", key, rawEntryKey, err)
		}
		if _, exists := entryKeys[entryKey]; exists {
			return Registry{}, fmt.Errorf("mojang registries report: registry %s duplicate entry %s", key, entryKey)
		}
		entryKeys[entryKey] = struct{}{}

		entryObject, err := decodeObject(rawEntry)
		if err != nil {
			return Registry{}, fmt.Errorf("mojang registries report: registry %s entry %s: %w", key, entryKey, err)
		}
		for field := range entryObject {
			if field != "protocol_id" {
				return Registry{}, fmt.Errorf("mojang registries report: registry %s entry %s: unknown field %q", key, entryKey, field)
			}
		}

		entry := RegistryEntry{Key: entryKey}
		if rawID, exists := entryObject["protocol_id"]; exists {
			id, err := decodeProtocolID(rawID)
			if err != nil {
				return Registry{}, fmt.Errorf("mojang registries report: registry %s entry %s protocol_id: %w", key, entryKey, err)
			}
			if previous, exists := entryIDs[id]; exists {
				return Registry{}, fmt.Errorf("mojang registries report: registry %s duplicate entry protocol ID %d for %s and %s", key, id, previous, entryKey)
			}
			entryIDs[id] = entryKey
			entry.ProtocolID = id
			entry.HasProtocolID = true
		}
		value.Entries = append(value.Entries, entry)
	}

	sort.Slice(value.Entries, func(i, j int) bool {
		left, right := value.Entries[i], value.Entries[j]
		if left.HasProtocolID != right.HasProtocolID {
			return left.HasProtocolID
		}
		if left.HasProtocolID && left.ProtocolID != right.ProtocolID {
			return left.ProtocolID < right.ProtocolID
		}
		return left.Key.String() < right.Key.String()
	})

	if value.Default != nil {
		if _, exists := entryKeys[*value.Default]; !exists {
			return Registry{}, fmt.Errorf("mojang registries report: registry %s default %s is not an entry", key, *value.Default)
		}
	}
	return value, nil
}

func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, fmt.Errorf("trailing data: %w", err)
	}
	return object, nil
}

func decodeProtocolID(raw []byte) (int32, error) {
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(number.String(), 10, 32)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid non-negative int32 %q", number.String())
	}
	return int32(value), nil
}

// Registries returns a defensive copy in deterministic protocol-ID/key order.
func (r *RegistriesReport) Registries() []Registry {
	values := make([]Registry, len(r.ordered))
	for i := range r.ordered {
		values[i] = cloneRegistry(r.ordered[i])
	}
	return values
}

// Registry looks up one report registry by resource location.
func (r *RegistriesReport) Registry(key registry.Identifier) (Registry, bool) {
	index, ok := r.byKey[key]
	if !ok {
		return Registry{}, false
	}
	return cloneRegistry(r.ordered[index]), true
}

func cloneRegistry(value Registry) Registry {
	cloned := value
	if value.Default != nil {
		defaultKey := *value.Default
		cloned.Default = &defaultKey
	}
	cloned.Entries = append([]RegistryEntry(nil), value.Entries...)
	return cloned
}

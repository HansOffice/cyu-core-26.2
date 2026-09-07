package datagen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"cyu-core-26.2/internal/registry"
)

// DatapackRegistry describes one registry capability row from Mojang's
// generated reports/datapack.json. Elements means the registry has data-pack
// elements on disk; Tags means it has a tag namespace; Stable reflects Mojang's
// report classification and is kept as source metadata rather than inferred.
type DatapackRegistry struct {
	Key      registry.Identifier
	Elements bool
	Stable   bool
	Tags     bool
}

// DatapackReport is a deterministic, immutable view of the registry capability
// section of reports/datapack.json.
type DatapackReport struct {
	ordered []DatapackRegistry
	byKey   map[registry.Identifier]int
}

type datapackDocument struct {
	Others     map[string]json.RawMessage `json:"others"`
	Registries map[string]json.RawMessage `json:"registries"`
}

type datapackCapabilities struct {
	Elements bool
	Stable   bool
	Tags     bool
}

// DecodeDatapackReport parses Mojang server datagen's reports/datapack.json.
// The exact capability fields are required for every registry so an upstream
// report-shape change fails during data generation instead of silently changing
// CyuCore's notion of dynamic registry coverage.
func DecodeDatapackReport(r io.Reader) (*DatapackReport, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	var document datapackDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("mojang datapack report: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("mojang datapack report: multiple JSON values")
		}
		return nil, fmt.Errorf("mojang datapack report: trailing data: %w", err)
	}
	if document.Others == nil {
		return nil, fmt.Errorf("mojang datapack report: missing others")
	}
	if document.Registries == nil {
		return nil, fmt.Errorf("mojang datapack report: missing registries")
	}

	for key, raw := range document.Others {
		if key == "" {
			return nil, fmt.Errorf("mojang datapack report: empty other resource key")
		}
		if _, err := decodeDatapackCapabilities(raw, true); err != nil {
			return nil, fmt.Errorf("mojang datapack report: other %q: %w", key, err)
		}
	}

	values := make([]DatapackRegistry, 0, len(document.Registries))
	for rawKey, raw := range document.Registries {
		key, err := registry.ParseIdentifier(rawKey)
		if err != nil {
			return nil, fmt.Errorf("mojang datapack report: registry key %q: %w", rawKey, err)
		}
		capabilities, err := decodeDatapackCapabilities(raw, false)
		if err != nil {
			return nil, fmt.Errorf("mojang datapack report: registry %s: %w", key, err)
		}
		values = append(values, DatapackRegistry{
			Key:      key,
			Elements: capabilities.Elements,
			Stable:   capabilities.Stable,
			Tags:     capabilities.Tags,
		})
	}

	sort.Slice(values, func(i, j int) bool {
		return values[i].Key.String() < values[j].Key.String()
	})
	report := &DatapackReport{
		ordered: append([]DatapackRegistry(nil), values...),
		byKey:   make(map[registry.Identifier]int, len(values)),
	}
	for i, value := range values {
		report.byKey[value.Key] = i
	}
	return report, nil
}

func decodeDatapackCapabilities(raw json.RawMessage, allowFormat bool) (datapackCapabilities, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var object map[string]json.RawMessage
	if err := decoder.Decode(&object); err != nil {
		return datapackCapabilities{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return datapackCapabilities{}, fmt.Errorf("multiple JSON values")
		}
		return datapackCapabilities{}, fmt.Errorf("trailing data: %w", err)
	}

	for field := range object {
		switch field {
		case "elements", "stable", "tags":
		case "format":
			if !allowFormat {
				return datapackCapabilities{}, fmt.Errorf("unknown field %q", field)
			}
		default:
			return datapackCapabilities{}, fmt.Errorf("unknown field %q", field)
		}
	}

	var value datapackCapabilities
	for _, field := range []struct {
		name   string
		target *bool
	}{
		{name: "elements", target: &value.Elements},
		{name: "stable", target: &value.Stable},
		{name: "tags", target: &value.Tags},
	} {
		rawValue, exists := object[field.name]
		if !exists {
			return datapackCapabilities{}, fmt.Errorf("missing %s", field.name)
		}
		if err := json.Unmarshal(rawValue, field.target); err != nil {
			return datapackCapabilities{}, fmt.Errorf("%s: %w", field.name, err)
		}
	}
	if rawFormat, exists := object["format"]; exists {
		var format string
		if err := json.Unmarshal(rawFormat, &format); err != nil {
			return datapackCapabilities{}, fmt.Errorf("format: %w", err)
		}
		if format == "" {
			return datapackCapabilities{}, fmt.Errorf("format is empty")
		}
	}
	return value, nil
}

// Registries returns a defensive copy sorted by resource location. The
// datapack report contains no numeric registry ordering, so no protocol order is
// invented here.
func (r *DatapackReport) Registries() []DatapackRegistry {
	return append([]DatapackRegistry(nil), r.ordered...)
}

// Registry returns one capability row by resource location.
func (r *DatapackReport) Registry(key registry.Identifier) (DatapackRegistry, bool) {
	index, ok := r.byKey[key]
	if !ok {
		return DatapackRegistry{}, false
	}
	return r.ordered[index], true
}

// ElementRegistries returns the registries whose elements are data-pack backed.
// It is a capability query only; it does not imply that every returned registry
// is synchronized to clients during Configuration.
func (r *DatapackReport) ElementRegistries() []DatapackRegistry {
	values := make([]DatapackRegistry, 0)
	for _, value := range r.ordered {
		if value.Elements {
			values = append(values, value)
		}
	}
	return values
}

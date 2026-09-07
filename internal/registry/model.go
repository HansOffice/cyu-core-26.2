package registry

import (
	"fmt"
	"sort"
	"strings"

	"cyu-core-26.2/internal/nbt"
)

// Entry is one ordered value in a registry. Its numeric runtime ID is derived
// from its position in Registry, matching the ordered nature of configuration
// registry data instead of duplicating an independently mutable ID field.
type Entry struct {
	Key  Identifier
	Data nbt.Value
}

// Registry is immutable after construction. Entry order is significant.
type Registry struct {
	key     Identifier
	entries []Entry
	ids     map[Identifier]int32
}

func NewRegistry(key Identifier, entries []Entry) (*Registry, error) {
	if err := validateIdentifier(key); err != nil {
		return nil, fmt.Errorf("registry key: %w", err)
	}

	r := &Registry{
		key:     key,
		entries: make([]Entry, 0, len(entries)),
		ids:     make(map[Identifier]int32, len(entries)),
	}
	for index, entry := range entries {
		if err := validateIdentifier(entry.Key); err != nil {
			return nil, fmt.Errorf("registry %s entry %d: %w", key, index, err)
		}
		if _, exists := r.ids[entry.Key]; exists {
			return nil, fmt.Errorf("registry %s: duplicate entry %s", key, entry.Key)
		}
		r.ids[entry.Key] = int32(index)
		r.entries = append(r.entries, cloneEntry(entry))
	}
	return r, nil
}

func (r *Registry) Key() Identifier {
	return r.key
}

func (r *Registry) Len() int {
	return len(r.entries)
}

func (r *Registry) Entries() []Entry {
	entries := make([]Entry, len(r.entries))
	for i := range r.entries {
		entries[i] = cloneEntry(r.entries[i])
	}
	return entries
}

func (r *Registry) Lookup(key Identifier) (Entry, int32, bool) {
	id, ok := r.ids[key]
	if !ok {
		return Entry{}, 0, false
	}
	return cloneEntry(r.entries[id]), id, true
}

func (r *Registry) EntryID(key Identifier) (int32, bool) {
	id, ok := r.ids[key]
	return id, ok
}

func cloneEntry(entry Entry) Entry {
	return Entry{Key: entry.Key, Data: nbt.Clone(entry.Data)}
}

// Tag is a resolved tag. Entries are resource locations, not nested tag
// references; source loaders are responsible for resolving nested source tags
// before constructing a Set.
type Tag struct {
	Key     Identifier
	Entries []Identifier
}

type TagGroup struct {
	Registry Identifier
	Tags     []Tag
}

// Set is an immutable, self-consistent graph of registries and resolved tags.
type Set struct {
	registries    map[Identifier]*Registry
	registryOrder []Identifier
	tagGroups     map[Identifier]TagGroup
}

func NewSet(registries []*Registry, tagGroups []TagGroup) (*Set, error) {
	set := &Set{
		registries:    make(map[Identifier]*Registry, len(registries)),
		registryOrder: make([]Identifier, 0, len(registries)),
		tagGroups:     make(map[Identifier]TagGroup, len(tagGroups)),
	}

	for index, registry := range registries {
		if registry == nil {
			return nil, fmt.Errorf("registry set: registry %d is nil", index)
		}
		key := registry.Key()
		if _, exists := set.registries[key]; exists {
			return nil, fmt.Errorf("registry set: duplicate registry %s", key)
		}
		set.registries[key] = registry
		set.registryOrder = append(set.registryOrder, key)
	}

	for _, group := range tagGroups {
		if err := set.addTagGroup(group); err != nil {
			return nil, err
		}
	}
	return set, nil
}

func (s *Set) addTagGroup(group TagGroup) error {
	if err := validateIdentifier(group.Registry); err != nil {
		return fmt.Errorf("registry tags: %w", err)
	}
	registry, exists := s.registries[group.Registry]
	if !exists {
		return fmt.Errorf("registry tags %s: target registry does not exist", group.Registry)
	}
	if _, exists := s.tagGroups[group.Registry]; exists {
		return fmt.Errorf("registry tags %s: duplicate tag group", group.Registry)
	}

	seenTags := make(map[Identifier]struct{}, len(group.Tags))
	cloned := TagGroup{Registry: group.Registry, Tags: make([]Tag, 0, len(group.Tags))}
	for _, tag := range group.Tags {
		if err := validateIdentifier(tag.Key); err != nil {
			return fmt.Errorf("registry tags %s: %w", group.Registry, err)
		}
		if _, exists := seenTags[tag.Key]; exists {
			return fmt.Errorf("registry tags %s: duplicate tag %s", group.Registry, tag.Key)
		}
		seenTags[tag.Key] = struct{}{}

		seenEntries := make(map[Identifier]struct{}, len(tag.Entries))
		clonedTag := Tag{Key: tag.Key, Entries: make([]Identifier, 0, len(tag.Entries))}
		for _, entryKey := range tag.Entries {
			if _, duplicate := seenEntries[entryKey]; duplicate {
				return fmt.Errorf("registry tags %s/%s: duplicate entry %s", group.Registry, tag.Key, entryKey)
			}
			seenEntries[entryKey] = struct{}{}
			if _, exists := registry.ids[entryKey]; !exists {
				return fmt.Errorf("registry tags %s/%s: missing entry %s", group.Registry, tag.Key, entryKey)
			}
			clonedTag.Entries = append(clonedTag.Entries, entryKey)
		}
		cloned.Tags = append(cloned.Tags, clonedTag)
	}
	s.tagGroups[group.Registry] = cloned
	return nil
}

func (s *Set) Registry(key Identifier) (*Registry, bool) {
	registry, ok := s.registries[key]
	return registry, ok
}

func (s *Set) Registries() []*Registry {
	registries := make([]*Registry, 0, len(s.registryOrder))
	for _, key := range s.registryOrder {
		registries = append(registries, s.registries[key])
	}
	return registries
}

func (s *Set) Tags(registry Identifier) (TagGroup, bool) {
	group, ok := s.tagGroups[registry]
	if !ok {
		return TagGroup{}, false
	}
	return cloneTagGroup(group), true
}

func cloneTagGroup(group TagGroup) TagGroup {
	cloned := TagGroup{Registry: group.Registry, Tags: make([]Tag, len(group.Tags))}
	for i, tag := range group.Tags {
		cloned.Tags[i] = Tag{Key: tag.Key, Entries: append([]Identifier(nil), tag.Entries...)}
	}
	return cloned
}

// Requirements describes version-specific invariants without coupling the
// generic registry model to one Minecraft protocol version.
type Requirements struct {
	Registries []Identifier
	Tags       map[Identifier][]Identifier
}

// ValidationError reports all missing required nodes in one startup error so a
// bad data set can be fixed as a graph rather than one client crash at a time.
type ValidationError struct {
	Issues []string
}

func (e *ValidationError) Error() string {
	issues := append([]string(nil), e.Issues...)
	sort.Strings(issues)
	return "registry requirements not satisfied: " + strings.Join(issues, "; ")
}

func (s *Set) ValidateRequirements(requirements Requirements) error {
	issues := make([]string, 0)
	for _, registryKey := range requirements.Registries {
		if _, exists := s.registries[registryKey]; !exists {
			issues = append(issues, fmt.Sprintf("missing registry %s", registryKey))
		}
	}
	for registryKey, requiredTags := range requirements.Tags {
		group, exists := s.tagGroups[registryKey]
		if !exists {
			for _, tagKey := range requiredTags {
				issues = append(issues, fmt.Sprintf("missing tag %s/%s", registryKey, tagKey))
			}
			continue
		}
		present := make(map[Identifier]struct{}, len(group.Tags))
		for _, tag := range group.Tags {
			present[tag.Key] = struct{}{}
		}
		for _, tagKey := range requiredTags {
			if _, exists := present[tagKey]; !exists {
				issues = append(issues, fmt.Sprintf("missing tag %s/%s", registryKey, tagKey))
			}
		}
	}
	if len(issues) == 0 {
		return nil
	}
	return &ValidationError{Issues: issues}
}

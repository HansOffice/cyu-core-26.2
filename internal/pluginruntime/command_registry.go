package pluginruntime

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	commandapi "cyu-core-26.2/api/command"
)

var (
	ErrCommandRegistrationClosed = errors.New("pluginruntime: command registration is closed")
	ErrCommandConflict           = errors.New("pluginruntime: command literal already registered")
	ErrReservedCommand           = errors.New("pluginruntime: command literal is reserved by host")
)

type commandRecord struct {
	id         uint64
	owner      *entry
	definition commandapi.Definition
	handler    commandapi.Handler
	literals   []commandapi.Name
}

type commandRegistry struct {
	mu       sync.RWMutex
	nextID   uint64
	byName   map[commandapi.Name]*commandRecord
	byID     map[uint64]*commandRecord
	reserved map[commandapi.Name]struct{}
}

func newCommandRegistry() *commandRegistry {
	return &commandRegistry{
		byName:   make(map[commandapi.Name]*commandRecord),
		byID:     make(map[uint64]*commandRecord),
		reserved: make(map[commandapi.Name]struct{}),
	}
}

func (r *commandRegistry) registrar(owner *entry) commandapi.Registrar {
	return scopedCommandRegistrar{registry: r, owner: owner}
}

func (r *commandRegistry) reserve(names ...commandapi.Name) error {
	if r == nil {
		return nil
	}

	validated := make([]commandapi.Name, 0, len(names))
	for _, name := range names {
		parsed, err := commandapi.ParseName(string(name))
		if err != nil {
			return err
		}
		validated = append(validated, parsed)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range validated {
		if _, exists := r.byName[name]; exists {
			return fmt.Errorf("%w: %s is already registered", ErrCommandConflict, name)
		}
	}
	for _, name := range validated {
		r.reserved[name] = struct{}{}
	}
	return nil
}

func (r *commandRegistry) register(owner *entry, definition commandapi.Definition, handler commandapi.Handler) (commandapi.Registration, error) {
	if r == nil || owner == nil || !owner.acceptSubscriptions.Load() {
		return nil, ErrCommandRegistrationClosed
	}
	if err := definition.Validate(); err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, commandapi.ErrNilHandler
	}

	literals := make([]commandapi.Name, 0, 1+len(definition.Aliases))
	literals = append(literals, definition.Name)
	literals = append(literals, definition.Aliases...)

	r.mu.Lock()
	defer r.mu.Unlock()
	if !owner.acceptSubscriptions.Load() {
		return nil, ErrCommandRegistrationClosed
	}
	for _, literal := range literals {
		if _, reserved := r.reserved[literal]; reserved {
			return nil, fmt.Errorf("%w: %s", ErrReservedCommand, literal)
		}
		if current, exists := r.byName[literal]; exists {
			return nil, fmt.Errorf("%w: %s owned by %s", ErrCommandConflict, literal, current.owner.descriptor.ID)
		}
	}

	r.nextID++
	record := &commandRecord{
		id:         r.nextID,
		owner:      owner,
		definition: definition,
		handler:    handler,
		literals:   append([]commandapi.Name(nil), literals...),
	}
	r.byID[record.id] = record
	for _, literal := range record.literals {
		r.byName[literal] = record
	}
	return &commandRegistration{registry: r, id: record.id}, nil
}

func (r *commandRegistry) remove(id uint64) bool {
	if r == nil || id == 0 {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.byID[id]
	if record == nil {
		return false
	}
	delete(r.byID, id)
	for _, literal := range record.literals {
		if r.byName[literal] == record {
			delete(r.byName, literal)
		}
	}
	return true
}

func (r *commandRegistry) removeOwner(owner *entry) int {
	if r == nil || owner == nil {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	removed := 0
	for id, record := range r.byID {
		if record.owner != owner {
			continue
		}
		delete(r.byID, id)
		for _, literal := range record.literals {
			if r.byName[literal] == record {
				delete(r.byName, literal)
			}
		}
		removed++
	}
	return removed
}

func (r *commandRegistry) execute(name commandapi.Name, args []string, source commandapi.Source) (bool, error) {
	if r == nil {
		return false, nil
	}
	parsed, err := commandapi.ParseName(string(name))
	if err != nil {
		return false, nil
	}

	r.mu.RLock()
	record := r.byName[parsed]
	r.mu.RUnlock()
	if record == nil || record.owner == nil || !record.owner.enabled.Load() {
		return false, nil
	}
	if source == nil {
		return true, commandapi.ErrNilSource
	}

	invocation := commandapi.Invocation{
		Name:   record.definition.Name,
		Args:   append([]string(nil), args...),
		Source: source,
	}
	started := time.Now()
	handlerErr := invokeCommandHandler(record, invocation)
	duration := time.Since(started)
	record.owner.recordCommandCall(duration, handlerErr != nil)
	if handlerErr != nil {
		return true, fmt.Errorf("plugin %s command %s: %w", record.owner.descriptor.ID, record.definition.Name, handlerErr)
	}
	return true, nil
}

func invokeCommandHandler(record *commandRecord, invocation commandapi.Invocation) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &PanicError{
				PluginID: record.owner.descriptor.ID,
				Phase:    "command:" + string(record.definition.Name),
				Value:    recovered,
				Stack:    debug.Stack(),
			}
		}
	}()
	return record.handler(invocation)
}

type scopedCommandRegistrar struct {
	registry *commandRegistry
	owner    *entry
}

func (r scopedCommandRegistrar) Register(definition commandapi.Definition, handler commandapi.Handler) (commandapi.Registration, error) {
	return r.registry.register(r.owner, definition, handler)
}

type commandRegistration struct {
	registry     *commandRegistry
	id           uint64
	unregistered atomic.Bool
}

func (r *commandRegistration) Unregister() bool {
	if r == nil || r.unregistered.Swap(true) {
		return false
	}
	return r.registry.remove(r.id)
}

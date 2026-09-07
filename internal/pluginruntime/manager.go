// Package pluginruntime contains CyuCore's host-side plugin lifecycle machinery.
// It depends on the public API, never the other way around.
package pluginruntime

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	pluginapi "cyu-core-26.2/api/plugin"
)

var (
	ErrNilPlugin       = errors.New("pluginruntime: nil plugin")
	ErrDuplicatePlugin = errors.New("pluginruntime: duplicate plugin id")
	ErrManagerActive   = errors.New("pluginruntime: manager is active")
	ErrPluginFailed    = errors.New("pluginruntime: plugin is in failed state")
)

// State describes host lifecycle state. It is implementation state, not part of
// the public plugin ABI.
type State uint8

const (
	StateRegistered State = iota
	StateEnabling
	StateEnabled
	StateDisabling
	StateDisabled
	StateFailed
)

func (s State) String() string {
	switch s {
	case StateRegistered:
		return "registered"
	case StateEnabling:
		return "enabling"
	case StateEnabled:
		return "enabled"
	case StateDisabling:
		return "disabling"
	case StateDisabled:
		return "disabled"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Snapshot is an immutable diagnostic view suitable for status/timing surfaces.
type Snapshot struct {
	Descriptor      pluginapi.Descriptor
	State           State
	EnableDuration  time.Duration
	DisableDuration time.Duration
	LastError       string
}

// LoggerFactory lets the host attach a plugin identity to log records without
// exposing its concrete logging implementation through the public API.
type LoggerFactory func(pluginapi.Descriptor) pluginapi.Logger

type entry struct {
	plugin          pluginapi.Plugin
	descriptor      pluginapi.Descriptor
	context         lifecycleContext
	state           State
	enableDuration  time.Duration
	disableDuration time.Duration
	lastError       string
}

// Manager owns deterministic plugin registration and lifecycle ordering.
// Lifecycle calls are serialized and never run concurrently with each other.
type Manager struct {
	mu            sync.Mutex
	entries       map[pluginapi.ID]*entry
	order         []pluginapi.ID
	loggerFactory LoggerFactory
	active        bool
}

func New(loggerFactory LoggerFactory) *Manager {
	return &Manager{
		entries:       make(map[pluginapi.ID]*entry),
		loggerFactory: loggerFactory,
	}
}

// Register validates and records a plugin without executing plugin code beyond
// Descriptor. Registration order defines enable order and reverse disable order.
func (m *Manager) Register(candidate pluginapi.Plugin) error {
	if m == nil || candidate == nil {
		return ErrNilPlugin
	}

	descriptor, err := readDescriptor(candidate)
	if err != nil {
		return err
	}
	if err := descriptor.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return ErrManagerActive
	}
	if _, exists := m.entries[descriptor.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicatePlugin, descriptor.ID)
	}

	logger := pluginapi.Logger(discardLogger{})
	if m.loggerFactory != nil {
		created, err := callLoggerFactory(m.loggerFactory, descriptor)
		if err != nil {
			return err
		}
		if created != nil {
			logger = created
		}
	}

	m.entries[descriptor.ID] = &entry{
		plugin:     candidate,
		descriptor: descriptor,
		context: lifecycleContext{
			descriptor: descriptor,
			logger:     logger,
		},
		state: StateRegistered,
	}
	m.order = append(m.order, descriptor.ID)
	return nil
}

// EnableAll enables plugins in registration order. Failure is transactional at
// the lifecycle level: the failing plugin gets a cleanup Disable call, then all
// previously enabled plugins are disabled in reverse order.
func (m *Manager) EnableAll() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return ErrManagerActive
	}
	if err := m.validateEnableStates(); err != nil {
		return err
	}

	enabled := make([]*entry, 0, len(m.order))
	for _, id := range m.order {
		current := m.entries[id]
		current.state = StateEnabling
		started := time.Now()
		err := invokeLifecycle(current, "enable", func() error {
			return current.plugin.Enable(current.context)
		})
		current.enableDuration = time.Since(started)
		if err == nil {
			current.state = StateEnabled
			current.lastError = ""
			enabled = append(enabled, current)
			continue
		}

		failure := fmt.Errorf("enable plugin %s: %w", id, err)
		failures := []error{failure}
		cleanupErr := m.disableEntry(current, StateFailed)
		if cleanupErr != nil {
			failures = append(failures, fmt.Errorf("cleanup plugin %s: %w", id, cleanupErr))
		}
		current.lastError = errors.Join(failures...).Error()

		for index := len(enabled) - 1; index >= 0; index-- {
			rollback := enabled[index]
			if rollbackErr := m.disableEntry(rollback, StateDisabled); rollbackErr != nil {
				failures = append(failures, fmt.Errorf("rollback plugin %s: %w", rollback.descriptor.ID, rollbackErr))
			}
		}
		m.active = false
		return errors.Join(failures...)
	}

	m.active = true
	return nil
}

func (m *Manager) validateEnableStates() error {
	for _, id := range m.order {
		current := m.entries[id]
		switch current.state {
		case StateRegistered, StateDisabled:
			continue
		case StateFailed:
			return fmt.Errorf("%w: %s", ErrPluginFailed, id)
		default:
			return fmt.Errorf("pluginruntime: cannot enable %s from state %s", id, current.state)
		}
	}
	return nil
}

// DisableAll disables enabled plugins in reverse registration order. Every
// enabled plugin is given a chance to clean up even when another Disable fails.
func (m *Manager) DisableAll() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var failures []error
	for index := len(m.order) - 1; index >= 0; index-- {
		current := m.entries[m.order[index]]
		if current.state != StateEnabled {
			continue
		}
		if err := m.disableEntry(current, StateDisabled); err != nil {
			failures = append(failures, fmt.Errorf("disable plugin %s: %w", current.descriptor.ID, err))
		}
	}
	m.active = false
	return errors.Join(failures...)
}

func (m *Manager) disableEntry(current *entry, successState State) error {
	current.state = StateDisabling
	started := time.Now()
	err := invokeLifecycle(current, "disable", func() error {
		return current.plugin.Disable(current.context)
	})
	current.disableDuration = time.Since(started)
	if err != nil {
		current.state = StateFailed
		current.lastError = err.Error()
		return err
	}
	current.state = successState
	if successState != StateFailed {
		current.lastError = ""
	}
	return nil
}

func (m *Manager) Active() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

func (m *Manager) Snapshots() []Snapshot {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	snapshots := make([]Snapshot, 0, len(m.order))
	for _, id := range m.order {
		current := m.entries[id]
		snapshots = append(snapshots, Snapshot{
			Descriptor:      current.descriptor,
			State:           current.state,
			EnableDuration:  current.enableDuration,
			DisableDuration: current.disableDuration,
			LastError:       current.lastError,
		})
	}
	return snapshots
}

type lifecycleContext struct {
	descriptor pluginapi.Descriptor
	logger     pluginapi.Logger
}

func (c lifecycleContext) Descriptor() pluginapi.Descriptor { return c.descriptor }
func (c lifecycleContext) Logger() pluginapi.Logger         { return c.logger }

type discardLogger struct{}

func (discardLogger) Debug(string) {}
func (discardLogger) Info(string)  {}
func (discardLogger) Warn(string)  {}
func (discardLogger) Error(string) {}

func readDescriptor(candidate pluginapi.Plugin) (descriptor pluginapi.Descriptor, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &PanicError{Phase: "descriptor", Value: recovered, Stack: debug.Stack()}
		}
	}()
	return candidate.Descriptor(), nil
}

func callLoggerFactory(factory LoggerFactory, descriptor pluginapi.Descriptor) (logger pluginapi.Logger, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &PanicError{PluginID: descriptor.ID, Phase: "logger-factory", Value: recovered, Stack: debug.Stack()}
		}
	}()
	return factory(descriptor), nil
}

func invokeLifecycle(current *entry, phase string, call func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &PanicError{
				PluginID: current.descriptor.ID,
				Phase:    phase,
				Value:    recovered,
				Stack:    debug.Stack(),
			}
		}
	}()
	return call()
}

// PanicError turns plugin/adapter panics into lifecycle errors instead of
// letting untrusted extension code crash the server process.
type PanicError struct {
	PluginID pluginapi.ID
	Phase    string
	Value    any
	Stack    []byte
}

func (e *PanicError) Error() string {
	if e.PluginID == "" {
		return fmt.Sprintf("pluginruntime: panic during %s: %v", e.Phase, e.Value)
	}
	return fmt.Sprintf("pluginruntime: plugin %s panicked during %s: %v", e.PluginID, e.Phase, e.Value)
}

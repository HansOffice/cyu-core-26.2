// Package pluginruntime contains CyuCore's host-side plugin lifecycle machinery.
// It depends on the public API, never the other way around.
package pluginruntime

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	eventapi "cyu-core-26.2/api/event"
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
	Descriptor         pluginapi.Descriptor
	State              State
	EnableDuration     time.Duration
	DisableDuration    time.Duration
	LastError          string
	EventCalls         uint64
	EventErrors        uint64
	EventTotalDuration time.Duration
	EventMaxDuration   time.Duration
}

// LoggerFactory lets the host attach a plugin identity to log records without
// exposing its concrete logging implementation through the public API.
type LoggerFactory func(pluginapi.Descriptor) pluginapi.Logger

type entry struct {
	plugin     pluginapi.Plugin
	descriptor pluginapi.Descriptor
	context    lifecycleContext

	enabled             atomic.Bool
	acceptSubscriptions atomic.Bool
	eventCalls          atomic.Uint64
	eventErrors         atomic.Uint64
	eventNanos          atomic.Uint64
	maxEventNanos       atomic.Uint64

	state           State
	enableDuration  time.Duration
	disableDuration time.Duration
	lastError       string
}

func (e *entry) recordEventCall(duration time.Duration, failed bool) {
	nanos := uint64(max(duration.Nanoseconds(), 0))
	e.eventCalls.Add(1)
	e.eventNanos.Add(nanos)
	if failed {
		e.eventErrors.Add(1)
	}
	for {
		current := e.maxEventNanos.Load()
		if nanos <= current || e.maxEventNanos.CompareAndSwap(current, nanos) {
			break
		}
	}
}

// Manager owns deterministic plugin registration and lifecycle ordering.
// lifecycleMu serializes registration/enable/disable without holding the state
// mutex while plugin code executes; plugin callbacks therefore cannot deadlock
// the manager merely by using capabilities supplied through Context.
type Manager struct {
	lifecycleMu sync.Mutex
	mu          sync.RWMutex
	entries     map[pluginapi.ID]*entry
	order       []pluginapi.ID

	loggerFactory LoggerFactory
	events        *eventBus
	active        atomic.Bool
}

func New(loggerFactory LoggerFactory) *Manager {
	return &Manager{
		entries:       make(map[pluginapi.ID]*entry),
		loggerFactory: loggerFactory,
		events:        newEventBus(),
	}
}

// Register validates and records a plugin without executing plugin code beyond
// Descriptor. Registration order defines enable order and reverse disable order.
func (m *Manager) Register(candidate pluginapi.Plugin) error {
	if m == nil || candidate == nil {
		return ErrNilPlugin
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.active.Load() {
		return ErrManagerActive
	}

	descriptor, err := readDescriptor(candidate)
	if err != nil {
		return err
	}
	if err := descriptor.Validate(); err != nil {
		return err
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

	current := &entry{
		plugin:     candidate,
		descriptor: descriptor,
		state:      StateRegistered,
	}
	current.context = lifecycleContext{
		descriptor: descriptor,
		logger:     logger,
		events:     m.events.registrar(current),
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.entries[descriptor.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicatePlugin, descriptor.ID)
	}
	m.entries[descriptor.ID] = current
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
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.active.Load() {
		return ErrManagerActive
	}

	entries, err := m.enablePlan()
	if err != nil {
		return err
	}

	enabled := make([]*entry, 0, len(entries))
	for _, current := range entries {
		current.enabled.Store(false)
		current.acceptSubscriptions.Store(true)
		m.updateEntryState(current, StateEnabling, 0, "")

		started := time.Now()
		err := invokeLifecycle(current, "enable", func() error {
			return current.plugin.Enable(current.context)
		})
		duration := time.Since(started)
		if err == nil {
			current.enabled.Store(true)
			m.updateEntryState(current, StateEnabled, duration, "")
			enabled = append(enabled, current)
			continue
		}

		failure := fmt.Errorf("enable plugin %s: %w", current.descriptor.ID, err)
		failures := []error{failure}
		if cleanupErr := m.disableEntry(current, StateFailed); cleanupErr != nil {
			failures = append(failures, fmt.Errorf("cleanup plugin %s: %w", current.descriptor.ID, cleanupErr))
		}
		m.setLastError(current, errors.Join(failures...).Error())

		for index := len(enabled) - 1; index >= 0; index-- {
			rollback := enabled[index]
			if rollbackErr := m.disableEntry(rollback, StateDisabled); rollbackErr != nil {
				failures = append(failures, fmt.Errorf("rollback plugin %s: %w", rollback.descriptor.ID, rollbackErr))
			}
		}
		m.active.Store(false)
		return errors.Join(failures...)
	}

	m.active.Store(true)
	return nil
}

func (m *Manager) enablePlan() ([]*entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]*entry, 0, len(m.order))
	for _, id := range m.order {
		current := m.entries[id]
		switch current.state {
		case StateRegistered, StateDisabled:
			entries = append(entries, current)
		case StateFailed:
			return nil, fmt.Errorf("%w: %s", ErrPluginFailed, id)
		default:
			return nil, fmt.Errorf("pluginruntime: cannot enable %s from state %s", id, current.state)
		}
	}
	return entries, nil
}

// DisableAll disables enabled plugins in reverse registration order. Every
// enabled plugin is given a chance to clean up even when another Disable fails.
func (m *Manager) DisableAll() error {
	if m == nil {
		return nil
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()

	entries := m.entriesInReverseOrder()
	var failures []error
	for _, current := range entries {
		if m.entryState(current) != StateEnabled {
			continue
		}
		if err := m.disableEntry(current, StateDisabled); err != nil {
			failures = append(failures, fmt.Errorf("disable plugin %s: %w", current.descriptor.ID, err))
		}
	}
	m.active.Store(false)
	return errors.Join(failures...)
}

func (m *Manager) disableEntry(current *entry, successState State) error {
	current.enabled.Store(false)
	current.acceptSubscriptions.Store(false)
	m.setState(current, StateDisabling)

	started := time.Now()
	err := invokeLifecycle(current, "disable", func() error {
		return current.plugin.Disable(current.context)
	})
	duration := time.Since(started)
	m.events.removeOwner(current)

	if err != nil {
		m.updateDisableState(current, StateFailed, duration, err.Error())
		return err
	}
	m.updateDisableState(current, successState, duration, "")
	return nil
}

// Dispatch invokes currently enabled plugin handlers synchronously. The server
// runtime owns when this method is called; plugins only receive a subscription
// capability and cannot dispatch arbitrary events themselves.
func (m *Manager) Dispatch(event eventapi.Event) []error {
	if m == nil {
		return nil
	}
	return m.events.dispatch(event)
}

func (m *Manager) Active() bool {
	return m != nil && m.active.Load()
}

func (m *Manager) Snapshots() []Snapshot {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshots := make([]Snapshot, 0, len(m.order))
	for _, id := range m.order {
		current := m.entries[id]
		snapshots = append(snapshots, Snapshot{
			Descriptor:         current.descriptor,
			State:              current.state,
			EnableDuration:     current.enableDuration,
			DisableDuration:    current.disableDuration,
			LastError:          current.lastError,
			EventCalls:         current.eventCalls.Load(),
			EventErrors:        current.eventErrors.Load(),
			EventTotalDuration: time.Duration(current.eventNanos.Load()),
			EventMaxDuration:   time.Duration(current.maxEventNanos.Load()),
		})
	}
	return snapshots
}

func (m *Manager) entriesInReverseOrder() []*entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entries := make([]*entry, 0, len(m.order))
	for index := len(m.order) - 1; index >= 0; index-- {
		entries = append(entries, m.entries[m.order[index]])
	}
	return entries
}

func (m *Manager) entryState(current *entry) State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return current.state
}

func (m *Manager) setState(current *entry, state State) {
	m.mu.Lock()
	current.state = state
	m.mu.Unlock()
}

func (m *Manager) updateEntryState(current *entry, state State, enableDuration time.Duration, lastError string) {
	m.mu.Lock()
	current.state = state
	current.enableDuration = enableDuration
	current.lastError = lastError
	m.mu.Unlock()
}

func (m *Manager) updateDisableState(current *entry, state State, disableDuration time.Duration, lastError string) {
	m.mu.Lock()
	current.state = state
	current.disableDuration = disableDuration
	current.lastError = lastError
	m.mu.Unlock()
}

func (m *Manager) setLastError(current *entry, lastError string) {
	m.mu.Lock()
	current.lastError = lastError
	m.mu.Unlock()
}

type lifecycleContext struct {
	descriptor pluginapi.Descriptor
	logger     pluginapi.Logger
	events     eventapi.Registrar
}

func (c lifecycleContext) Descriptor() pluginapi.Descriptor { return c.descriptor }
func (c lifecycleContext) Logger() pluginapi.Logger         { return c.logger }
func (c lifecycleContext) Events() eventapi.Registrar       { return c.events }

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

// PanicError turns plugin/adapter panics into lifecycle/event errors instead of
// letting extension code crash the server process.
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

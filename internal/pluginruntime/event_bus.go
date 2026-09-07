package pluginruntime

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	eventapi "cyu-core-26.2/api/event"
)

var ErrSubscriptionsClosed = errors.New("pluginruntime: event subscriptions are closed")

type eventHandlerRecord struct {
	id      uint64
	owner   *entry
	handler eventapi.Handler
}

type eventHandlerTable struct {
	byType map[eventapi.Type][]eventHandlerRecord
}

// eventBus uses copy-on-write immutable tables. Subscription changes are rare
// compared with dispatch, so the runtime hot path performs one atomic load and
// one map lookup without copying handler slices or taking a bus mutex.
type eventBus struct {
	mu     sync.Mutex
	nextID uint64
	table  atomic.Pointer[eventHandlerTable]
}

func newEventBus() *eventBus {
	bus := &eventBus{}
	bus.table.Store(&eventHandlerTable{byType: make(map[eventapi.Type][]eventHandlerRecord)})
	return bus
}

func (b *eventBus) registrar(owner *entry) eventapi.Registrar {
	return scopedEventRegistrar{bus: b, owner: owner}
}

func (b *eventBus) subscribe(owner *entry, eventType eventapi.Type, handler eventapi.Handler) (eventapi.Subscription, error) {
	if b == nil || owner == nil || !owner.acceptSubscriptions.Load() {
		return nil, ErrSubscriptionsClosed
	}
	if _, err := eventapi.ParseType(string(eventType)); err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, eventapi.ErrNilHandler
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if !owner.acceptSubscriptions.Load() {
		return nil, ErrSubscriptionsClosed
	}

	b.nextID++
	id := b.nextID
	current := b.table.Load()
	next := cloneEventTable(current)
	previous := next.byType[eventType]
	handlers := make([]eventHandlerRecord, len(previous), len(previous)+1)
	copy(handlers, previous)
	handlers = append(handlers, eventHandlerRecord{id: id, owner: owner, handler: handler})
	next.byType[eventType] = handlers
	b.table.Store(next)

	return &eventSubscription{bus: b, eventType: eventType, id: id}, nil
}

func (b *eventBus) remove(eventType eventapi.Type, id uint64) bool {
	if b == nil || id == 0 {
		return false
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	current := b.table.Load()
	handlers := current.byType[eventType]
	index := -1
	for i := range handlers {
		if handlers[i].id == id {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}

	next := cloneEventTable(current)
	if len(handlers) == 1 {
		delete(next.byType, eventType)
	} else {
		updated := make([]eventHandlerRecord, 0, len(handlers)-1)
		updated = append(updated, handlers[:index]...)
		updated = append(updated, handlers[index+1:]...)
		next.byType[eventType] = updated
	}
	b.table.Store(next)
	return true
}

func (b *eventBus) removeOwner(owner *entry) int {
	if b == nil || owner == nil {
		return 0
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	current := b.table.Load()
	next := cloneEventTable(current)
	removed := 0
	for eventType, handlers := range current.byType {
		kept := make([]eventHandlerRecord, 0, len(handlers))
		for _, record := range handlers {
			if record.owner == owner {
				removed++
				continue
			}
			kept = append(kept, record)
		}
		if len(kept) == 0 {
			delete(next.byType, eventType)
		} else if len(kept) != len(handlers) {
			next.byType[eventType] = kept
		}
	}
	if removed > 0 {
		b.table.Store(next)
	}
	return removed
}

func (b *eventBus) dispatch(event eventapi.Event) []error {
	if b == nil || event == nil {
		return nil
	}
	eventType, err := readEventType(event)
	if err != nil {
		return []error{err}
	}
	if _, err := eventapi.ParseType(string(eventType)); err != nil {
		return []error{err}
	}

	table := b.table.Load()
	if table == nil {
		return nil
	}
	handlers := table.byType[eventType]
	var failures []error
	for _, record := range handlers {
		if record.owner == nil || !record.owner.enabled.Load() {
			continue
		}

		started := time.Now()
		handlerErr := invokeEventHandler(record, eventType, event)
		duration := time.Since(started)
		record.owner.recordEventCall(duration, handlerErr != nil)
		if handlerErr != nil {
			failures = append(failures, fmt.Errorf("plugin %s handling %s: %w", record.owner.descriptor.ID, eventType, handlerErr))
		}
	}
	return failures
}

func cloneEventTable(current *eventHandlerTable) *eventHandlerTable {
	next := &eventHandlerTable{byType: make(map[eventapi.Type][]eventHandlerRecord)}
	if current == nil {
		return next
	}
	for eventType, handlers := range current.byType {
		next.byType[eventType] = handlers
	}
	return next
}

func readEventType(event eventapi.Event) (eventType eventapi.Type, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &PanicError{Phase: "event-type", Value: recovered, Stack: debug.Stack()}
		}
	}()
	return event.Type(), nil
}

func invokeEventHandler(record eventHandlerRecord, eventType eventapi.Type, event eventapi.Event) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &PanicError{
				PluginID: record.owner.descriptor.ID,
				Phase:    "event:" + string(eventType),
				Value:    recovered,
				Stack:    debug.Stack(),
			}
		}
	}()
	return record.handler(event)
}

type scopedEventRegistrar struct {
	bus   *eventBus
	owner *entry
}

func (r scopedEventRegistrar) Subscribe(eventType eventapi.Type, handler eventapi.Handler) (eventapi.Subscription, error) {
	return r.bus.subscribe(r.owner, eventType, handler)
}

type eventSubscription struct {
	bus       *eventBus
	eventType eventapi.Type
	id        uint64
	canceled  atomic.Bool
}

func (s *eventSubscription) Cancel() bool {
	if s == nil || s.canceled.Swap(true) {
		return false
	}
	return s.bus.remove(s.eventType, s.id)
}

package mailbox

import "sync"

// Queue is a generic multi-producer, single-consumer mailbox. Ordinary values
// are bounded so producer pressure cannot consume unbounded memory. Critical
// values are retained separately because lifecycle work must not be dropped.
//
// Queue deliberately stores values rather than callbacks. Runtime ownership is
// therefore explicit at the consumer boundary and hot paths do not need a heap-
// escaping closure for every event.
type Queue[T any] struct {
	values chan T

	criticalMu sync.Mutex
	critical   []T
}

func New[T any](capacity int) *Queue[T] {
	if capacity <= 0 {
		panic("mailbox: capacity must be positive")
	}
	return &Queue[T]{values: make(chan T, capacity)}
}

// TryPost enqueues ordinary work without blocking. Queue pressure is explicit
// so callers can reject work or disconnect a producer instead of silently
// losing authoritative state.
func (q *Queue[T]) TryPost(value T) bool {
	if q == nil {
		return false
	}
	select {
	case q.values <- value:
		return true
	default:
		return false
	}
}

// PostCritical retains lifecycle work even when the ordinary bounded queue is
// full. Critical values are returned before ordinary values by TryPop.
func (q *Queue[T]) PostCritical(value T) bool {
	if q == nil {
		return false
	}
	q.criticalMu.Lock()
	q.critical = append(q.critical, value)
	q.criticalMu.Unlock()
	return true
}

// TryPop returns one value without blocking. Critical lifecycle values have
// priority; ordinary values preserve channel FIFO order. Exactly one goroutine
// must consume from a Queue.
func (q *Queue[T]) TryPop() (T, bool) {
	var zero T
	if q == nil {
		return zero, false
	}
	if value, ok := q.popCritical(); ok {
		return value, true
	}
	select {
	case value := <-q.values:
		return value, true
	default:
		return zero, false
	}
}

func (q *Queue[T]) popCritical() (T, bool) {
	var zero T
	q.criticalMu.Lock()
	defer q.criticalMu.Unlock()
	if len(q.critical) == 0 {
		return zero, false
	}
	value := q.critical[0]
	q.critical[0] = zero
	if len(q.critical) == 1 {
		q.critical = nil
	} else {
		q.critical = q.critical[1:]
	}
	return value, true
}

func (q *Queue[T]) Len() int {
	if q == nil {
		return 0
	}
	return len(q.values)
}

func (q *Queue[T]) CriticalLen() int {
	if q == nil {
		return 0
	}
	q.criticalMu.Lock()
	defer q.criticalMu.Unlock()
	return len(q.critical)
}

func (q *Queue[T]) Cap() int {
	if q == nil {
		return 0
	}
	return cap(q.values)
}

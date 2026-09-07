// Package scheduler defines the public tick-based plugin scheduling contract.
// Scheduling is expressed in server ticks so it remains deterministic and tied
// to the runtime owner rather than wall-clock goroutines.
package scheduler

import "errors"

var (
	ErrNilHandler      = errors.New("scheduler: nil handler")
	ErrInvalidDelay    = errors.New("scheduler: delay must be at least one tick")
	ErrInvalidInterval = errors.New("scheduler: interval must be at least one tick")
	ErrClosed          = errors.New("scheduler: registrar is closed")
)

// Handler executes synchronously on CyuCore's runtime owner. It must not block
// on network or disk I/O. Errors are recorded in plugin health metrics and do
// not stop other tasks due in the same tick.
type Handler func() error

// Task controls one scheduled registration. Cancel is idempotent. A task that
// is already executing may finish its current invocation; cancellation prevents
// future invocations.
type Task interface {
	Cancel() bool
}

// Registrar is the scheduling capability exposed to one plugin. Callbacks are
// stored by pluginruntime and resolved by the tick owner; they are never placed
// directly into the server's cross-thread runtime mailbox.
//
// EveryTicks uses fixed-delay semantics: after an invocation, the next run is
// scheduled interval ticks after the tick that executed it. Missed intervals
// are not replayed as a catch-up burst.
type Registrar interface {
	AfterTicks(delay uint64, handler Handler) (Task, error)
	EveryTicks(interval uint64, handler Handler) (Task, error)
}

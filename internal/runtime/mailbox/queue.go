package mailbox

// Task is one unit of state mutation that must execute on the runtime owner
// goroutine. Tasks must not block on network or disk I/O.
type Task func()

// Queue is a bounded multi-producer, single-consumer mailbox. Producers use
// TryPost from network/console goroutines; the tick goroutine is the only
// consumer and calls Drain.
type Queue struct {
	tasks chan Task
}

func New(capacity int) *Queue {
	if capacity <= 0 {
		panic("mailbox: capacity must be positive")
	}
	return &Queue{tasks: make(chan Task, capacity)}
}

// TryPost enqueues task without blocking. Queue pressure is explicit so callers
// can disconnect or reject work instead of silently losing authoritative state.
func (q *Queue) TryPost(task Task) bool {
	if q == nil || task == nil {
		return false
	}
	select {
	case q.tasks <- task:
		return true
	default:
		return false
	}
}

// Drain executes up to limit queued tasks in FIFO receive order. It never
// blocks waiting for work. Drain is intended to be called by exactly one
// runtime owner goroutine.
func (q *Queue) Drain(limit int) int {
	if q == nil || limit <= 0 {
		return 0
	}

	drained := 0
	for drained < limit {
		select {
		case task := <-q.tasks:
			task()
			drained++
		default:
			return drained
		}
	}
	return drained
}

func (q *Queue) Len() int {
	if q == nil {
		return 0
	}
	return len(q.tasks)
}

func (q *Queue) Cap() int {
	if q == nil {
		return 0
	}
	return cap(q.tasks)
}

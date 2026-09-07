package mailbox

import "sync"

// Task is one unit of state mutation that must execute on the runtime owner
// goroutine. Tasks must not block on network or disk I/O.
type Task func()

// Queue is a multi-producer, single-consumer mailbox. Ordinary gameplay work is
// bounded so network pressure cannot consume unbounded memory. Critical
// lifecycle work is retained in a mutex-protected side queue because dropping a
// disconnect/removal can leave authoritative state permanently inconsistent.
type Queue struct {
	tasks chan Task

	criticalMu sync.Mutex
	critical   []Task
}

func New(capacity int) *Queue {
	if capacity <= 0 {
		panic("mailbox: capacity must be positive")
	}
	return &Queue{tasks: make(chan Task, capacity)}
}

// TryPost enqueues ordinary work without blocking. Queue pressure is explicit
// so callers can disconnect or reject work instead of silently losing
// authoritative state.
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

// PostCritical retains lifecycle work even when the ordinary bounded queue is
// full. Critical tasks are executed before ordinary tasks on the next Drain.
func (q *Queue) PostCritical(task Task) bool {
	if q == nil || task == nil {
		return false
	}
	q.criticalMu.Lock()
	q.critical = append(q.critical, task)
	q.criticalMu.Unlock()
	return true
}

// Drain executes up to limit tasks. Critical lifecycle tasks have priority;
// ordinary tasks preserve channel FIFO order. Drain never blocks waiting for
// work and must have exactly one consumer.
func (q *Queue) Drain(limit int) int {
	if q == nil || limit <= 0 {
		return 0
	}

	drained := 0
	for drained < limit {
		if task := q.popCritical(); task != nil {
			task()
			drained++
			continue
		}

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

func (q *Queue) popCritical() Task {
	q.criticalMu.Lock()
	defer q.criticalMu.Unlock()
	if len(q.critical) == 0 {
		return nil
	}
	task := q.critical[0]
	q.critical[0] = nil
	q.critical = q.critical[1:]
	return task
}

func (q *Queue) Len() int {
	if q == nil {
		return 0
	}
	return len(q.tasks)
}

func (q *Queue) CriticalLen() int {
	if q == nil {
		return 0
	}
	q.criticalMu.Lock()
	defer q.criticalMu.Unlock()
	return len(q.critical)
}

func (q *Queue) Cap() int {
	if q == nil {
		return 0
	}
	return cap(q.tasks)
}

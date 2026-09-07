package pluginruntime

import (
	"container/heap"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	schedulerapi "cyu-core-26.2/api/scheduler"
)

type schedulerSnapshot struct {
	calls         uint64
	errors        uint64
	totalDuration time.Duration
	maxDuration   time.Duration
	pending       int64
}

type schedulerOwnerState struct {
	open     atomic.Bool
	calls    atomic.Uint64
	errors   atomic.Uint64
	nanos    atomic.Uint64
	maxNanos atomic.Uint64
	pending  atomic.Int64
}

func (s *schedulerOwnerState) record(duration time.Duration, failed bool) {
	if s == nil {
		return
	}
	nanos := uint64(max(duration.Nanoseconds(), 0))
	s.calls.Add(1)
	s.nanos.Add(nanos)
	if failed {
		s.errors.Add(1)
	}
	for {
		current := s.maxNanos.Load()
		if nanos <= current || s.maxNanos.CompareAndSwap(current, nanos) {
			return
		}
	}
}

type scheduledTask struct {
	scheduler *taskScheduler
	owner     *entry
	state     *schedulerOwnerState
	id        uint64
	due       uint64
	interval  uint64
	handler   schedulerapi.Handler
	index     int
	active    atomic.Bool
}

func (t *scheduledTask) Cancel() bool {
	if t == nil || t.scheduler == nil {
		return false
	}
	return t.scheduler.cancel(t)
}

type taskHeap []*scheduledTask

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool {
	if h[i].due == h[j].due {
		return h[i].id < h[j].id
	}
	return h[i].due < h[j].due
}

func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *taskHeap) Push(value any) {
	task := value.(*scheduledTask)
	task.index = len(*h)
	*h = append(*h, task)
}

func (h *taskHeap) Pop() any {
	old := *h
	last := len(old) - 1
	task := old[last]
	old[last] = nil
	task.index = -1
	*h = old[:last]
	return task
}

type taskScheduler struct {
	mu      sync.Mutex
	now     uint64
	nextID  uint64
	queue   taskHeap
	byOwner map[*entry]map[uint64]*scheduledTask
	owners  map[*entry]*schedulerOwnerState
}

func newTaskScheduler() *taskScheduler {
	scheduler := &taskScheduler{
		byOwner: make(map[*entry]map[uint64]*scheduledTask),
		owners:  make(map[*entry]*schedulerOwnerState),
	}
	heap.Init(&scheduler.queue)
	return scheduler
}

func (s *taskScheduler) registrar(owner *entry) schedulerapi.Registrar {
	return scopedSchedulerRegistrar{scheduler: s, owner: owner}
}

func (s *taskScheduler) openOwner(owner *entry) {
	if s == nil || owner == nil {
		return
	}
	s.mu.Lock()
	state := s.ownerStateLocked(owner)
	state.open.Store(true)
	s.mu.Unlock()
}

func (s *taskScheduler) closeOwner(owner *entry) {
	if s == nil || owner == nil {
		return
	}
	s.mu.Lock()
	if state := s.owners[owner]; state != nil {
		state.open.Store(false)
	}
	s.removeOwnerLocked(owner)
	s.mu.Unlock()
}

func (s *taskScheduler) ownerStateLocked(owner *entry) *schedulerOwnerState {
	state := s.owners[owner]
	if state == nil {
		state = &schedulerOwnerState{}
		s.owners[owner] = state
	}
	return state
}

func (s *taskScheduler) schedule(owner *entry, delay, interval uint64, handler schedulerapi.Handler) (schedulerapi.Task, error) {
	if handler == nil {
		return nil, schedulerapi.ErrNilHandler
	}
	if delay == 0 {
		if interval == 0 {
			return nil, schedulerapi.ErrInvalidDelay
		}
		return nil, schedulerapi.ErrInvalidInterval
	}
	if s == nil || owner == nil {
		return nil, schedulerapi.ErrClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.ownerStateLocked(owner)
	if !state.open.Load() {
		return nil, schedulerapi.ErrClosed
	}
	if ^uint64(0)-s.now < delay {
		return nil, fmt.Errorf("pluginruntime: scheduler deadline overflow")
	}

	s.nextID++
	if s.nextID == 0 {
		return nil, fmt.Errorf("pluginruntime: scheduler task id overflow")
	}

	task := &scheduledTask{
		scheduler: s,
		owner:     owner,
		state:     state,
		id:        s.nextID,
		due:       s.now + delay,
		interval:  interval,
		handler:   handler,
		index:     -1,
	}
	task.active.Store(true)

	ownerTasks := s.byOwner[owner]
	if ownerTasks == nil {
		ownerTasks = make(map[uint64]*scheduledTask)
		s.byOwner[owner] = ownerTasks
	}
	ownerTasks[task.id] = task
	state.pending.Add(1)
	heap.Push(&s.queue, task)
	return task, nil
}

func (s *taskScheduler) cancel(task *scheduledTask) bool {
	if s == nil || task == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !task.active.CompareAndSwap(true, false) {
		return false
	}
	if task.index >= 0 {
		heap.Remove(&s.queue, task.index)
	}
	s.removeRegistrationLocked(task)
	return true
}

func (s *taskScheduler) removeOwnerLocked(owner *entry) {
	tasks := s.byOwner[owner]
	for _, task := range tasks {
		if !task.active.CompareAndSwap(true, false) {
			continue
		}
		if task.index >= 0 {
			heap.Remove(&s.queue, task.index)
		}
		task.state.pending.Add(-1)
	}
	delete(s.byOwner, owner)
}

func (s *taskScheduler) removeRegistrationLocked(task *scheduledTask) {
	if ownerTasks := s.byOwner[task.owner]; ownerTasks != nil {
		delete(ownerTasks, task.id)
		if len(ownerTasks) == 0 {
			delete(s.byOwner, task.owner)
		}
	}
	task.state.pending.Add(-1)
}

func (s *taskScheduler) retireOneShotLocked(task *scheduledTask) {
	if task == nil || !task.active.CompareAndSwap(true, false) {
		return
	}
	s.removeRegistrationLocked(task)
}

func (s *taskScheduler) advance(limit int) []error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	s.now++
	now := s.now
	s.mu.Unlock()

	if limit <= 0 {
		return nil
	}

	var failures []error
	executed := 0
	for executed < limit {
		s.mu.Lock()
		if len(s.queue) == 0 || s.queue[0].due > now {
			s.mu.Unlock()
			break
		}
		task := heap.Pop(&s.queue).(*scheduledTask)
		oneShot := task.interval == 0
		if oneShot {
			s.retireOneShotLocked(task)
		}
		s.mu.Unlock()

		if task.owner == nil || !task.owner.enabled.Load() {
			if !oneShot {
				s.cancel(task)
			}
			continue
		}

		started := time.Now()
		err := invokeScheduledTask(task)
		duration := time.Since(started)
		task.state.record(duration, err != nil)
		executed++
		if err != nil {
			failures = append(failures, fmt.Errorf("plugin %s scheduled task %d: %w", task.owner.descriptor.ID, task.id, err))
		}

		if oneShot {
			continue
		}

		s.mu.Lock()
		if task.active.Load() && task.owner.enabled.Load() && task.state.open.Load() {
			if ^uint64(0)-now < task.interval {
				task.active.Store(false)
				s.removeRegistrationLocked(task)
				failures = append(failures, fmt.Errorf("plugin %s scheduled task %d: deadline overflow", task.owner.descriptor.ID, task.id))
			} else {
				task.due = now + task.interval
				heap.Push(&s.queue, task)
			}
		} else if task.active.CompareAndSwap(true, false) {
			s.removeRegistrationLocked(task)
		}
		s.mu.Unlock()
	}
	return failures
}

func (s *taskScheduler) snapshot(owner *entry) schedulerSnapshot {
	if s == nil || owner == nil {
		return schedulerSnapshot{}
	}
	s.mu.Lock()
	state := s.owners[owner]
	s.mu.Unlock()
	if state == nil {
		return schedulerSnapshot{}
	}
	return schedulerSnapshot{
		calls:         state.calls.Load(),
		errors:        state.errors.Load(),
		totalDuration: time.Duration(state.nanos.Load()),
		maxDuration:   time.Duration(state.maxNanos.Load()),
		pending:       state.pending.Load(),
	}
}

func invokeScheduledTask(task *scheduledTask) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &PanicError{
				PluginID: task.owner.descriptor.ID,
				Phase:    "scheduler",
				Value:    recovered,
				Stack:    debug.Stack(),
			}
		}
	}()
	return task.handler()
}

type scopedSchedulerRegistrar struct {
	scheduler *taskScheduler
	owner     *entry
}

func (r scopedSchedulerRegistrar) AfterTicks(delay uint64, handler schedulerapi.Handler) (schedulerapi.Task, error) {
	if delay == 0 {
		return nil, schedulerapi.ErrInvalidDelay
	}
	return r.scheduler.schedule(r.owner, delay, 0, handler)
}

func (r scopedSchedulerRegistrar) EveryTicks(interval uint64, handler schedulerapi.Handler) (schedulerapi.Task, error) {
	if interval == 0 {
		return nil, schedulerapi.ErrInvalidInterval
	}
	return r.scheduler.schedule(r.owner, interval, interval, handler)
}

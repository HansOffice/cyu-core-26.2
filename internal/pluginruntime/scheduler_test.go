package pluginruntime

import (
	"errors"
	"reflect"
	"testing"

	pluginapi "cyu-core-26.2/api/plugin"
	schedulerapi "cyu-core-26.2/api/scheduler"
)

func TestSchedulerRunsDeterministicallyAndRepeats(t *testing.T) {
	manager := New(nil)
	var calls []string

	candidate := &fakePlugin{
		descriptor: descriptor("scheduler-order"),
		enable: func(ctx pluginapi.Context) error {
			if _, err := ctx.Scheduler().AfterTicks(2, func() error {
				calls = append(calls, "after")
				return nil
			}); err != nil {
				return err
			}
			_, err := ctx.Scheduler().EveryTicks(2, func() error {
				calls = append(calls, "repeat")
				return nil
			})
			return err
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}

	if failures := manager.AdvanceTick(16); len(failures) != 0 {
		t.Fatalf("tick 1 failures = %v", failures)
	}
	if len(calls) != 0 {
		t.Fatalf("tick 1 calls = %v", calls)
	}
	if failures := manager.AdvanceTick(16); len(failures) != 0 {
		t.Fatalf("tick 2 failures = %v", failures)
	}
	if got, want := calls, []string{"after", "repeat"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tick 2 calls = %v, want %v", got, want)
	}

	manager.AdvanceTick(16)
	manager.AdvanceTick(16)
	if got, want := calls, []string{"after", "repeat", "repeat"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tick 4 calls = %v, want %v", got, want)
	}

	snapshot := manager.Snapshots()[0]
	if snapshot.TaskCalls != 3 || snapshot.TaskErrors != 0 || snapshot.ScheduledTasks != 1 {
		t.Fatalf("task metrics = calls:%d errors:%d pending:%d", snapshot.TaskCalls, snapshot.TaskErrors, snapshot.ScheduledTasks)
	}
}

func TestSchedulerBudgetDefersDueTasksInIDOrder(t *testing.T) {
	manager := New(nil)
	var calls []int
	candidate := &fakePlugin{
		descriptor: descriptor("scheduler-budget"),
		enable: func(ctx pluginapi.Context) error {
			for value := 1; value <= 3; value++ {
				value := value
				if _, err := ctx.Scheduler().AfterTicks(1, func() error {
					calls = append(calls, value)
					return nil
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}

	manager.AdvanceTick(2)
	if got, want := calls, []int{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first budgeted tick = %v, want %v", got, want)
	}
	manager.AdvanceTick(2)
	if got, want := calls, []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deferred task order = %v, want %v", got, want)
	}
}

func TestSchedulerCancelAndDisableCleanup(t *testing.T) {
	manager := New(nil)
	var registrar schedulerapi.Registrar
	var repeating schedulerapi.Task
	calls := 0

	candidate := &fakePlugin{
		descriptor: descriptor("scheduler-cleanup"),
		enable: func(ctx pluginapi.Context) error {
			registrar = ctx.Scheduler()
			var err error
			repeating, err = registrar.EveryTicks(1, func() error {
				calls++
				return nil
			})
			return err
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}

	manager.AdvanceTick(16)
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if !repeating.Cancel() {
		t.Fatal("first cancel returned false")
	}
	if repeating.Cancel() {
		t.Fatal("second cancel returned true")
	}
	manager.AdvanceTick(16)
	if calls != 1 {
		t.Fatalf("cancelled task ran again; calls = %d", calls)
	}

	if _, err := registrar.EveryTicks(1, func() error { return nil }); err != nil {
		t.Fatalf("schedule while enabled: %v", err)
	}
	if err := manager.DisableAll(); err != nil {
		t.Fatal(err)
	}
	if snapshot := manager.Snapshots()[0]; snapshot.ScheduledTasks != 0 {
		t.Fatalf("pending tasks after disable = %d", snapshot.ScheduledTasks)
	}
	if _, err := registrar.AfterTicks(1, func() error { return nil }); !errors.Is(err, schedulerapi.ErrClosed) {
		t.Fatalf("schedule after disable = %v, want ErrClosed", err)
	}
}

func TestSchedulerPanicAndErrorsDoNotStopLaterTasks(t *testing.T) {
	manager := New(nil)
	calls := 0
	candidate := &fakePlugin{
		descriptor: descriptor("scheduler-errors"),
		enable: func(ctx pluginapi.Context) error {
			if _, err := ctx.Scheduler().AfterTicks(1, func() error {
				panic("scheduled boom")
			}); err != nil {
				return err
			}
			if _, err := ctx.Scheduler().AfterTicks(1, func() error {
				return errors.New("task error")
			}); err != nil {
				return err
			}
			_, err := ctx.Scheduler().AfterTicks(1, func() error {
				calls++
				return nil
			})
			return err
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}

	failures := manager.AdvanceTick(16)
	if calls != 1 {
		t.Fatalf("later task calls = %d, want 1", calls)
	}
	if len(failures) != 2 {
		t.Fatalf("failures = %v, want 2", failures)
	}
	var panicErr *PanicError
	if !errors.As(failures[0], &panicErr) || panicErr.Phase != "scheduler" || len(panicErr.Stack) == 0 {
		t.Fatalf("panic diagnostics = %v", failures[0])
	}
	snapshot := manager.Snapshots()[0]
	if snapshot.TaskCalls != 3 || snapshot.TaskErrors != 2 {
		t.Fatalf("task metrics = calls:%d errors:%d", snapshot.TaskCalls, snapshot.TaskErrors)
	}
}

func TestSchedulerCanScheduleFromTaskWithoutDeadlock(t *testing.T) {
	manager := New(nil)
	calls := 0
	candidate := &fakePlugin{
		descriptor: descriptor("scheduler-reentrant"),
		enable: func(ctx pluginapi.Context) error {
			_, err := ctx.Scheduler().AfterTicks(1, func() error {
				calls++
				_, scheduleErr := ctx.Scheduler().AfterTicks(1, func() error {
					calls++
					return nil
				})
				return scheduleErr
			})
			return err
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}

	manager.AdvanceTick(16)
	if calls != 1 {
		t.Fatalf("tick 1 calls = %d, want 1", calls)
	}
	manager.AdvanceTick(16)
	if calls != 2 {
		t.Fatalf("tick 2 calls = %d, want 2", calls)
	}
}

func TestSchedulerValidation(t *testing.T) {
	manager := New(nil)
	var registrar schedulerapi.Registrar
	candidate := &fakePlugin{
		descriptor: descriptor("scheduler-validation"),
		enable: func(ctx pluginapi.Context) error {
			registrar = ctx.Scheduler()
			return nil
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}

	if _, err := registrar.AfterTicks(0, func() error { return nil }); !errors.Is(err, schedulerapi.ErrInvalidDelay) {
		t.Fatalf("AfterTicks(0) = %v", err)
	}
	if _, err := registrar.EveryTicks(0, func() error { return nil }); !errors.Is(err, schedulerapi.ErrInvalidInterval) {
		t.Fatalf("EveryTicks(0) = %v", err)
	}
	if _, err := registrar.AfterTicks(1, nil); !errors.Is(err, schedulerapi.ErrNilHandler) {
		t.Fatalf("AfterTicks(nil) = %v", err)
	}
}

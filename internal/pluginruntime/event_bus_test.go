package pluginruntime

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	eventapi "cyu-core-26.2/api/event"
	pluginapi "cyu-core-26.2/api/plugin"
)

const testEventType eventapi.Type = "test:event"

type testEvent struct {
	value string
}

func (testEvent) Type() eventapi.Type { return testEventType }

func TestEventDispatchOrderErrorsAndMetrics(t *testing.T) {
	manager := New(nil)
	var calls []string

	for _, id := range []string{"alpha", "beta"} {
		id := id
		candidate := &fakePlugin{
			descriptor: descriptor(id),
			enable: func(ctx pluginapi.Context) error {
				_, err := ctx.Events().Subscribe(testEventType, func(event eventapi.Event) error {
					calls = append(calls, id+":"+event.(testEvent).value)
					if id == "alpha" {
						return errors.New("alpha failed")
					}
					return nil
				})
				return err
			},
		}
		if err := manager.Register(candidate); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}

	failures := manager.Dispatch(testEvent{value: "one"})
	if got, want := calls, []string{"alpha:one", "beta:one"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dispatch order = %v, want %v", got, want)
	}
	if len(failures) != 1 || !strings.Contains(failures[0].Error(), "alpha failed") {
		t.Fatalf("dispatch failures = %v, want one alpha error", failures)
	}

	snapshots := manager.Snapshots()
	if snapshots[0].EventCalls != 1 || snapshots[0].EventErrors != 1 {
		t.Fatalf("alpha event metrics = calls:%d errors:%d", snapshots[0].EventCalls, snapshots[0].EventErrors)
	}
	if snapshots[1].EventCalls != 1 || snapshots[1].EventErrors != 0 {
		t.Fatalf("beta event metrics = calls:%d errors:%d", snapshots[1].EventCalls, snapshots[1].EventErrors)
	}
}

func TestEventPanicIsIsolatedAndLaterHandlersRun(t *testing.T) {
	manager := New(nil)
	var calls []string

	panicPlugin := &fakePlugin{
		descriptor: descriptor("panic-handler"),
		enable: func(ctx pluginapi.Context) error {
			_, err := ctx.Events().Subscribe(testEventType, func(eventapi.Event) error {
				calls = append(calls, "panic")
				panic("event boom")
			})
			return err
		},
	}
	laterPlugin := &fakePlugin{
		descriptor: descriptor("later-handler"),
		enable: func(ctx pluginapi.Context) error {
			_, err := ctx.Events().Subscribe(testEventType, func(eventapi.Event) error {
				calls = append(calls, "later")
				return nil
			})
			return err
		},
	}
	if err := manager.Register(panicPlugin); err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(laterPlugin); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}

	failures := manager.Dispatch(testEvent{})
	if !reflect.DeepEqual(calls, []string{"panic", "later"}) {
		t.Fatalf("calls = %v", calls)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %v, want one panic error", failures)
	}
	var panicErr *PanicError
	if !errors.As(failures[0], &panicErr) || panicErr.Phase != "event:test:event" || len(panicErr.Stack) == 0 {
		t.Fatalf("panic diagnostics = %v", failures[0])
	}
}

func TestDisableAutomaticallyRemovesSubscriptions(t *testing.T) {
	manager := New(nil)
	var registrar eventapi.Registrar
	calls := 0
	candidate := &fakePlugin{
		descriptor: descriptor("subscriptions"),
		enable: func(ctx pluginapi.Context) error {
			registrar = ctx.Events()
			_, err := registrar.Subscribe(testEventType, func(eventapi.Event) error {
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
	manager.Dispatch(testEvent{})
	if calls != 1 {
		t.Fatalf("calls before disable = %d, want 1", calls)
	}

	if err := manager.DisableAll(); err != nil {
		t.Fatal(err)
	}
	manager.Dispatch(testEvent{})
	if calls != 1 {
		t.Fatalf("handler survived disable; calls = %d", calls)
	}
	if _, err := registrar.Subscribe(testEventType, func(eventapi.Event) error { return nil }); !errors.Is(err, ErrSubscriptionsClosed) {
		t.Fatalf("subscribe after disable = %v, want ErrSubscriptionsClosed", err)
	}
}

func TestSubscriptionCancelIsIdempotent(t *testing.T) {
	manager := New(nil)
	var subscription eventapi.Subscription
	calls := 0
	candidate := &fakePlugin{
		descriptor: descriptor("cancel"),
		enable: func(ctx pluginapi.Context) error {
			var err error
			subscription, err = ctx.Events().Subscribe(testEventType, func(eventapi.Event) error {
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
	if !subscription.Cancel() {
		t.Fatal("first Cancel returned false")
	}
	if subscription.Cancel() {
		t.Fatal("second Cancel returned true")
	}
	manager.Dispatch(testEvent{})
	if calls != 0 {
		t.Fatalf("canceled handler called %d times", calls)
	}
}

func TestFailedEnableRemovesSubscriptions(t *testing.T) {
	manager := New(nil)
	calls := 0
	candidate := &fakePlugin{
		descriptor: descriptor("failed-subscription"),
		enable: func(ctx pluginapi.Context) error {
			_, err := ctx.Events().Subscribe(testEventType, func(eventapi.Event) error {
				calls++
				return nil
			})
			if err != nil {
				return err
			}
			return errors.New("enable failure")
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); err == nil {
		t.Fatal("EnableAll unexpectedly succeeded")
	}
	manager.Dispatch(testEvent{})
	if calls != 0 {
		t.Fatalf("failed plugin handler dispatched %d times", calls)
	}
}

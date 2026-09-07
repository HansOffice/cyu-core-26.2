package pluginruntime

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	pluginapi "cyu-core-26.2/api/plugin"
)

type fakePlugin struct {
	descriptor pluginapi.Descriptor
	enable     func(pluginapi.Context) error
	disable    func(pluginapi.Context) error
}

func (p *fakePlugin) Descriptor() pluginapi.Descriptor { return p.descriptor }
func (p *fakePlugin) Enable(ctx pluginapi.Context) error {
	if p.enable != nil {
		return p.enable(ctx)
	}
	return nil
}
func (p *fakePlugin) Disable(ctx pluginapi.Context) error {
	if p.disable != nil {
		return p.disable(ctx)
	}
	return nil
}

func descriptor(id string) pluginapi.Descriptor {
	return pluginapi.Descriptor{
		ID:         pluginapi.ID(id),
		Name:       strings.ToUpper(id),
		Version:    "1.0.0",
		APIVersion: pluginapi.CurrentAPIVersion,
	}
}

func TestManagerLifecycleOrderAndRestart(t *testing.T) {
	manager := New(nil)
	var calls []string

	for _, id := range []string{"alpha", "beta"} {
		id := id
		plugin := &fakePlugin{
			descriptor: descriptor(id),
			enable: func(pluginapi.Context) error {
				calls = append(calls, "enable:"+id)
				return nil
			},
			disable: func(pluginapi.Context) error {
				calls = append(calls, "disable:"+id)
				return nil
			},
		}
		if err := manager.Register(plugin); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	if err := manager.EnableAll(); err != nil {
		t.Fatalf("EnableAll: %v", err)
	}
	if !manager.Active() {
		t.Fatal("manager not active after enable")
	}
	if got, want := calls, []string{"enable:alpha", "enable:beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("enable order = %v, want %v", got, want)
	}

	if err := manager.DisableAll(); err != nil {
		t.Fatalf("DisableAll: %v", err)
	}
	if manager.Active() {
		t.Fatal("manager active after disable")
	}
	want := []string{"enable:alpha", "enable:beta", "disable:beta", "disable:alpha"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("lifecycle order = %v, want %v", calls, want)
	}

	for _, snapshot := range manager.Snapshots() {
		if snapshot.State != StateDisabled {
			t.Fatalf("plugin %s state = %s, want disabled", snapshot.Descriptor.ID, snapshot.State)
		}
	}

	calls = nil
	if err := manager.EnableAll(); err != nil {
		t.Fatalf("EnableAll after clean disable: %v", err)
	}
	if got, want := calls, []string{"enable:alpha", "enable:beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restart enable order = %v, want %v", got, want)
	}
}

func TestEnableFailureCleansFailingPluginAndRollsBack(t *testing.T) {
	manager := New(nil)
	var calls []string

	alpha := &fakePlugin{
		descriptor: descriptor("alpha"),
		enable: func(pluginapi.Context) error {
			calls = append(calls, "enable:alpha")
			return nil
		},
		disable: func(pluginapi.Context) error {
			calls = append(calls, "disable:alpha")
			return nil
		},
	}
	beta := &fakePlugin{
		descriptor: descriptor("beta"),
		enable: func(pluginapi.Context) error {
			calls = append(calls, "enable:beta")
			return errors.New("boom")
		},
		disable: func(pluginapi.Context) error {
			calls = append(calls, "disable:beta")
			return nil
		},
	}
	if err := manager.Register(alpha); err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(beta); err != nil {
		t.Fatal(err)
	}

	err := manager.EnableAll()
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("EnableAll error = %v, want boom", err)
	}
	want := []string{"enable:alpha", "enable:beta", "disable:beta", "disable:alpha"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("rollback order = %v, want %v", calls, want)
	}
	if manager.Active() {
		t.Fatal("manager active after failed enable")
	}

	snapshots := manager.Snapshots()
	if snapshots[0].State != StateDisabled || snapshots[1].State != StateFailed {
		t.Fatalf("states after rollback = %s, %s", snapshots[0].State, snapshots[1].State)
	}

	calls = nil
	if err := manager.EnableAll(); !errors.Is(err, ErrPluginFailed) {
		t.Fatalf("second EnableAll = %v, want ErrPluginFailed", err)
	}
	if len(calls) != 0 {
		t.Fatalf("preflight failure executed lifecycle hooks: %v", calls)
	}
}

func TestPluginPanicBecomesLifecycleError(t *testing.T) {
	manager := New(nil)
	candidate := &fakePlugin{
		descriptor: descriptor("panic-plugin"),
		enable: func(pluginapi.Context) error {
			panic("kaboom")
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}

	err := manager.EnableAll()
	if err == nil {
		t.Fatal("panic escaped as success")
	}
	var panicErr *PanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("EnableAll error type = %T, want PanicError in chain", err)
	}
	if panicErr.PluginID != "panic-plugin" || panicErr.Phase != "enable" || len(panicErr.Stack) == 0 {
		t.Fatalf("panic diagnostics = %+v", panicErr)
	}
}

func TestRegistrationValidationDuplicateAndActiveGuard(t *testing.T) {
	manager := New(nil)
	first := &fakePlugin{descriptor: descriptor("same")}
	if err := manager.Register(first); err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(&fakePlugin{descriptor: descriptor("same")}); !errors.Is(err, ErrDuplicatePlugin) {
		t.Fatalf("duplicate registration = %v", err)
	}
	if err := manager.Register(&fakePlugin{descriptor: pluginapi.Descriptor{ID: "Bad", Name: "Bad", Version: "1", APIVersion: pluginapi.CurrentAPIVersion}}); !errors.Is(err, pluginapi.ErrInvalidID) {
		t.Fatalf("invalid descriptor registration = %v", err)
	}

	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(&fakePlugin{descriptor: descriptor("late")}); !errors.Is(err, ErrManagerActive) {
		t.Fatalf("active registration = %v, want ErrManagerActive", err)
	}
}

func TestLifecycleContextUsesHostLoggerFactory(t *testing.T) {
	logger := &recordingLogger{}
	manager := New(func(desc pluginapi.Descriptor) pluginapi.Logger {
		if desc.ID != "logger-test" {
			panic(fmt.Sprintf("unexpected descriptor %s", desc.ID))
		}
		return logger
	})
	candidate := &fakePlugin{
		descriptor: descriptor("logger-test"),
		enable: func(ctx pluginapi.Context) error {
			if ctx.Descriptor().ID != "logger-test" {
				return fmt.Errorf("context descriptor = %s", ctx.Descriptor().ID)
			}
			ctx.Logger().Info("enabled")
			return nil
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(logger.messages, []string{"info:enabled"}) {
		t.Fatalf("logger messages = %v", logger.messages)
	}
}

type recordingLogger struct {
	messages []string
}

func (l *recordingLogger) Debug(message string) { l.messages = append(l.messages, "debug:"+message) }
func (l *recordingLogger) Info(message string)  { l.messages = append(l.messages, "info:"+message) }
func (l *recordingLogger) Warn(message string)  { l.messages = append(l.messages, "warn:"+message) }
func (l *recordingLogger) Error(message string) { l.messages = append(l.messages, "error:"+message) }

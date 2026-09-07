package pluginruntime

import (
	"errors"
	"reflect"
	"testing"

	commandapi "cyu-core-26.2/api/command"
	playerapi "cyu-core-26.2/api/player"
	pluginapi "cyu-core-26.2/api/plugin"
)

type fakeCommandSource struct {
	replies []string
	player  playerapi.Snapshot
}

func (*fakeCommandSource) Kind() commandapi.SourceKind { return commandapi.SourcePlayer }
func (s *fakeCommandSource) Player() (playerapi.Snapshot, bool) {
	return s.player, true
}
func (s *fakeCommandSource) Reply(message string) { s.replies = append(s.replies, message) }

func TestPluginCommandDispatchMetricsAndDisableCleanup(t *testing.T) {
	manager := New(nil)
	candidate := &fakePlugin{
		descriptor: descriptor("command-test"),
		enable: func(ctx pluginapi.Context) error {
			_, err := ctx.Commands().Register(commandapi.Definition{
				Name:        "greet",
				Aliases:     []commandapi.Name{"hi"},
				Description: "greets the player",
			}, func(invocation commandapi.Invocation) error {
				if invocation.Name != "greet" {
					t.Fatalf("canonical name = %q, want greet", invocation.Name)
				}
				if !reflect.DeepEqual(invocation.Args, []string{"one", "two"}) {
					t.Fatalf("args = %v", invocation.Args)
				}
				invocation.Source.Reply("hello")
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

	source := &fakeCommandSource{}
	handled, err := manager.ExecuteCommand("hi", []string{"one", "two"}, source)
	if err != nil || !handled {
		t.Fatalf("ExecuteCommand = handled %v, err %v", handled, err)
	}
	if !reflect.DeepEqual(source.replies, []string{"hello"}) {
		t.Fatalf("replies = %v", source.replies)
	}

	snapshot := manager.Snapshots()[0]
	if snapshot.CommandCalls != 1 || snapshot.CommandErrors != 0 {
		t.Fatalf("command metrics = calls %d errors %d", snapshot.CommandCalls, snapshot.CommandErrors)
	}

	if err := manager.DisableAll(); err != nil {
		t.Fatal(err)
	}
	handled, err = manager.ExecuteCommand("greet", nil, source)
	if err != nil || handled {
		t.Fatalf("command survived disable: handled %v err %v", handled, err)
	}
}

func TestReservedCommandFailsEnable(t *testing.T) {
	manager := New(nil)
	if err := manager.ReserveCommands("help", "gm"); err != nil {
		t.Fatal(err)
	}
	candidate := &fakePlugin{
		descriptor: descriptor("reserved-test"),
		enable: func(ctx pluginapi.Context) error {
			_, err := ctx.Commands().Register(commandapi.Definition{Name: "help"}, func(commandapi.Invocation) error { return nil })
			return err
		},
	}
	if err := manager.Register(candidate); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); !errors.Is(err, ErrReservedCommand) {
		t.Fatalf("EnableAll = %v, want ErrReservedCommand", err)
	}
}

func TestCommandAliasConflictRollsBackEarlierPlugin(t *testing.T) {
	manager := New(nil)
	alpha := &fakePlugin{
		descriptor: descriptor("alpha-command"),
		enable: func(ctx pluginapi.Context) error {
			_, err := ctx.Commands().Register(commandapi.Definition{Name: "alpha", Aliases: []commandapi.Name{"shared"}}, func(commandapi.Invocation) error { return nil })
			return err
		},
	}
	beta := &fakePlugin{
		descriptor: descriptor("beta-command"),
		enable: func(ctx pluginapi.Context) error {
			_, err := ctx.Commands().Register(commandapi.Definition{Name: "beta", Aliases: []commandapi.Name{"shared"}}, func(commandapi.Invocation) error { return nil })
			return err
		},
	}
	if err := manager.Register(alpha); err != nil {
		t.Fatal(err)
	}
	if err := manager.Register(beta); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); !errors.Is(err, ErrCommandConflict) {
		t.Fatalf("EnableAll = %v, want ErrCommandConflict", err)
	}

	handled, err := manager.ExecuteCommand("alpha", nil, &fakeCommandSource{})
	if err != nil || handled {
		t.Fatalf("rolled-back command remained registered: handled %v err %v", handled, err)
	}
}

func TestCommandPanicIsIsolatedAndMeasured(t *testing.T) {
	manager := New(nil)
	candidate := &fakePlugin{
		descriptor: descriptor("panic-command"),
		enable: func(ctx pluginapi.Context) error {
			_, err := ctx.Commands().Register(commandapi.Definition{Name: "explode"}, func(commandapi.Invocation) error {
				panic("boom")
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

	handled, err := manager.ExecuteCommand("explode", nil, &fakeCommandSource{})
	if !handled || err == nil {
		t.Fatalf("ExecuteCommand = handled %v err %v", handled, err)
	}
	var panicErr *PanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("error type = %T, want PanicError in chain", err)
	}
	if panicErr.PluginID != "panic-command" || panicErr.Phase != "command:explode" {
		t.Fatalf("panic diagnostics = %+v", panicErr)
	}

	snapshot := manager.Snapshots()[0]
	if snapshot.CommandCalls != 1 || snapshot.CommandErrors != 1 {
		t.Fatalf("command metrics = calls %d errors %d", snapshot.CommandCalls, snapshot.CommandErrors)
	}
}

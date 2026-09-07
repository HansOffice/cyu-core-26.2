package main

import (
	"errors"
	"testing"

	commandapi "cyu-core-26.2/api/command"
	playerapi "cyu-core-26.2/api/player"
	pluginapi "cyu-core-26.2/api/plugin"
	"cyu-core-26.2/internal/pluginruntime"
)

type commandHostPlugin struct {
	calls      int
	name       commandapi.Name
	args       []string
	player     playerapi.Snapshot
	source     commandapi.Source
	registerAs commandapi.Name
}

func (p *commandHostPlugin) Descriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{
		ID:         "test.command-host",
		Name:       "Command Host",
		Version:    "1.0.0",
		APIVersion: pluginapi.CurrentAPIVersion,
	}
}

func (p *commandHostPlugin) Enable(ctx pluginapi.Context) error {
	name := p.registerAs
	if name == "" {
		name = "hello"
	}
	_, err := ctx.Commands().Register(commandapi.Definition{
		Name:    name,
		Aliases: []commandapi.Name{"hi"},
	}, func(invocation commandapi.Invocation) error {
		p.calls++
		p.name = invocation.Name
		p.args = append([]string(nil), invocation.Args...)
		p.source = invocation.Source
		player, ok := invocation.Source.Player()
		if !ok {
			return errors.New("expected player command source")
		}
		p.player = player
		invocation.Source.Reply("&ahello from plugin")
		return nil
	})
	return err
}

func (*commandHostPlugin) Disable(pluginapi.Context) error { return nil }

func TestServerDispatchesPluginCommandOnRuntimeOwner(t *testing.T) {
	server := &Server{
		world:   NewWorld(8, 65, 8),
		plugins: newPluginManager(),
	}
	server.cmdHandler = NewCommandHandler(server)

	plugin := &commandHostPlugin{}
	if err := server.RegisterPlugin(plugin); err != nil {
		t.Fatal(err)
	}
	if err := server.plugins.EnableAll(); err != nil {
		t.Fatal(err)
	}
	defer server.plugins.DisableAll()

	session := NewPlayerSession(server, nil, 1)
	session.sendChan = make(chan PacketOut, 16)
	session.username = "CommandTester"
	session.uuid = [16]byte{9, 8, 7, 6}
	session.gameMode = 1
	session.x, session.y, session.z = 12.5, 70, -4.25
	session.onGround = true
	session.setState(StatePlay)
	session.publishPlayerSnapshot()

	server.cmdHandler.Handle(session, "/hi one two")
	if plugin.calls != 1 {
		t.Fatalf("command calls = %d, want 1", plugin.calls)
	}
	if plugin.name != "hello" {
		t.Fatalf("canonical command name = %q, want hello", plugin.name)
	}
	if len(plugin.args) != 2 || plugin.args[0] != "one" || plugin.args[1] != "two" {
		t.Fatalf("command args = %v, want [one two]", plugin.args)
	}
	if plugin.player.Name != "CommandTester" || plugin.player.Position.X != 12.5 {
		t.Fatalf("player snapshot = %+v", plugin.player)
	}
	if got := len(session.sendChan); got != 1 {
		t.Fatalf("reply queued packets = %d, want 1", got)
	}

	snapshots := server.PluginSnapshots()
	if len(snapshots) != 1 || snapshots[0].CommandCalls != 1 || snapshots[0].CommandErrors != 0 {
		t.Fatalf("plugin command metrics = %+v", snapshots)
	}

	<-session.sendChan
	if _, ok := plugin.source.Player(); ok {
		t.Fatal("retained command source remained valid after handler returned")
	}
	if got := plugin.source.Kind(); got != commandapi.SourceUnknown {
		t.Fatalf("retained source kind = %v, want SourceUnknown", got)
	}
	plugin.source.Reply("late reply")
	if got := len(session.sendChan); got != 0 {
		t.Fatalf("expired source queued %d packets, want 0", got)
	}
}

func TestCoreCommandNamesAreReservedFromPlugins(t *testing.T) {
	manager := newPluginManager()
	plugin := &commandHostPlugin{registerAs: "help"}
	if err := manager.Register(plugin); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnableAll(); !errors.Is(err, pluginruntime.ErrReservedCommand) {
		t.Fatalf("EnableAll error = %v, want ErrReservedCommand", err)
	}
}

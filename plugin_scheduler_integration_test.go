package main

import (
	"testing"

	pluginapi "cyu-core-26.2/api/plugin"
)

type hostSchedulerPlugin struct {
	calls int
}

func (*hostSchedulerPlugin) Descriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{
		ID:         "test.host-scheduler",
		Name:       "Host Scheduler",
		Version:    "1.0.0",
		APIVersion: pluginapi.CurrentAPIVersion,
	}
}

func (p *hostSchedulerPlugin) Enable(ctx pluginapi.Context) error {
	_, err := ctx.Scheduler().AfterTicks(2, func() error {
		p.calls++
		return nil
	})
	return err
}

func (*hostSchedulerPlugin) Disable(pluginapi.Context) error { return nil }

func TestServerTickAdvancesPluginScheduler(t *testing.T) {
	server := &Server{
		world:   NewWorld(0, 64, 0),
		plugins: newPluginManager(),
	}
	candidate := &hostSchedulerPlugin{}
	if err := server.RegisterPlugin(candidate); err != nil {
		t.Fatal(err)
	}
	if err := server.plugins.EnableAll(); err != nil {
		t.Fatal(err)
	}
	defer server.plugins.DisableAll()

	server.tick()
	if candidate.calls != 0 {
		t.Fatalf("task ran after one tick; calls = %d", candidate.calls)
	}
	server.tick()
	if candidate.calls != 1 {
		t.Fatalf("task calls after two ticks = %d, want 1", candidate.calls)
	}

	snapshots := server.PluginSnapshots()
	if len(snapshots) != 1 || snapshots[0].TaskCalls != 1 || snapshots[0].ScheduledTasks != 0 {
		t.Fatalf("scheduler snapshot = %+v", snapshots)
	}
}

package main

import (
	"testing"

	eventapi "cyu-core-26.2/api/event"
	pluginapi "cyu-core-26.2/api/plugin"
)

type hostIntegrationPlugin struct {
	joins      int
	quits      int
	chats      []string
	cancelChat bool
}

func (*hostIntegrationPlugin) Descriptor() pluginapi.Descriptor {
	return pluginapi.Descriptor{
		ID:         "test.host-integration",
		Name:       "Host Integration",
		Version:    "1.0.0",
		APIVersion: pluginapi.CurrentAPIVersion,
	}
}

func (p *hostIntegrationPlugin) Enable(ctx pluginapi.Context) error {
	if _, err := ctx.Events().Subscribe(eventapi.PlayerJoinType, func(event eventapi.Event) error {
		joined := event.(eventapi.PlayerJoin)
		if joined.Player.Name == "PluginTester" {
			p.joins++
		}
		return nil
	}); err != nil {
		return err
	}
	if _, err := ctx.Events().Subscribe(eventapi.PlayerQuitType, func(event eventapi.Event) error {
		quit := event.(eventapi.PlayerQuit)
		if quit.Player.Name == "PluginTester" {
			p.quits++
		}
		return nil
	}); err != nil {
		return err
	}
	_, err := ctx.Events().Subscribe(eventapi.PlayerChatType, func(event eventapi.Event) error {
		chat := event.(*eventapi.PlayerChat)
		p.chats = append(p.chats, chat.Message())
		if p.cancelChat {
			chat.Cancel()
		}
		return nil
	})
	return err
}

func (*hostIntegrationPlugin) Disable(pluginapi.Context) error { return nil }

func TestServerDispatchesPublicPlayerEvents(t *testing.T) {
	server := &Server{
		world:   NewWorld(8, 65, 8),
		plugins: newPluginManager(),
	}
	server.cmdHandler = NewCommandHandler(server)

	plugin := &hostIntegrationPlugin{cancelChat: true}
	if err := server.RegisterPlugin(plugin); err != nil {
		t.Fatal(err)
	}
	if err := server.plugins.EnableAll(); err != nil {
		t.Fatal(err)
	}
	defer server.plugins.DisableAll()

	session := NewPlayerSession(server, nil, 1)
	session.sendChan = make(chan PacketOut, 1024)
	session.username = "PluginTester"
	session.uuid = [16]byte{1, 2, 3, 4}
	session.gameMode = 1
	session.x, session.y, session.z = 8, 65, 8
	session.onGround = true
	session.setState(StatePlay)
	session.publishPlayerSnapshot()

	server.AddPlayer(session)
	if plugin.joins != 1 {
		t.Fatalf("join events = %d, want 1", plugin.joins)
	}
	if got := server.GetOnlineCount(); got != 1 {
		t.Fatalf("online count = %d, want 1", got)
	}

	for len(session.sendChan) > 0 {
		<-session.sendChan
	}
	server.HandlePlayerChat(session, "cancel me")
	if len(plugin.chats) != 1 || plugin.chats[0] != "cancel me" {
		t.Fatalf("chat events = %v", plugin.chats)
	}
	if got := len(session.sendChan); got != 0 {
		t.Fatalf("cancelled chat queued %d outbound packets", got)
	}

	server.removePlayerOwned(session)
	if plugin.quits != 1 {
		t.Fatalf("quit events = %d, want 1", plugin.quits)
	}
	if got := server.GetOnlineCount(); got != 0 {
		t.Fatalf("online count after quit = %d, want 0", got)
	}
}

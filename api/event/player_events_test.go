package event

import (
	"testing"

	playerapi "cyu-core-26.2/api/player"
)

func TestPlayerEventTypes(t *testing.T) {
	player := playerapi.Snapshot{Name: "Test"}
	if got := (PlayerJoin{Player: player}).Type(); got != PlayerJoinType {
		t.Fatalf("PlayerJoin.Type() = %q", got)
	}
	if got := (PlayerQuit{Player: player}).Type(); got != PlayerQuitType {
		t.Fatalf("PlayerQuit.Type() = %q", got)
	}
	if got := NewPlayerChat(player, "hello").Type(); got != PlayerChatType {
		t.Fatalf("PlayerChat.Type() = %q", got)
	}
}

func TestPlayerChatDecisionSurface(t *testing.T) {
	event := NewPlayerChat(playerapi.Snapshot{Name: "Test"}, "hello")
	if event.Cancelled() || event.Message() != "hello" {
		t.Fatalf("initial chat = cancelled:%v message:%q", event.Cancelled(), event.Message())
	}
	event.SetMessage("changed")
	event.Cancel()
	if !event.Cancelled() || event.Message() != "changed" {
		t.Fatalf("changed chat = cancelled:%v message:%q", event.Cancelled(), event.Message())
	}
}

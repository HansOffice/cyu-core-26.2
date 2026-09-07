package main

import (
	"testing"

	"cyu-core-26.2/internal/runtime/mailbox"
	"cyu-core-26.2/internal/runtime/tick"
)

func TestClosedSessionCannotPublishPlay(t *testing.T) {
	session := NewPlayerSession(nil, nil, 1)
	session.setState(StatePlayPending)
	session.Close()

	if session.transitionState(StatePlayPending, StatePlay) {
		t.Fatal("closed session transitioned back to Play")
	}
	if got := session.State(); got != StateClosed {
		t.Fatalf("state = %v, want StateClosed", got)
	}
}

func TestPrePlayCloseDoesNotCreateLifecycleWork(t *testing.T) {
	server := &Server{runtimeMailbox: mailbox.New[runtimeEvent](4)}
	session := NewPlayerSession(server, nil, 1)
	session.username = "before-play"
	session.setState(StatePlayPending)

	session.Close()

	if got := server.runtimeMailbox.CriticalLen(); got != 0 {
		t.Fatalf("critical lifecycle events = %d, want 0", got)
	}
	if session.registered.Load() {
		t.Fatal("pre-play session became registered")
	}
}

func TestRegisteredCloseIsSerializedThroughCriticalMailbox(t *testing.T) {
	server := &Server{runtimeMailbox: mailbox.New[runtimeEvent](4)}
	server.tickLoop = tick.New(tick.DefaultRate, func() {})
	if !server.tickLoop.Start() {
		t.Fatal("tick loop did not start")
	}
	defer server.tickLoop.Stop()

	session := NewPlayerSession(server, nil, 7)
	session.username = "registered"
	session.setState(StatePlay)
	session.registered.Store(true)
	server.players.Store("registered", session)
	server.onlineCount.Store(1)

	session.Close()

	if _, ok := server.players.Load("registered"); !ok {
		t.Fatal("network close mutated player map before runtime owner drained lifecycle work")
	}
	if got := server.runtimeMailbox.CriticalLen(); got != 1 {
		t.Fatalf("critical lifecycle events = %d, want 1", got)
	}

	if drained := server.drainRuntimeEvents(1); drained != 1 {
		t.Fatalf("drained = %d, want 1", drained)
	}
	if _, ok := server.players.Load("registered"); ok {
		t.Fatal("registered player remained after lifecycle drain")
	}
	if got := server.GetOnlineCount(); got != 0 {
		t.Fatalf("online count = %d, want 0", got)
	}
	if session.registered.Load() {
		t.Fatal("registered flag remained set after removal")
	}
}

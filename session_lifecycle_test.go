package main

import (
	"net"
	"sync"
	"testing"

	"cyu-core-26.2/internal/runtime/mailbox"
	"cyu-core-26.2/internal/runtime/tick"
)

func TestPlayerSessionStateLifecycle(t *testing.T) {
	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()

	session := NewPlayerSession(nil, serverConn, 1)
	if got := session.State(); got != StateHandshake {
		t.Fatalf("initial state: want %d got %d", StateHandshake, got)
	}

	session.Close()
	if got := session.State(); got != StateClosed {
		t.Fatalf("closed state: want %d got %d", StateClosed, got)
	}
}

func TestPlayerSessionQueueOverflowClosesSlowClient(t *testing.T) {
	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()

	session := NewPlayerSession(nil, serverConn, 1)
	for i := 0; i < outboundQueueCapacity; i++ {
		if !session.SendPacket(1, []byte{byte(i)}) {
			t.Fatalf("packet %d unexpectedly rejected before queue capacity", i)
		}
	}

	if session.SendPacket(1, []byte("overflow")) {
		t.Fatal("expected overflowing packet to be rejected")
	}
	if got := session.State(); got != StateClosed {
		t.Fatalf("overflow should close session, got state %d", got)
	}
}

func TestConcurrentSendAndCloseDoesNotPanic(t *testing.T) {
	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()

	session := NewPlayerSession(nil, serverConn, 1)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 32; i++ {
				session.SendPacket(1, []byte{1, 2, 3})
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		session.Close()
	}()

	wg.Wait()
	if got := session.State(); got != StateClosed {
		t.Fatalf("expected closed session, got state %d", got)
	}
}

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

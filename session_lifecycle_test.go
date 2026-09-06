package main

import (
	"net"
	"sync"
	"testing"
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

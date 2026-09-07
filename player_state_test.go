package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"cyu-core-26.2/internal/runtime/mailbox"
)

func TestMovementDoesNotMutateAuthoritativeStateBeforeTick(t *testing.T) {
	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()

	server := &Server{
		world:          NewWorld(8, 65, 8),
		runtimeMailbox: mailbox.New[runtimeEvent](8),
	}
	session := NewPlayerSession(server, serverConn, 1)
	session.setState(StatePlay)
	session.registered.Store(true)
	session.x, session.y, session.z = 1, 70, 3
	session.publishPlayerSnapshot()

	var payload bytes.Buffer
	for _, value := range []float64{10.5, 71.25, -4.75} {
		if err := binary.Write(&payload, binary.BigEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	payload.WriteByte(1)

	session.handlePlay(PlayPktServerBoundMovePos, payload.Bytes())

	if session.x != 1 || session.y != 70 || session.z != 3 {
		t.Fatalf("network decode mutated authoritative position before tick: %.2f %.2f %.2f", session.x, session.y, session.z)
	}
	before := session.readPlayerSnapshot()
	if before.x != 1 || before.y != 70 || before.z != 3 {
		t.Fatalf("snapshot changed before tick: %+v", before)
	}
	if server.runtimeMailbox.Len() != 1 {
		t.Fatalf("runtime mailbox len = %d, want 1", server.runtimeMailbox.Len())
	}

	server.tick()

	if session.x != 10.5 || session.y != 71.25 || session.z != -4.75 || !session.onGround {
		t.Fatalf("tick did not apply movement: %.2f %.2f %.2f ground=%v", session.x, session.y, session.z, session.onGround)
	}
	after := session.readPlayerSnapshot()
	if after.x != 10.5 || after.y != 71.25 || after.z != -4.75 || !after.onGround {
		t.Fatalf("snapshot was not published by tick: %+v", after)
	}
}

func TestOwnedGameModePublishesSnapshot(t *testing.T) {
	session := &PlayerSession{}
	session.setGameModeOwned(3)
	if session.gameMode != 3 {
		t.Fatalf("authoritative game mode = %d, want 3", session.gameMode)
	}
	if got := session.readPlayerSnapshot().gameMode; got != 3 {
		t.Fatalf("snapshot game mode = %d, want 3", got)
	}
}

func TestPlayerSnapshotPublicationDoesNotAllocate(t *testing.T) {
	session := &PlayerSession{
		gameMode: 1,
		x:        10.25,
		y:        64,
		z:        -3.5,
		yaw:      90,
		pitch:    12,
		onGround: true,
	}

	allocs := testing.AllocsPerRun(1000, func() {
		session.publishPlayerSnapshot()
		_ = session.readPlayerSnapshot()
	})
	if allocs != 0 {
		t.Fatalf("snapshot publish/read allocations = %.2f, want 0", allocs)
	}
}

func TestMovementMailboxHandoffDoesNotAllocate(t *testing.T) {
	server := &Server{runtimeMailbox: mailbox.New[runtimeEvent](1)}
	session := &PlayerSession{}
	session.setState(StatePlay)
	event := runtimeEvent{
		kind:    runtimeEventPlayerMove,
		session: session,
		move:    playerMove{hasPosition: true, x: 1, y: 2, z: 3, onGround: true},
	}

	allocs := testing.AllocsPerRun(1000, func() {
		if !server.postPlayerRuntime(event) {
			panic("movement event rejected")
		}
		if _, ok := server.runtimeMailbox.TryPop(); !ok {
			panic("movement event missing")
		}
	})
	if allocs != 0 {
		t.Fatalf("movement mailbox handoff allocations = %.2f, want 0", allocs)
	}
}

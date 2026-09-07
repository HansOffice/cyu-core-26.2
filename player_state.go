package main

import (
	"fmt"
	"math"
	"sync/atomic"

	"cyu-core-26.2/internal/runtime/mailbox"
)

const (
	runtimeMailboxCapacity = 4096
	maxRuntimeTasksPerTick = 1024
)

type playerMove struct {
	hasPosition bool
	hasRotation bool
	x, y, z     float64
	yaw, pitch  float32
	onGround    bool
}

// playerSnapshot is a coherent cross-goroutine view of tick-owned gameplay
// state. It is a copy, never the authoritative mutable state itself.
type playerSnapshot struct {
	gameMode   int
	x, y, z    float64
	yaw, pitch float32
	onGround   bool
}

// playerSnapshotStore is a zero-allocation seqlock. The runtime owner is the
// only writer; readers may run on console/network goroutines. Atomics make the
// published snapshot coherent without allocating a new object on every move.
type playerSnapshotStore struct {
	sequence atomic.Uint64
	gameMode atomic.Int32
	x        atomic.Uint64
	y        atomic.Uint64
	z        atomic.Uint64
	yaw      atomic.Uint32
	pitch    atomic.Uint32
	onGround atomic.Bool
}

func (s *playerSnapshotStore) Store(snapshot playerSnapshot) {
	// Odd sequence means a write is in progress. Go atomics are sequentially
	// consistent, so readers that observe the same even sequence before and
	// after their loads have a coherent snapshot.
	s.sequence.Add(1)
	s.gameMode.Store(int32(snapshot.gameMode))
	s.x.Store(math.Float64bits(snapshot.x))
	s.y.Store(math.Float64bits(snapshot.y))
	s.z.Store(math.Float64bits(snapshot.z))
	s.yaw.Store(math.Float32bits(snapshot.yaw))
	s.pitch.Store(math.Float32bits(snapshot.pitch))
	s.onGround.Store(snapshot.onGround)
	s.sequence.Add(1)
}

func (s *playerSnapshotStore) Load() playerSnapshot {
	for {
		start := s.sequence.Load()
		if start&1 != 0 {
			continue
		}

		snapshot := playerSnapshot{
			gameMode:   int(s.gameMode.Load()),
			x:          math.Float64frombits(s.x.Load()),
			y:          math.Float64frombits(s.y.Load()),
			z:          math.Float64frombits(s.z.Load()),
			yaw:        math.Float32frombits(s.yaw.Load()),
			pitch:      math.Float32frombits(s.pitch.Load()),
			onGround:   s.onGround.Load(),
		}
		end := s.sequence.Load()
		if start == end && end&1 == 0 {
			return snapshot
		}
	}
}

func (s *PlayerSession) publishPlayerSnapshot() {
	if s == nil {
		return
	}
	s.playerSnapshot.Store(playerSnapshot{
		gameMode:   s.gameMode,
		x:          s.x,
		y:          s.y,
		z:          s.z,
		yaw:        s.yaw,
		pitch:      s.pitch,
		onGround:   s.onGround,
	})
}

func (s *PlayerSession) readPlayerSnapshot() playerSnapshot {
	if s == nil {
		return playerSnapshot{}
	}
	return s.playerSnapshot.Load()
}

func (s *Server) postRuntime(task mailbox.Task) bool {
	if s == nil || s.runtimeMailbox == nil {
		return false
	}
	return s.runtimeMailbox.TryPost(task)
}

func (s *Server) postRuntimeCritical(task mailbox.Task) bool {
	if s == nil || s.runtimeMailbox == nil {
		return false
	}
	return s.runtimeMailbox.PostCritical(task)
}

func (s *Server) postPlayerRuntime(session *PlayerSession, task func()) bool {
	if session == nil || task == nil {
		return false
	}
	if s.postRuntime(func() {
		if session.State() != StatePlay {
			return
		}
		task()
	}) {
		return true
	}

	logWarn("[runtime] mailbox overflow for %s; disconnecting client", session.remoteAddress())
	session.Close()
	return false
}

func (s *Server) applyPlayerMove(session *PlayerSession, move playerMove) {
	if session == nil || session.State() != StatePlay {
		return
	}
	if move.hasPosition {
		session.x = move.x
		session.y = move.y
		session.z = move.z
	}
	if move.hasRotation {
		session.yaw = move.yaw
		session.pitch = move.pitch
	}
	session.onGround = move.onGround

	if session.y < -10.0 {
		w := s.world
		session.Teleport(w.spawnX, w.spawnY, w.spawnZ, session.yaw, session.pitch)
		session.SendSystemMessage("&e[保护] 你已坠入虚空，已自动将你拉回出生点平台！")
		return
	}

	session.publishPlayerSnapshot()
	s.BroadcastEntityMove(session)
}

func (s *PlayerSession) setGameModeOwned(mode int) {
	s.gameMode = mode
	s.publishPlayerSnapshot()
}

// Teleport mutates tick-owned player state. Callers after Play begins must run
// on the Server runtime owner goroutine.
func (s *PlayerSession) Teleport(x, y, z float64, yaw, pitch float32) {
	if s == nil || s.server == nil {
		return
	}
	s.teleportSeq++
	s.x = x
	s.y = y
	s.z = z
	s.yaw = yaw
	s.pitch = pitch
	s.publishPlayerSnapshot()
	s.SendPacket(PlayPktClientBoundPlayerPosition, buildPlayerPositionSync(s.teleportSeq, x, y, z, yaw, pitch))
	s.server.BroadcastEntityMove(s)
}

func (s *Server) playerListLine(session *PlayerSession) string {
	snapshot := session.readPlayerSnapshot()
	return fmt.Sprintf("%s (pos: %.1f, %.1f, %.1f)", session.username, snapshot.x, snapshot.y, snapshot.z)
}

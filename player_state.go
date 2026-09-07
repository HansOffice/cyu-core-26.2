package main

import (
	"fmt"

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

// playerSnapshot is the immutable cross-goroutine view of tick-owned gameplay
// state. The authoritative fields remain on PlayerSession and are mutated only
// by the runtime owner after Play begins.
type playerSnapshot struct {
	gameMode   int
	x, y, z    float64
	yaw, pitch float32
	onGround   bool
}

func (s *PlayerSession) publishPlayerSnapshot() {
	if s == nil {
		return
	}
	s.playerSnapshot.Store(&playerSnapshot{
		gameMode: s.gameMode,
		x:        s.x,
		y:        s.y,
		z:        s.z,
		yaw:      s.yaw,
		pitch:    s.pitch,
		onGround: s.onGround,
	})
}

func (s *PlayerSession) readPlayerSnapshot() playerSnapshot {
	if s == nil {
		return playerSnapshot{}
	}
	if snapshot := s.playerSnapshot.Load(); snapshot != nil {
		return *snapshot
	}
	return playerSnapshot{}
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

	remote := "unknown"
	if session.conn != nil && session.conn.RemoteAddr() != nil {
		remote = session.conn.RemoteAddr().String()
	}
	logWarn("[runtime] mailbox overflow for %s; disconnecting client", remote)
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

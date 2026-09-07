package main

import (
	"net"
	"sync/atomic"
	"time"
)

type SessionState int32

const (
	StateHandshake   SessionState = 0
	StateStatus      SessionState = 1
	StateLogin       SessionState = 2
	StateConfig      SessionState = 3
	StatePlay        SessionState = 4
	StateClosed      SessionState = 5
	StatePlayPending SessionState = 6
)

const outboundQueueCapacity = 128

type PacketOut struct {
	ID      int
	Payload []byte
}

type PlayerSession struct {
	server            *Server
	conn              net.Conn
	state             atomic.Int32
	entityID          int
	username          string
	uuid              [16]byte
	clientProto       int
	configurationSent bool
	gameMode          int
	x, y, z           float64
	yaw, pitch        float32
	onGround          bool
	teleportSeq       int
	playerSnapshot    playerSnapshotStore
	registered        atomic.Bool
	sendChan          chan PacketOut
	done              chan struct{}
	closeOnce         atomic.Bool
	lastPingTime      int64
}

func NewPlayerSession(server *Server, conn net.Conn, entityID int) *PlayerSession {
	s := &PlayerSession{
		server:   server,
		conn:     conn,
		entityID: entityID,
		sendChan: make(chan PacketOut, outboundQueueCapacity),
		done:     make(chan struct{}),
	}
	s.setState(StateHandshake)
	return s
}

func (s *PlayerSession) State() SessionState {
	return SessionState(s.state.Load())
}

func (s *PlayerSession) setState(state SessionState) {
	s.state.Store(int32(state))
}

func (s *PlayerSession) transitionState(from, to SessionState) bool {
	return s.state.CompareAndSwap(int32(from), int32(to))
}

func (s *PlayerSession) Run() {
	go s.writeLoop()
	s.readLoop()
}

func (s *PlayerSession) SendPacket(packetID int, payload []byte) bool {
	if s == nil || s.State() == StateClosed {
		return false
	}

	packet := PacketOut{ID: packetID, Payload: payload}
	select {
	case <-s.done:
		return false
	case s.sendChan <- packet:
		return true
	default:
		logWarn("[network] outbound queue overflow for %s; disconnecting slow client", s.remoteAddress())
		s.Close()
		return false
	}
}

func (s *PlayerSession) SendSystemMessage(text string) {
	s.SendPacket(PlayPktClientBoundSystemChat, buildSystemChatMessage(text))
}

func (s *PlayerSession) writeLoop() {
	for {
		select {
		case <-s.done:
			return
		case p := <-s.sendChan:
			if err := writePacket(s.conn, p.ID, p.Payload); err != nil {
				s.Close()
				return
			}
		}
	}
}

func (s *PlayerSession) Close() {
	if s == nil || s.closeOnce.Swap(true) {
		return
	}

	s.state.Store(int32(StateClosed))
	close(s.done)
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if s.registered.Load() && s.server != nil {
		s.server.RemovePlayer(s)
	}
}

func (s *PlayerSession) readLoop() {
	defer s.Close()

	for {
		if s.conn == nil {
			return
		}
		_ = s.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		packetID, payload, err := readPacket(s.conn)
		if err != nil {
			return
		}

		switch s.State() {
		case StateHandshake:
			s.handleHandshake(packetID, payload)
		case StateStatus:
			s.handleStatus(packetID, payload)
		case StateLogin:
			s.handleLogin(packetID, payload)
		case StateConfig:
			s.handleConfig(packetID, payload)
		case StatePlay:
			s.handlePlay(packetID, payload)
		case StatePlayPending:
			// Play initialization is owned by the tick loop. Ignore packets until
			// the transition has been published as StatePlay.
		case StateClosed:
			return
		}
	}
}

func (s *PlayerSession) remoteAddress() string {
	if s == nil || s.conn == nil || s.conn.RemoteAddr() == nil {
		return "unknown"
	}
	return s.conn.RemoteAddr().String()
}

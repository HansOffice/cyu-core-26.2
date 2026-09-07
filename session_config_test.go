package main

import (
	"net"
	"testing"
)

func TestConfigurationKnownPacksOnlyAcceptedOnce(t *testing.T) {
	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()

	server := &Server{
		configuration: &vanillaConfiguration{
			registryData: [][]byte{{0x01}, {0x02}},
			updateTags:   []byte{0x03},
		},
	}
	session := NewPlayerSession(server, serverConn, 1)
	session.setState(StateConfig)

	session.handleConfig(ConfigPktServerBoundKnownPacks, nil)
	if session.State() != StateConfig {
		t.Fatalf("first KnownPacks changed state to %d", session.State())
	}
	if !session.configurationSent {
		t.Fatal("first KnownPacks did not mark configuration as sent")
	}
	if got, want := len(session.sendChan), 4; got != want {
		t.Fatalf("queued configuration packets = %d, want %d", got, want)
	}

	session.handleConfig(ConfigPktServerBoundKnownPacks, nil)
	if got := session.State(); got != StateClosed {
		t.Fatalf("duplicate KnownPacks should close session, got state %d", got)
	}
}

func TestConfigurationFinishRejectedBeforeDataSent(t *testing.T) {
	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()

	session := NewPlayerSession(&Server{}, serverConn, 1)
	session.setState(StateConfig)
	session.handleConfig(ConfigPktServerBoundFinishConfig, nil)

	if got := session.State(); got != StateClosed {
		t.Fatalf("premature FinishConfiguration should close session, got state %d", got)
	}
}

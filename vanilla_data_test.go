package main

import (
	"bytes"
	"net"
	"testing"

	v776 "cyu-core-26.2/internal/protocol/java/v776"
	"cyu-core-26.2/internal/registry"
	"cyu-core-26.2/internal/vanilla/capture"
)

func TestEmbeddedVanillaConfiguration26_2(t *testing.T) {
	configuration, err := loadVanillaConfiguration26_2()
	if err != nil {
		t.Fatal(err)
	}

	expected := v776.SynchronizedRegistryKeys()
	if len(configuration.registryData) != len(expected) {
		t.Fatalf("RegistryData packet count = %d, want %d", len(configuration.registryData), len(expected))
	}
	registries := make([]*registry.Registry, len(configuration.registryData))
	for i, payload := range configuration.registryData {
		reg, err := capture.DecodeRegistryData(payload, true)
		if err != nil {
			t.Fatalf("RegistryData packet %d: %v", i, err)
		}
		registries[i] = reg
	}
	if err := capture.ValidateRegistrySequence(registries, expected); err != nil {
		t.Fatal(err)
	}

	groups, err := capture.DecodeUpdateTags(configuration.updateTags)
	if err != nil {
		t.Fatal(err)
	}
	damageType := registry.Identifier("minecraft:damage_type")
	isFire := registry.Identifier("minecraft:is_fire")
	worldPreset := registry.Identifier("minecraft:worldgen/world_preset")
	foundFire := false
	for _, group := range groups {
		if group.Registry == worldPreset {
			t.Fatalf("non-network-safe tag registry %s was encoded", worldPreset)
		}
		if group.Registry != damageType {
			continue
		}
		for _, tag := range group.Tags {
			if tag.Key == isFire {
				foundFire = true
				break
			}
		}
	}
	if !foundFire {
		t.Fatalf("missing regression tag %s/%s", damageType, isFire)
	}
}

func TestServerQueuesValidatedConfigurationInProtocolOrder(t *testing.T) {
	configuration, err := loadVanillaConfiguration26_2()
	if err != nil {
		t.Fatal(err)
	}

	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()
	server := &Server{configuration: configuration}
	session := NewPlayerSession(server, serverConn, 1)
	defer session.Close()

	if !server.sendConfiguration(session) {
		t.Fatal("configuration sequence was not accepted by outbound queue")
	}

	for i, expectedPayload := range configuration.registryData {
		packet := <-session.sendChan
		if packet.ID != ConfigPktClientBoundRegistryData {
			t.Fatalf("packet %d ID = %#x, want RegistryData %#x", i, packet.ID, ConfigPktClientBoundRegistryData)
		}
		if !bytes.Equal(packet.Payload, expectedPayload) {
			t.Fatalf("packet %d RegistryData payload changed while queued", i)
		}
	}

	tags := <-session.sendChan
	if tags.ID != ConfigPktClientBoundUpdateTags {
		t.Fatalf("UpdateTags ID = %#x, want %#x", tags.ID, ConfigPktClientBoundUpdateTags)
	}
	if !bytes.Equal(tags.Payload, configuration.updateTags) {
		t.Fatal("UpdateTags payload changed while queued")
	}

	finish := <-session.sendChan
	if finish.ID != ConfigPktClientBoundFinishConfig {
		t.Fatalf("FinishConfiguration ID = %#x, want %#x", finish.ID, ConfigPktClientBoundFinishConfig)
	}
	if len(finish.Payload) != 0 {
		t.Fatalf("FinishConfiguration payload length = %d, want 0", len(finish.Payload))
	}
	if len(session.sendChan) != 0 {
		t.Fatalf("unexpected extra Configuration packets: %d", len(session.sendChan))
	}
}

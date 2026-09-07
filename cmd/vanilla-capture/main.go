package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"cyu-core-26.2/internal/protocol"
	v776 "cyu-core-26.2/internal/protocol/java/v776"
	"cyu-core-26.2/internal/registry"
	"cyu-core-26.2/internal/vanilla/capture"
	"cyu-core-26.2/internal/vanilla/datagen"
)

const (
	minecraftVersion = "26.2"
	dataVersion      = 4903

	handshakeNextLogin = 2

	loginClientboundDisconnect = 0x00
	loginClientboundSuccess    = 0x02
	loginClientboundCompress   = 0x03
	loginServerboundStart      = 0x00
	loginServerboundAck        = 0x03

	configServerboundClientInformation = 0x00
	configServerboundKeepAlive         = 0x04
	configServerboundPong              = 0x05
	configServerboundKnownPacks        = 0x07

	configClientboundDisconnect   = 0x02
	configClientboundFinish       = 0x03
	configClientboundKeepAlive    = 0x04
	configClientboundPing         = 0x05
	configClientboundRegistryData = 0x07
	configClientboundUpdateTags   = 0x0d
	configClientboundKnownPacks   = 0x0e
)

type options struct {
	addr            string
	reportsPath     string
	outputPath      string
	manifestPath    string
	serverJarSHA256 string
}

func main() {
	log.SetFlags(0)
	var opts options
	flag.StringVar(&opts.addr, "addr", "127.0.0.1:25565", "offline vanilla server address")
	flag.StringVar(&opts.reportsPath, "reports", "generated/reports/registries.json", "official Mojang registries.json report")
	flag.StringVar(&opts.outputPath, "out", "data/26.2/registry-set.json", "structured registry dataset output")
	flag.StringVar(&opts.manifestPath, "manifest", "data/26.2/manifest.json", "provenance manifest output")
	flag.StringVar(&opts.serverJarSHA256, "server-jar-sha256", "", "lowercase SHA-256 of the exact official server.jar")
	flag.Parse()

	if err := run(opts); err != nil {
		log.Fatal(err)
	}
}

func run(opts options) error {
	reportBytes, err := os.ReadFile(opts.reportsPath)
	if err != nil {
		return fmt.Errorf("read registries report: %w", err)
	}
	reportHash, err := datagen.SHA256Hex(bytes.NewReader(reportBytes))
	if err != nil {
		return fmt.Errorf("hash registries report: %w", err)
	}
	report, err := datagen.DecodeRegistriesReport(bytes.NewReader(reportBytes))
	if err != nil {
		return err
	}

	registries, tags, err := captureVanillaConfiguration(opts.addr)
	if err != nil {
		return err
	}
	if err := capture.ValidateRegistrySequence(registries, v776.SynchronizedRegistryKeys()); err != nil {
		return err
	}
	dataset, err := capture.BuildDataset(registries, tags, report)
	if err != nil {
		return err
	}
	encoded, set, err := capture.MarshalDataset(dataset)
	if err != nil {
		return err
	}
	if err := v776.ValidateConfigurationSet(set); err != nil {
		return fmt.Errorf("validate v776 dataset: %w", err)
	}
	worldPreset := registry.Identifier("minecraft:worldgen/world_preset")
	if _, ok := set.Tags(worldPreset); ok {
		return fmt.Errorf("captured non-network-safe tag group %s", worldPreset)
	}

	manifest := datagen.Manifest{
		Schema: datagen.ManifestSchemaVersion,
		Version: datagen.Version{
			Minecraft: minecraftVersion,
			Protocol:  int(v776.ProtocolVersion),
			Data:      dataVersion,
		},
		Source: datagen.Source{
			Kind:            datagen.MojangServerCapture,
			ServerJarSHA256: opts.serverJarSHA256,
			Command: []string{
				"go", "run", "./cmd/vanilla-capture",
				"-addr", opts.addr,
				"-reports", opts.reportsPath,
				"-out", opts.outputPath,
				"-manifest", opts.manifestPath,
				"-server-jar-sha256", opts.serverJarSHA256,
			},
		},
		Inputs: []datagen.Input{{
			Path:   "generated/reports/registries.json",
			SHA256: reportHash,
		}},
	}
	var manifestBytes bytes.Buffer
	if err := datagen.WriteManifest(&manifestBytes, manifest); err != nil {
		return err
	}
	if err := writeFileAtomic(opts.outputPath, encoded, 0o644); err != nil {
		return fmt.Errorf("write dataset: %w", err)
	}
	if err := writeFileAtomic(opts.manifestPath, manifestBytes.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	entryCount := 0
	for _, reg := range registries {
		entryCount += reg.Len()
	}
	log.Printf("captured Minecraft %s: %d synchronized registries, %d synchronized entries, %d tag groups", minecraftVersion, len(registries), entryCount, len(tags))
	return nil
}

func captureVanillaConfiguration(addr string) ([]*registry.Registry, []capture.RawTagGroup, error) {
	host, port, err := splitAddress(addr)
	if err != nil {
		return nil, nil, err
	}
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("connect vanilla server: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(45 * time.Second)); err != nil {
		return nil, nil, fmt.Errorf("set capture deadline: %w", err)
	}

	handshake := protocol.AppendVarInt(nil, v776.ProtocolVersion)
	handshake = protocol.AppendString(handshake, host)
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], port)
	handshake = append(handshake, portBytes[:]...)
	handshake = protocol.AppendVarInt(handshake, handshakeNextLogin)
	if err := writePacket(conn, 0x00, handshake); err != nil {
		return nil, nil, fmt.Errorf("send handshake: %w", err)
	}

	loginStart := protocol.AppendString(nil, "CyuCapture")
	uuid := [16]byte{0x43, 0x79, 0x75, 0x43, 0x61, 0x70, 0x74, 0x75, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	loginStart = append(loginStart, uuid[:]...)
	if err := writePacket(conn, loginServerboundStart, loginStart); err != nil {
		return nil, nil, fmt.Errorf("send login start: %w", err)
	}

	for {
		packetID, payload, err := readPacket(conn)
		if err != nil {
			return nil, nil, fmt.Errorf("read login packet: %w", err)
		}
		switch packetID {
		case loginClientboundDisconnect:
			return nil, nil, fmt.Errorf("vanilla login disconnect: %x", payload)
		case loginClientboundCompress:
			return nil, nil, fmt.Errorf("vanilla enabled network compression; capture requires network-compression-threshold=-1")
		case loginClientboundSuccess:
			if err := writePacket(conn, loginServerboundAck, nil); err != nil {
				return nil, nil, fmt.Errorf("send login acknowledged: %w", err)
			}
			return captureConfigurationState(conn)
		}
	}
}

func captureConfigurationState(conn net.Conn) ([]*registry.Registry, []capture.RawTagGroup, error) {
	clientInfo := protocol.AppendString(nil, "en_us")
	clientInfo = append(clientInfo, byte(8))
	clientInfo = protocol.AppendVarInt(clientInfo, 0)
	clientInfo = append(clientInfo, 1, 0x7f)
	clientInfo = protocol.AppendVarInt(clientInfo, 1)
	clientInfo = append(clientInfo, 0, 1)
	clientInfo = protocol.AppendVarInt(clientInfo, 0)
	if err := writePacket(conn, configServerboundClientInformation, clientInfo); err != nil {
		return nil, nil, fmt.Errorf("send client information: %w", err)
	}

	var registries []*registry.Registry
	var tags []capture.RawTagGroup
	knownPacksSelected := false
	tagsReceived := false
	for {
		packetID, payload, err := readPacket(conn)
		if err != nil {
			return nil, nil, fmt.Errorf("read configuration packet: %w", err)
		}
		switch packetID {
		case configClientboundDisconnect:
			return nil, nil, fmt.Errorf("vanilla configuration disconnect: %x", payload)
		case configClientboundKnownPacks:
			if knownPacksSelected {
				return nil, nil, fmt.Errorf("vanilla sent select-known-packs twice")
			}
			knownPacksSelected = true
			if err := writePacket(conn, configServerboundKnownPacks, protocol.AppendVarInt(nil, 0)); err != nil {
				return nil, nil, fmt.Errorf("reject known packs: %w", err)
			}
		case configClientboundKeepAlive:
			if len(payload) != 8 {
				return nil, nil, fmt.Errorf("invalid keepalive payload length %d", len(payload))
			}
			if err := writePacket(conn, configServerboundKeepAlive, payload); err != nil {
				return nil, nil, fmt.Errorf("echo keepalive: %w", err)
			}
		case configClientboundPing:
			if len(payload) != 4 {
				return nil, nil, fmt.Errorf("invalid ping payload length %d", len(payload))
			}
			if err := writePacket(conn, configServerboundPong, payload); err != nil {
				return nil, nil, fmt.Errorf("echo ping: %w", err)
			}
		case configClientboundRegistryData:
			if !knownPacksSelected {
				return nil, nil, fmt.Errorf("received RegistryData before select-known-packs negotiation")
			}
			reg, err := capture.DecodeRegistryData(payload, true)
			if err != nil {
				return nil, nil, err
			}
			registries = append(registries, reg)
		case configClientboundUpdateTags:
			if tagsReceived {
				return nil, nil, fmt.Errorf("vanilla sent UpdateTags twice")
			}
			tags, err = capture.DecodeUpdateTags(payload)
			if err != nil {
				return nil, nil, err
			}
			tagsReceived = true
		case configClientboundFinish:
			if !knownPacksSelected {
				return nil, nil, fmt.Errorf("configuration finished without known-pack negotiation")
			}
			if !tagsReceived {
				return nil, nil, fmt.Errorf("configuration finished without UpdateTags")
			}
			if err := capture.ValidateRegistrySequence(registries, v776.SynchronizedRegistryKeys()); err != nil {
				return nil, nil, err
			}
			return registries, tags, nil
		}
	}
}

func splitAddress(addr string) (string, uint16, error) {
	host, rawPort, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid server address %q: %w", addr, err)
	}
	port, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil || port == 0 {
		return "", 0, fmt.Errorf("invalid server port %q", rawPort)
	}
	return host, uint16(port), nil
}

func readPacket(r io.Reader) (int32, []byte, error) {
	return protocol.ReadPacket(r, protocol.DefaultMaxPacketSize)
}

func writePacket(w io.Writer, packetID int32, payload []byte) error {
	return protocol.WritePacket(w, packetID, payload, protocol.DefaultMaxPacketSize)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".cyucore-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}

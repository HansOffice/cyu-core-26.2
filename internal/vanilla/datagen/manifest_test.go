package datagen

import (
	"bytes"
	"strings"
	"testing"
)

const testSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func validManifest() Manifest {
	return Manifest{
		Schema: ManifestSchemaVersion,
		Version: Version{
			Minecraft: "26.2",
			Protocol:  776,
			Data:      4903,
		},
		Source: Source{
			Kind:            MojangServerDatagen,
			ServerJarSHA256: testSHA256,
			Command: []string{
				"java",
				"-DbundlerMainClass=net.minecraft.data.Main",
				"-jar",
				"server.jar",
				"--reports",
				"--output",
				"generated",
			},
		},
		Inputs: []Input{
			{Path: "reports/registries.json", SHA256: testSHA256},
			{Path: "reports/datapack.json", SHA256: testSHA256},
		},
	}
}

func TestManifestValidatesExactVersion(t *testing.T) {
	manifest := validManifest()
	if err := manifest.Validate(Version{Minecraft: "26.2", Protocol: 776, Data: 4903}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		expected Version
		want     string
	}{
		{name: "minecraft", expected: Version{Minecraft: "26.1"}, want: "Minecraft version 26.2, want 26.1"},
		{name: "protocol", expected: Version{Protocol: 775}, want: "protocol version 776, want 775"},
		{name: "data", expected: Version{Data: 4902}, want: "data version 4903, want 4902"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := manifest.Validate(test.expected)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestManifestRejectsWeakProvenance(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{name: "schema", mutate: func(m *Manifest) { m.Schema = 2 }, want: "unsupported schema"},
		{name: "source kind", mutate: func(m *Manifest) { m.Source.Kind = "third-party" }, want: "unsupported source kind"},
		{name: "uppercase hash", mutate: func(m *Manifest) { m.Source.ServerJarSHA256 = strings.ToUpper(testSHA256) }, want: "lowercase SHA-256 hex"},
		{name: "short hash", mutate: func(m *Manifest) { m.Inputs[0].SHA256 = "abcd" }, want: "expected 64 lowercase hex characters"},
		{name: "empty command", mutate: func(m *Manifest) { m.Source.Command = nil }, want: "source command is empty"},
		{name: "absolute input", mutate: func(m *Manifest) { m.Inputs[0].Path = "/reports/registries.json" }, want: "invalid path"},
		{name: "parent input", mutate: func(m *Manifest) { m.Inputs[0].Path = "../registries.json" }, want: "invalid path"},
		{name: "duplicate input", mutate: func(m *Manifest) { m.Inputs[1].Path = m.Inputs[0].Path }, want: "duplicate input path"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifest()
			test.mutate(&manifest)
			err := manifest.Validate(Version{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestWriteManifestIsDeterministic(t *testing.T) {
	manifest := validManifest()
	manifest.Inputs[0], manifest.Inputs[1] = manifest.Inputs[1], manifest.Inputs[0]

	var first bytes.Buffer
	if err := WriteManifest(&first, manifest); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := WriteManifest(&second, manifest); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("manifest output is not deterministic:\n%s\n%s", first.Bytes(), second.Bytes())
	}
	if strings.Index(first.String(), "reports/datapack.json") > strings.Index(first.String(), "reports/registries.json") {
		t.Fatalf("inputs are not canonicalized by path:\n%s", first.String())
	}
}

func TestDecodeManifestRejectsUnknownFields(t *testing.T) {
	const source = `{
		"schema":1,
		"version":{"minecraft":"26.2","protocol":776,"data":4903},
		"source":{"kind":"mojang-server-datagen","server_jar_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","command":["java"]},
		"inputs":[{"path":"reports/registries.json","sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}],
		"typo":true
	}`
	_, err := DecodeManifest(strings.NewReader(source), Version{})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v", err)
	}
}

func TestSHA256Hex(t *testing.T) {
	got, err := SHA256Hex(strings.NewReader("cyucore"))
	if err != nil {
		t.Fatal(err)
	}
	const want = "afbe2d4223f5cda33c59cff69bc864d2cf7d44d08132cab867ab5667ed9ea2be"
	if got != want {
		t.Fatalf("SHA256Hex = %s, want %s", got, want)
	}
}

package datagen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

const ManifestSchemaVersion = 1

const (
	MojangServerDatagen = "mojang-server-datagen"
	MojangServerCapture = "mojang-server-capture"
)

// Version identifies the Minecraft data contract a generated dataset targets.
type Version struct {
	Minecraft string `json:"minecraft"`
	Protocol  int    `json:"protocol"`
	Data      int    `json:"data"`
}

// Manifest records enough information to reproduce and audit generated vanilla
// data. Hashes are SHA-256 over the exact input bytes consumed by the importer.
type Manifest struct {
	Schema  int     `json:"schema"`
	Version Version `json:"version"`
	Source  Source  `json:"source"`
	Inputs  []Input `json:"inputs"`
}

type Source struct {
	Kind            string   `json:"kind"`
	ServerJarSHA256 string   `json:"server_jar_sha256"`
	Command         []string `json:"command"`
}

type Input struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// DecodeManifest decodes and validates one provenance manifest. Unknown JSON
// fields are rejected so provenance schema changes are explicit migrations.
func DecodeManifest(r io.Reader, expected Version) (*Manifest, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("datagen manifest: decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("datagen manifest: multiple JSON values")
		}
		return nil, fmt.Errorf("datagen manifest: trailing data: %w", err)
	}
	if err := manifest.Validate(expected); err != nil {
		return nil, err
	}
	return cloneManifest(manifest), nil
}

// Validate checks the manifest itself and, when expected contains non-zero
// fields, verifies it targets the exact Minecraft/protocol/data version.
func (m Manifest) Validate(expected Version) error {
	if m.Schema != ManifestSchemaVersion {
		return fmt.Errorf("datagen manifest: unsupported schema %d", m.Schema)
	}
	if strings.TrimSpace(m.Version.Minecraft) == "" || m.Version.Protocol <= 0 || m.Version.Data <= 0 {
		return fmt.Errorf("datagen manifest: invalid version %+v", m.Version)
	}
	if expected.Minecraft != "" && m.Version.Minecraft != expected.Minecraft {
		return fmt.Errorf("datagen manifest: Minecraft version %s, want %s", m.Version.Minecraft, expected.Minecraft)
	}
	if expected.Protocol != 0 && m.Version.Protocol != expected.Protocol {
		return fmt.Errorf("datagen manifest: protocol version %d, want %d", m.Version.Protocol, expected.Protocol)
	}
	if expected.Data != 0 && m.Version.Data != expected.Data {
		return fmt.Errorf("datagen manifest: data version %d, want %d", m.Version.Data, expected.Data)
	}

	if m.Source.Kind != MojangServerDatagen && m.Source.Kind != MojangServerCapture {
		return fmt.Errorf("datagen manifest: unsupported source kind %q", m.Source.Kind)
	}
	if err := validateSHA256(m.Source.ServerJarSHA256); err != nil {
		return fmt.Errorf("datagen manifest: server jar hash: %w", err)
	}
	if len(m.Source.Command) == 0 {
		return fmt.Errorf("datagen manifest: source command is empty")
	}
	for index, argument := range m.Source.Command {
		if argument == "" {
			return fmt.Errorf("datagen manifest: source command argument %d is empty", index)
		}
	}

	if len(m.Inputs) == 0 {
		return fmt.Errorf("datagen manifest: no inputs")
	}
	seenPaths := make(map[string]struct{}, len(m.Inputs))
	for index, input := range m.Inputs {
		if !validRelativePath(input.Path) {
			return fmt.Errorf("datagen manifest: input %d has invalid path %q", index, input.Path)
		}
		if _, exists := seenPaths[input.Path]; exists {
			return fmt.Errorf("datagen manifest: duplicate input path %q", input.Path)
		}
		seenPaths[input.Path] = struct{}{}
		if err := validateSHA256(input.SHA256); err != nil {
			return fmt.Errorf("datagen manifest: input %s hash: %w", input.Path, err)
		}
	}
	return nil
}

// WriteManifest emits a deterministic, human-reviewable manifest. Input order
// is canonicalized by path; callers cannot make equivalent manifests differ
// merely because filesystem traversal order changed.
func WriteManifest(w io.Writer, manifest Manifest) error {
	if err := manifest.Validate(Version{}); err != nil {
		return err
	}
	cloned := cloneManifest(manifest)
	sort.Slice(cloned.Inputs, func(i, j int) bool {
		return cloned.Inputs[i].Path < cloned.Inputs[j].Path
	})
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(cloned); err != nil {
		return fmt.Errorf("datagen manifest: encode: %w", err)
	}
	return nil
}

// SHA256Hex hashes an exact input stream for inclusion in a provenance manifest.
func SHA256Hex(r io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateSHA256(value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("expected %d lowercase hex characters", sha256.Size*2)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || strings.ToLower(value) != value {
		return fmt.Errorf("expected lowercase SHA-256 hex")
	}
	return nil
}

func validRelativePath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return false
	}
	cleaned := path.Clean(value)
	return cleaned == value && cleaned != "." && cleaned != ".." && !strings.HasPrefix(cleaned, "../")
}

func cloneManifest(value Manifest) *Manifest {
	cloned := value
	cloned.Source.Command = append([]string(nil), value.Source.Command...)
	cloned.Inputs = append([]Input(nil), value.Inputs...)
	return &cloned
}

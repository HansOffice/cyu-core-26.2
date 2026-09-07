// Package plugin defines the stable Go-facing lifecycle contract for CyuCore
// plugins. It intentionally contains no server internals, protocol types, or
// loader-specific concepts.
package plugin

import (
	"errors"
	"fmt"
	"strings"

	eventapi "cyu-core-26.2/api/event"
)

const (
	// CurrentAPIVersion is the compatibility version of the public plugin API.
	// It is independent from both the CyuCore release version and the Minecraft
	// protocol version.
	CurrentAPIVersion uint32 = 1

	maxIDLength      = 64
	maxNameLength    = 128
	maxVersionLength = 64
)

var (
	ErrInvalidID       = errors.New("plugin: invalid id")
	ErrInvalidMetadata = errors.New("plugin: invalid metadata")
	ErrIncompatibleAPI = errors.New("plugin: incompatible api version")
)

// ID is the stable machine-readable identity of a plugin. IDs are lowercase
// ASCII and remain stable across plugin display-name/version changes.
type ID string

// ParseID validates a plugin identifier. The first and last characters must be
// alphanumeric; internal '.', '_' and '-' separators are allowed.
func ParseID(raw string) (ID, error) {
	if len(raw) == 0 || len(raw) > maxIDLength {
		return "", fmt.Errorf("%w: length must be 1..%d", ErrInvalidID, maxIDLength)
	}
	for index := 0; index < len(raw); index++ {
		ch := raw[index]
		alphaNumeric := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		separator := ch == '.' || ch == '_' || ch == '-'
		if !alphaNumeric && !separator {
			return "", fmt.Errorf("%w: %q contains unsupported character %q", ErrInvalidID, raw, ch)
		}
		if (index == 0 || index == len(raw)-1) && !alphaNumeric {
			return "", fmt.Errorf("%w: %q must start and end with [a-z0-9]", ErrInvalidID, raw)
		}
	}
	return ID(raw), nil
}

// Descriptor is immutable plugin identity/compatibility metadata from the
// host's perspective. Version is intentionally opaque at API v1; loaders may
// impose additional packaging/version policies without changing this contract.
type Descriptor struct {
	ID         ID
	Name       string
	Version    string
	APIVersion uint32
}

// Validate checks only host-level identity and API compatibility invariants.
func (d Descriptor) Validate() error {
	id, err := ParseID(string(d.ID))
	if err != nil {
		return err
	}
	if id != d.ID {
		return fmt.Errorf("%w: plugin id changed during validation", ErrInvalidMetadata)
	}
	name := strings.TrimSpace(d.Name)
	if name == "" || len(name) > maxNameLength {
		return fmt.Errorf("%w: name length must be 1..%d after trimming", ErrInvalidMetadata, maxNameLength)
	}
	version := strings.TrimSpace(d.Version)
	if version == "" || len(version) > maxVersionLength {
		return fmt.Errorf("%w: version length must be 1..%d after trimming", ErrInvalidMetadata, maxVersionLength)
	}
	if d.APIVersion != CurrentAPIVersion {
		return fmt.Errorf("%w: plugin %q requests API %d, host supports %d", ErrIncompatibleAPI, d.ID, d.APIVersion, CurrentAPIVersion)
	}
	return nil
}

// Logger is the minimal logging surface guaranteed by API v1. Plugin runtimes
// may enrich log records internally, but plugins do not receive the server's
// concrete logger implementation.
type Logger interface {
	Debug(message string)
	Info(message string)
	Warn(message string)
	Error(message string)
}

// Context is a host-owned capability container passed to plugin lifecycle
// hooks. Additional API services are added through this public boundary rather
// than exposing internal Server/PlayerSession structures.
type Context interface {
	Descriptor() Descriptor
	Logger() Logger
	Events() eventapi.Registrar
}

// Plugin is the loader-independent lifecycle contract. Dynamic runtimes such as
// WASM adapt their module ABI to this semantic contract; they do not redefine
// server lifecycle semantics.
type Plugin interface {
	Descriptor() Descriptor
	Enable(Context) error
	Disable(Context) error
}

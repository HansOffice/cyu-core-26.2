// Package command defines CyuCore's public plugin command contract.
// Commands are dispatched by the server runtime owner; plugins only receive a
// registration capability and public source snapshots.
package command

import (
	"errors"
	"fmt"
	"strings"

	playerapi "cyu-core-26.2/api/player"
)

const (
	maxNameLength        = 48
	maxDescriptionLength = 256
)

var (
	ErrInvalidName        = errors.New("command: invalid name")
	ErrInvalidDefinition  = errors.New("command: invalid definition")
	ErrNilHandler         = errors.New("command: nil handler")
	ErrNilSource          = errors.New("command: nil source")
)

// Name is a canonical lower-case command literal without a leading slash.
type Name string

// ParseName validates a command literal. CyuCore keeps command identity ASCII
// and case-stable so aliases remain deterministic across loaders and locales.
func ParseName(raw string) (Name, error) {
	if len(raw) == 0 || len(raw) > maxNameLength {
		return "", fmt.Errorf("%w: length must be 1..%d", ErrInvalidName, maxNameLength)
	}
	for index := 0; index < len(raw); index++ {
		ch := raw[index]
		alphaNumeric := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		separator := ch == '_' || ch == '-'
		if !alphaNumeric && !separator {
			return "", fmt.Errorf("%w: %q contains unsupported character %q", ErrInvalidName, raw, ch)
		}
		if (index == 0 || index == len(raw)-1) && !alphaNumeric {
			return "", fmt.Errorf("%w: %q must start and end with [a-z0-9]", ErrInvalidName, raw)
		}
	}
	return Name(raw), nil
}

// Definition is immutable registration metadata. Aliases are peers of Name for
// dispatch purposes, but Name is the stable canonical identity shown in health
// and diagnostic surfaces.
type Definition struct {
	Name        Name
	Aliases     []Name
	Description string
}

func (d Definition) Validate() error {
	name, err := ParseName(string(d.Name))
	if err != nil {
		return err
	}
	if name != d.Name {
		return fmt.Errorf("%w: canonical name changed during validation", ErrInvalidDefinition)
	}

	description := strings.TrimSpace(d.Description)
	if len(description) > maxDescriptionLength {
		return fmt.Errorf("%w: description exceeds %d bytes", ErrInvalidDefinition, maxDescriptionLength)
	}

	seen := map[Name]struct{}{d.Name: {}}
	for _, alias := range d.Aliases {
		parsed, err := ParseName(string(alias))
		if err != nil {
			return err
		}
		if parsed != alias {
			return fmt.Errorf("%w: alias changed during validation", ErrInvalidDefinition)
		}
		if _, exists := seen[alias]; exists {
			return fmt.Errorf("%w: duplicate literal %q", ErrInvalidDefinition, alias)
		}
		seen[alias] = struct{}{}
	}
	return nil
}

// SourceKind identifies the public command source category without exposing
// internal console/session implementations.
type SourceKind uint8

const (
	SourceUnknown SourceKind = iota
	SourcePlayer
	SourceConsole
)

// Source is valid only for the synchronous duration of a command handler.
// Reply is intentionally the only imperative capability in API v1; gameplay
// mutations will be exposed through dedicated public services rather than by
// leaking PlayerSession or Server.
type Source interface {
	Kind() SourceKind
	Player() (playerapi.Snapshot, bool)
	Reply(message string)
}

// Invocation is one parsed command invocation. Args excludes the command name.
type Invocation struct {
	Name   Name
	Args   []string
	Source Source
}

// Handler executes synchronously on CyuCore's runtime owner. It must not block
// on disk/network I/O. Errors are isolated and recorded by pluginruntime.
type Handler func(Invocation) error

// Registration controls one command registration. Unregister is idempotent.
type Registration interface {
	Unregister() bool
}

// Registrar is scoped to one plugin owner. Registrations made during Enable are
// automatically removed on Disable or failed-enable rollback.
type Registrar interface {
	Register(definition Definition, handler Handler) (Registration, error)
}

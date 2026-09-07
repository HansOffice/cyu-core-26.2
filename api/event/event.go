// Package event defines the public plugin event subscription contract. Plugins
// can subscribe to events; only CyuCore's internal runtime owns dispatch.
package event

import (
	"errors"
	"fmt"
)

const maxTypeLength = 96

var (
	ErrInvalidType = errors.New("event: invalid type")
	ErrNilHandler  = errors.New("event: nil handler")
)

// Type is the stable identity of an event contract. Event names use lowercase
// namespaced identifiers such as "cyucore:player_join".
type Type string

func ParseType(raw string) (Type, error) {
	if len(raw) == 0 || len(raw) > maxTypeLength {
		return "", fmt.Errorf("%w: length must be 1..%d", ErrInvalidType, maxTypeLength)
	}

	colon := -1
	for index := 0; index < len(raw); index++ {
		ch := raw[index]
		switch {
		case ch == ':':
			if colon >= 0 {
				return "", fmt.Errorf("%w: %q contains multiple namespace separators", ErrInvalidType, raw)
			}
			colon = index
		case ch >= 'a' && ch <= 'z':
		case ch >= '0' && ch <= '9':
		case ch == '.', ch == '_', ch == '-', ch == '/':
		default:
			return "", fmt.Errorf("%w: %q contains unsupported character %q", ErrInvalidType, raw, ch)
		}
	}
	if colon <= 0 || colon == len(raw)-1 {
		return "", fmt.Errorf("%w: %q must be namespaced as namespace:path", ErrInvalidType, raw)
	}
	return Type(raw), nil
}

// Event is the public event contract. Most concrete events are immutable value
// notifications; decision events may expose narrowly documented mutation such
// as Cancel or SetMessage. Plugins never receive dispatch authority through it.
type Event interface {
	Type() Type
}

// Handler runs synchronously on the server runtime owner unless a concrete
// event contract explicitly documents otherwise. Handlers must not block on
// network/disk I/O. Returning an error records a plugin failure for that event
// invocation but does not prevent later handlers from running.
type Handler func(Event) error

// Subscription controls one handler registration. Cancel is idempotent. A
// concurrent dispatch that already loaded the old immutable handler table may
// complete the current invocation; cancellation affects subsequent dispatches.
type Subscription interface {
	Cancel() bool
}

// Registrar is the only event-bus capability exposed to plugins. It does not
// expose dispatch, internal runtime ownership, or other plugins' subscriptions.
type Registrar interface {
	Subscribe(Type, Handler) (Subscription, error)
}

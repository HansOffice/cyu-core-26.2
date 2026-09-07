package event

import playerapi "cyu-core-26.2/api/player"

const (
	PlayerJoinType Type = "cyucore:player_join"
	PlayerQuitType Type = "cyucore:player_quit"
	PlayerChatType Type = "cyucore:player_chat"
)

// PlayerJoin is emitted after the player has been published into the runtime's
// active player set. It is a notification event and cannot cancel the join.
type PlayerJoin struct {
	Player playerapi.Snapshot
}

func (PlayerJoin) Type() Type { return PlayerJoinType }

// PlayerQuit is emitted by the runtime owner after the player has been removed
// from the active player set. The snapshot represents the last published state.
type PlayerQuit struct {
	Player playerapi.Snapshot
}

func (PlayerQuit) Type() Type { return PlayerQuitType }

// PlayerChat is a runtime-owner decision event. Handlers may replace the
// message or cancel broadcast through the narrow methods below; they never
// receive the internal PlayerSession or chat packet bytes.
type PlayerChat struct {
	Player playerapi.Snapshot

	message   string
	cancelled bool
}

func NewPlayerChat(player playerapi.Snapshot, message string) *PlayerChat {
	return &PlayerChat{Player: player, message: message}
}

func (*PlayerChat) Type() Type { return PlayerChatType }

func (e *PlayerChat) Message() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *PlayerChat) SetMessage(message string) {
	if e != nil {
		e.message = message
	}
}

func (e *PlayerChat) Cancel() {
	if e != nil {
		e.cancelled = true
	}
}

func (e *PlayerChat) Cancelled() bool {
	return e != nil && e.cancelled
}

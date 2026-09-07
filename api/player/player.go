// Package player defines stable plugin-facing player value types. It contains
// snapshots and identifiers, never internal session/network objects.
package player

// UUID is a Minecraft player UUID represented independently from protocol wire
// encoding and internal session storage.
type UUID [16]byte

func (u UUID) String() string {
	const hex = "0123456789abcdef"
	var out [36]byte
	groups := [...]int{4, 6, 8, 10}
	groupIndex := 0
	outIndex := 0
	for byteIndex, value := range u {
		if groupIndex < len(groups) && byteIndex == groups[groupIndex] {
			out[outIndex] = '-'
			outIndex++
			groupIndex++
		}
		out[outIndex] = hex[value>>4]
		out[outIndex+1] = hex[value&0x0f]
		outIndex += 2
	}
	return string(out[:])
}

// GameMode is gameplay state, not a protocol numeric ID.
type GameMode uint8

const (
	GameModeSurvival GameMode = iota
	GameModeCreative
	GameModeAdventure
	GameModeSpectator
)

func (g GameMode) Valid() bool { return g <= GameModeSpectator }

func (g GameMode) String() string {
	switch g {
	case GameModeSurvival:
		return "survival"
	case GameModeCreative:
		return "creative"
	case GameModeAdventure:
		return "adventure"
	case GameModeSpectator:
		return "spectator"
	default:
		return "unknown"
	}
}

// Position is a coherent value copied from the runtime-owned player state.
type Position struct {
	X        float64
	Y        float64
	Z        float64
	Yaw      float32
	Pitch    float32
	OnGround bool
}

// Snapshot is an immutable-by-convention view captured by the core for plugin
// callbacks. Mutating a local copy cannot mutate authoritative server state.
type Snapshot struct {
	UUID     UUID
	Name     string
	GameMode GameMode
	Position Position
}

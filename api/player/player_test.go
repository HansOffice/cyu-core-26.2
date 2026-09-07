package player

import "testing"

func TestUUIDString(t *testing.T) {
	uuid := UUID{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}
	if got, want := uuid.String(), "12345678-9abc-def0-1234-56789abcdef0"; got != want {
		t.Fatalf("UUID.String() = %q, want %q", got, want)
	}
}

func TestGameModeStringAndValidation(t *testing.T) {
	tests := []struct {
		mode GameMode
		name string
	}{
		{GameModeSurvival, "survival"},
		{GameModeCreative, "creative"},
		{GameModeAdventure, "adventure"},
		{GameModeSpectator, "spectator"},
	}
	for _, test := range tests {
		if !test.mode.Valid() || test.mode.String() != test.name {
			t.Fatalf("mode %d = valid:%v string:%q", test.mode, test.mode.Valid(), test.mode.String())
		}
	}
	if invalid := GameMode(4); invalid.Valid() || invalid.String() != "unknown" {
		t.Fatalf("invalid mode = valid:%v string:%q", invalid.Valid(), invalid.String())
	}
}

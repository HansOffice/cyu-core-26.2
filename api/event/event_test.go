package event

import (
	"errors"
	"testing"
)

func TestParseType(t *testing.T) {
	valid := []string{
		"cyucore:player_join",
		"cyucore:block/break",
		"example.mod:custom-event",
	}
	for _, raw := range valid {
		got, err := ParseType(raw)
		if err != nil {
			t.Fatalf("ParseType(%q): %v", raw, err)
		}
		if string(got) != raw {
			t.Fatalf("ParseType(%q) = %q", raw, got)
		}
	}

	invalid := []string{"", "player_join", ":join", "core:", "Core:join", "a:b:c", "a:玩家"}
	for _, raw := range invalid {
		if _, err := ParseType(raw); !errors.Is(err, ErrInvalidType) {
			t.Fatalf("ParseType(%q) error = %v, want ErrInvalidType", raw, err)
		}
	}
}

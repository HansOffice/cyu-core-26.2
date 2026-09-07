package command

import "testing"

func TestDefinitionValidation(t *testing.T) {
	valid := Definition{Name: "hello", Aliases: []Name{"hi", "hello-world"}, Description: "test"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid definition rejected: %v", err)
	}

	cases := []Definition{
		{Name: "Hello"},
		{Name: "-hello"},
		{Name: "hello", Aliases: []Name{"hello"}},
		{Name: "hello", Aliases: []Name{"BAD"}},
	}
	for _, candidate := range cases {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid definition accepted: %+v", candidate)
		}
	}
}

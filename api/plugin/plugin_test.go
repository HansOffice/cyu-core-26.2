package plugin

import (
	"errors"
	"testing"
)

func TestParseID(t *testing.T) {
	valid := []string{
		"hello",
		"hello-world",
		"hello_world",
		"com.example.plugin",
		"plugin2",
	}
	for _, raw := range valid {
		got, err := ParseID(raw)
		if err != nil {
			t.Fatalf("ParseID(%q): %v", raw, err)
		}
		if string(got) != raw {
			t.Fatalf("ParseID(%q) = %q", raw, got)
		}
	}

	invalid := []string{"", "Hello", "-hello", "hello-", "hello world", "插件"}
	for _, raw := range invalid {
		if _, err := ParseID(raw); err == nil {
			t.Fatalf("ParseID(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestDescriptorValidation(t *testing.T) {
	valid := Descriptor{
		ID:         "com.example.hello",
		Name:       "Hello",
		Version:    "1.0.0",
		APIVersion: CurrentAPIVersion,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid descriptor rejected: %v", err)
	}

	tests := []struct {
		name string
		desc Descriptor
		want error
	}{
		{name: "id", desc: Descriptor{ID: "Bad", Name: "Hello", Version: "1", APIVersion: CurrentAPIVersion}, want: ErrInvalidID},
		{name: "name", desc: Descriptor{ID: "hello", Name: " ", Version: "1", APIVersion: CurrentAPIVersion}, want: ErrInvalidMetadata},
		{name: "version", desc: Descriptor{ID: "hello", Name: "Hello", Version: "", APIVersion: CurrentAPIVersion}, want: ErrInvalidMetadata},
		{name: "api", desc: Descriptor{ID: "hello", Name: "Hello", Version: "1", APIVersion: CurrentAPIVersion + 1}, want: ErrIncompatibleAPI},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.desc.Validate()
			if !errors.Is(err, test.want) {
				t.Fatalf("Validate() = %v, want errors.Is(%v)", err, test.want)
			}
		})
	}
}

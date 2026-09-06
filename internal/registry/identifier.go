package registry

import (
	"fmt"
	"strings"
)

// Identifier is a fully-qualified Minecraft resource location.
// CyuCore's structured data layer requires an explicit namespace so versioned
// data never relies on an implicit default.
type Identifier string

func ParseIdentifier(raw string) (Identifier, error) {
	colon := strings.IndexByte(raw, ':')
	if colon <= 0 || colon == len(raw)-1 || strings.IndexByte(raw[colon+1:], ':') >= 0 {
		return "", fmt.Errorf("registry: invalid identifier %q: expected namespace:path", raw)
	}

	namespace := raw[:colon]
	path := raw[colon+1:]
	for _, r := range namespace {
		if !isNamespaceRune(r) {
			return "", fmt.Errorf("registry: invalid identifier %q: invalid namespace character %q", raw, r)
		}
	}
	for _, r := range path {
		if !isPathRune(r) {
			return "", fmt.Errorf("registry: invalid identifier %q: invalid path character %q", raw, r)
		}
	}
	return Identifier(raw), nil
}

func (i Identifier) String() string {
	return string(i)
}

func validateIdentifier(i Identifier) error {
	parsed, err := ParseIdentifier(i.String())
	if err != nil {
		return err
	}
	if parsed != i {
		return fmt.Errorf("registry: invalid identifier %q", i)
	}
	return nil
}

func isNamespaceRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.'
}

func isPathRune(r rune) bool {
	return isNamespaceRune(r) || r == '/'
}

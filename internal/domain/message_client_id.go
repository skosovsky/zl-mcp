package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const MessageClientIDLimit = 64

// ValidMessageClientID accepts an exact bounded opaque identifier, not text or
// a normalized approximation that could collapse different native identities.
func ValidMessageClientID(id string) bool {
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > MessageClientIDLimit ||
		strings.TrimSpace(id) != id {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

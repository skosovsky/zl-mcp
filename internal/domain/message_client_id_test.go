package domain

import (
	"strings"
	"testing"
)

func TestMessageClientIDRequiresExactBoundedValue(t *testing.T) {
	for _, tc := range []struct {
		id    string
		valid bool
	}{
		{"123", true}, {strings.Repeat("x", 64), true}, {strings.Repeat("x", 65), false},
		{"", false}, {" 123", false}, {"123\n", false}, {"x\x00", false}, {"\xff", false},
	} {
		// Arrange / Act: validation never trims or truncates an identifier.
		got := ValidMessageClientID(tc.id)
		// Assert.
		if got != tc.valid {
			t.Fatal("invalid client ID boundary behavior")
		}
	}
}

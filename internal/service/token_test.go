package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateTokenCreationAndReuse(t *testing.T) {
	// Arrange
	path := filepath.Join(t.TempDir(), "token")
	// Act
	first, err := tokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: restart preserves identity and creates no world-readable credential.
	if first != second || len(first) != 64 || info.Mode().Perm() != 0600 {
		t.Fatal("invalid token lifecycle")
	}
}
func TestPrivateTokenRejectsUnsafeExistingFile(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		mode       os.FileMode
	}{{"public", strings.Repeat("a", 64), 0644}, {"short", "abc", 0600}, {"spaces", strings.Repeat("a", 32) + " x", 0600}} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "token")
			if err := os.WriteFile(path, []byte(tc.text), tc.mode); err != nil {
				t.Fatal(err)
			}
			// Act
			_, err := tokenFile(path)
			saved, readErr := os.ReadFile(path)
			// Assert: bad credentials are rejected without silent regeneration.
			if err == nil || readErr != nil || string(saved) != tc.text {
				t.Fatalf("err=%v read=%v", err, readErr)
			}
		})
	}
}

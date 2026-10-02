package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceTransportConfiguration(t *testing.T) {
	for _, tc := range []struct {
		listen string
		valid  bool
	}{{"127.0.0.1:18765", true}, {"[::1]:18765", true}, {"0.0.0.0:18765", false}, {"192.168.1.1:18765", false}, {"localhost:18765", false}, {"127.0.0.1:0", false}, {"127.0.0.1:65536", false}} {
		t.Run(tc.listen, func(t *testing.T) {
			// Arrange
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte("state_dir = 'state'\n[mcp]\nlisten = '"+tc.listen+"'\ntoken_file = 'token'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			// Act
			c, err := Load(path)
			// Assert
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if err == nil && c.MCP.TokenFile != filepath.Join(filepath.Dir(path), "token") {
				t.Fatal("relative token path resolved incorrectly")
			}
		})
	}
}

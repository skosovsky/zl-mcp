package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitLocalCallbackConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, suffix string
		valid        bool
	}{
		{"default", "", true},
		{"enabled", "[events]\nlocal_callback_urls=['http://127.0.0.1:19001/events','https://[::1]:19002/events']\n", true},
		{"duplicate", "[events]\nlocal_callback_urls=['http://127.0.0.1:19001/events','http://127.0.0.1:19001/events']\n", false},
		{"hostname", "[events]\nlocal_callback_urls=['http://localhost:19001/events']\n", false},
		{"public", "[events]\nlocal_callback_urls=['https://example.com/events']\n", false},
		{"port", "[events]\nlocal_callback_urls=['http://127.0.0.1:65536/events']\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			p := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(p, []byte("state_dir='state'\n"+tc.suffix), 0600); err != nil {
				t.Fatal(err)
			}
			// Act.
			c, err := Load(p)
			// Assert.
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.name == "default" && len(c.Events.LocalCallbackURLs) != 0 {
				t.Fatal("local callbacks enabled by default")
			}
		})
	}
}

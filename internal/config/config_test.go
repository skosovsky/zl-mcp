package config

import (
	"fmt"
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

func TestFullEventTraceRequiresSeparateAbsoluteFile(t *testing.T) {
	for _, tc := range []struct {
		name, trace string
		valid       bool
	}{
		{"disabled", "", true}, {"private absolute", "/tmp/zl-event-trace.jsonl", true},
		{"relative", "trace.jsonl", false}, {"same as service log", "/tmp/zl-service.log", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			path := filepath.Join(t.TempDir(), "config.toml")
			content := fmt.Sprintf("state_dir = 'state'\n[logging]\nfile = '/tmp/zl-service.log'\nevent_trace_file = %q\n", tc.trace)
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			// Act.
			c, err := Load(path)
			// Assert.
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if err == nil && c.Logging.EventTraceFile != tc.trace {
				t.Fatal("trace option changed")
			}
		})
	}
}

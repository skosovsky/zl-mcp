package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupDiagnosticIsPrivateAndContainsNoRawError(t *testing.T) {
	// Arrange: fail before the service logger exists, with adversarial private text.
	path := filepath.Join(t.TempDir(), "logs", "startup.log")
	var fallback bytes.Buffer
	// Act
	handled := reportStartupFailure(path, errors.New("invalid config TOML: PRIVATE_SIGNING_SECRET"), &fallback)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err = json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	// Assert: diagnosis survives startup failure without exposing configuration.
	info, _ := os.Stat(path)
	if !handled || fallback.Len() != 0 || record["category"] != "invalid_config" || strings.Contains(string(body), "PRIVATE_") || info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe startup diagnostic: %s", body)
	}
}
func TestStartupDiagnosticSurvivesUnavailableSink(t *testing.T) {
	// Arrange: an impossible parent path for a log file.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	os.WriteFile(blocker, nil, 0600)
	var fallback bytes.Buffer
	// Act
	handled := reportStartupFailure(filepath.Join(blocker, "startup.log"), os.ErrPermission, &fallback)
	// Assert: bounded JSON fallback remains available, never silent.
	var record map[string]any
	if handled || json.Unmarshal(fallback.Bytes(), &record) != nil || record["category"] != "permission_denied" {
		t.Fatal("startup diagnostic was lost")
	}
}

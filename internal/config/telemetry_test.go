package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemetryConfigDefaultsAndOptInPrivacy(t *testing.T) {
	// Arrange: installed configuration has no telemetry fields.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("state_dir = \"state\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Act.
	old, err := Load(path)
	// Assert: disabled and no network endpoint/header configuration.
	if err != nil || old.Telemetry.Enabled || old.Telemetry.Exporter != "none" || old.Telemetry.Endpoint != "" ||
		len(old.Telemetry.Headers) != 0 {
		t.Fatal(old.Telemetry, err)
	}
	// Arrange: explicit valid private exporter settings.
	body := "state_dir = \"state\"\n[telemetry]\nenabled = true\nexporter = \"otlp_http\"\nendpoint = \"http://127.0.0.1:4318\"\n[telemetry.headers]\nAuthorization = \"SENTINEL_TELEMETRY_SECRET\"\n"
	if err = os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	// Act.
	opted, err := Load(path)
	// Assert: secrets remain configuration; error messages never include them.
	if err != nil || !opted.Telemetry.Enabled ||
		opted.Telemetry.Headers["Authorization"] != "SENTINEL_TELEMETRY_SECRET" {
		t.Fatal("valid telemetry opt-in rejected")
	}
	for _, bad := range []string{
		strings.Replace(body, "http://127.0.0.1:4318", "http://remote.invalid", 1),
		strings.Replace(body, "http://127.0.0.1:4318", "https://SENTINEL_TELEMETRY_SECRET@remote.invalid", 1),
		strings.Replace(body, "http://127.0.0.1:4318", "https://remote.invalid?secret=SENTINEL_TELEMETRY_SECRET", 1),
		strings.Replace(body, "enabled = true", "enabled = false", 1),
		body + "Broken = [SENTINEL_TELEMETRY_SECRET\n",
	} {
		if err = os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		_, err = Load(path)
		if err == nil || strings.Contains(err.Error(), "SENTINEL_TELEMETRY_SECRET") {
			t.Fatalf("configuration accepted or leaked private input: %v", err)
		}
	}
}

func TestTelemetryExecutableBoundsRejectZeroAndHeaderInjection(t *testing.T) {
	for _, edit := range []func(*TelemetryConfig){
		func(c *TelemetryConfig) { c.ExportTimeoutMS = 0 }, func(c *TelemetryConfig) { c.ShutdownTimeoutMS = 5001 }, func(c *TelemetryConfig) {
			c.Headers = map[string]string{"Authorization": "SENTINEL\r\nInjected: yes"}
			c.Enabled = true
			c.Exporter = "otlp_http"
			c.Endpoint = "https://example.invalid"
		},
	} {
		// Arrange: explicit bounds are validated after defaults, not silently repaired.
		c := (TelemetryConfig{}).WithDefaults()
		edit(&c)
		// Act.
		err := c.Validate()
		// Assert.
		if err == nil || strings.Contains(err.Error(), "SENTINEL") {
			t.Fatal(err)
		}
	}
}

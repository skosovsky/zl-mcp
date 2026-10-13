package config

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/skosovsky/zl-mcp/docs/contracts"
)

// TelemetryConfig is additive and private: headers never become telemetry fields.
type TelemetryConfig struct {
	Enabled           bool              `toml:"enabled"`
	Exporter          string            `toml:"exporter"`
	Endpoint          string            `toml:"endpoint"`
	Headers           map[string]string `toml:"headers"`
	ExportTimeoutMS   int               `toml:"export_timeout_ms"`
	ShutdownTimeoutMS int               `toml:"shutdown_timeout_ms"`
}

func (c TelemetryConfig) WithDefaults() TelemetryConfig {
	if c.Exporter == "" {
		c.Exporter = "none"
	}
	if c.Headers == nil {
		c.Headers = map[string]string{}
	}
	if c.ExportTimeoutMS == 0 {
		c.ExportTimeoutMS = 1000
	}
	if c.ShutdownTimeoutMS == 0 {
		c.ShutdownTimeoutMS = 2000
	}
	return c
}

// Validate returns neutral errors, including for malformed private endpoints or
// header contents. SDK/exporter errors must not reveal this configuration.
func (c TelemetryConfig) Validate() error {
	schema, err := contracts.Compile("service_telemetry", "input")
	if err != nil {
		return errors.New("telemetry contract unavailable")
	}
	headers := make(map[string]any, len(c.Headers))
	for k, v := range c.Headers {
		headers[k] = v
	}
	if err = schema.Validate(
		map[string]any{
			"enabled":             c.Enabled,
			"exporter":            c.Exporter,
			"endpoint":            c.Endpoint,
			"headers":             headers,
			"export_timeout_ms":   c.ExportTimeoutMS,
			"shutdown_timeout_ms": c.ShutdownTimeoutMS,
		},
	); err != nil {
		return errors.New("invalid telemetry configuration")
	}
	for _, value := range c.Headers {
		if strings.ContainsAny(value, "\r\n\x00") {
			return errors.New("invalid telemetry configuration")
		}
	}
	if c.Exporter == "none" {
		return nil
	}
	endpoint, err := url.Parse(c.Endpoint)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" ||
		endpoint.Fragment != "" {
		return errors.New("invalid telemetry endpoint")
	}
	if endpoint.Scheme == "https" {
		return nil
	}
	if endpoint.Scheme == "http" {
		ip := net.ParseIP(endpoint.Hostname())
		if ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return errors.New("telemetry endpoint requires HTTPS or a literal loopback HTTP address")
}

package mcpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

const discoveryCaptureLimit = 1 << 20

func discoveryMethod(method string) bool {
	switch method {
	case "initialize", "server/discover", "tools/list", "resources/list", "resources/templates/list", "events/list":
		return true
	}
	return false
}

// Pass through the response and retain only bounded tool definitions for diagnostics.
// Other discovery methods retain no response body.
type discoveryResponse struct {
	http.ResponseWriter
	status   int
	written  int
	capture  bool
	overflow bool
	body     bytes.Buffer
}

func (w *discoveryResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *discoveryResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *discoveryResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(body)
	w.written += n
	if w.capture && !w.overflow {
		if w.body.Len()+n > discoveryCaptureLimit {
			w.overflow = true
			w.body.Reset()
		} else {
			w.body.Write(body[:n])
		}
	}
	return n, err
}

func logDiscovery(method, protocol string, w *discoveryResponse, started time.Time) {
	switch protocol {
	case "2026-07-28", "2025-11-25", "2025-06-18", "2024-11-05":
	case "":
		protocol = "absent"
	default:
		protocol = "other"
	}
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	fields := []any{"method", method, "protocol", protocol, "http_status", status, "duration_ms", time.Since(started).Milliseconds(), "response_bytes", w.written}
	if method == "tools/list" && status == http.StatusOK && !w.overflow {
		var response struct {
			Result *struct {
				Tools      json.RawMessage `json:"tools"`
				NextCursor string          `json:"nextCursor"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal(w.body.Bytes(), &response) == nil && response.Result != nil && len(response.Error) == 0 {
			var tools []struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(response.Result.Tools, &tools) == nil && tools != nil {
				archive := false
				for _, tool := range tools {
					archive = archive || tool.Name == "zalo_list_archive_sources"
				}
				hash := sha256.Sum256(response.Result.Tools)
				fields = append(fields, "tool_count", len(tools), "archive_inventory_present", archive, "has_more", response.Result.NextCursor != "", "catalogue_sha256", hex.EncodeToString(hash[:]))
			}
		}
	}
	slog.Info("mcp_discovery", fields...)
	clear(w.body.Bytes())
}

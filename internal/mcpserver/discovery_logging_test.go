package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestDiscoveryLogsActualCatalogueWithoutRequestSecrets(t *testing.T) {
	// Arrange: a real authenticated HTTP handler and a private request marker.
	dir := t.TempDir()
	store, err := storage.Open(context.Background(), filepath.Join(dir, "messages.sqlite"), nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token := strings.Repeat("private-auth-token", 4)
	handler, err := NewHTTP(store, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	wire := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"private-request-marker":"do-not-log-this"}}}`
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", strings.NewReader(wire))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", "tools/list")
	response := httptest.NewRecorder()
	// Act.
	handler.ServeHTTP(response, request)
	// Assert: the unchanged response and executed logging contract agree.
	var result struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil || len(result.Result.Tools) != 22 {
		t.Fatal("actual catalogue unavailable")
	}
	var log map[string]any
	if json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &log) != nil {
		t.Fatal("discovery log unavailable")
	}
	schema, err := contracts.Compile("mcp_discovery_log", "output")
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(log); err != nil {
		t.Fatal(err)
	}
	if log["tool_count"] != float64(22) || log["archive_inventory_present"] != true || log["has_more"] != false || log["response_bytes"] != float64(response.Body.Len()) {
		t.Fatal("discovery evidence differs from the actual catalogue")
	}
	if strings.Contains(logs.String(), token) || strings.Contains(logs.String(), "do-not-log-this") || strings.Contains(logs.String(), "private-request-marker") {
		t.Fatal("private request was logged")
	}
	// Act / Assert: authentication rejection creates no discovery evidence.
	logs.Reset()
	rejected := request.Clone(context.Background())
	rejected.Body = http.NoBody
	rejected.Header.Set("Authorization", "Bearer rejected-token")
	handler.ServeHTTP(httptest.NewRecorder(), rejected)
	if logs.Len() != 0 {
		t.Fatal("unauthenticated discovery was logged")
	}
}

func TestDiscoveryCaptureBoundsAndErrorsDoNotClaimCatalogue(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", discoveryCaptureLimit+1), `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"private-error"}}`} {
		// Arrange: forwarding must preserve the response, including oversize/error results.
		var logs bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
		recorder := httptest.NewRecorder()
		observed := &discoveryResponse{ResponseWriter: recorder, capture: true}
		// Act.
		observed.WriteHeader(200)
		_, err := observed.Write([]byte(body))
		logDiscovery("tools/list", "private-protocol-marker", observed, time.Now())
		slog.SetDefault(previous)
		// Assert: retain no unbounded body or erroneous catalogue claim.
		if err != nil || recorder.Body.String() != body || observed.body.Len() > discoveryCaptureLimit {
			t.Fatal("response forwarding or bounded capture failed")
		}
		var log map[string]any
		if json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &log) != nil {
			t.Fatal("log unavailable")
		}
		if log["tool_count"] != nil || log["catalogue_sha256"] != nil || log["protocol"] != "other" || strings.Contains(logs.String(), "private-error") || strings.Contains(logs.String(), "private-protocol-marker") {
			t.Fatal("unsafe discovery evidence")
		}
	}
}

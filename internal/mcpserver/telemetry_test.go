package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type wireSample struct {
	tool      string
	transport domain.Transport
	bytes     int64
}
type wireObservation struct {
	executions int
	samples    []wireSample
}

func (*wireObservation) Recording() bool { return true }
func (o *wireObservation) Execute(ctx context.Context, _ string, next shared.Invocation) error {
	o.executions++
	_, err := next(ctx)
	return err
}
func (o *wireObservation) ResponseBytes(_ context.Context, tool string, transport domain.Transport, size int64) {
	o.samples = append(o.samples, wireSample{tool, transport, size})
}
func (*wireObservation) Upstream(ctx context.Context, _ domain.WorkKind, next func(context.Context) error) error {
	return next(ctx)
}
func (*wireObservation) Transition(context.Context, domain.WorkKind, domain.OperationState, domain.OperationState, *time.Duration) {
}
func (*wireObservation) Queue(context.Context, domain.WorkKind, int64)     {}
func (*wireObservation) Remember(context.Context, domain.WorkKind, string) {}
func (*wireObservation) Worker(ctx context.Context, _ domain.WorkKind, _ string, next func(context.Context) error) error {
	return next(ctx)
}

func TestHTTPResponseObservationCountsActualSDKWireBytes(t *testing.T) {
	// Arrange: independent recorder observes the real SDK output with escaping and Unicode.
	ctx := context.Background()
	dir := t.TempDir()
	owner, err := storage.Open(ctx, filepath.Join(dir, "native.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = owner.BindAccount(ctx, "synthetic-account"); err != nil {
		t.Fatal(err)
	}
	if err = owner.Put(ctx, domain.Message{GroupID: "g", ID: "9007199254740993", SenderID: "peer", Text: "界 🍀 <>& newline\nquote\"", SentAt: time.Now().UTC(), Source: "live"}); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("t", 32)
	handler, err := migrationHTTP(t, owner, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	observer := &wireObservation{}
	for _, test := range []struct {
		method     string
		params     map[string]any
		transport  domain.Transport
		executions int
	}{
		{"tools/call", map[string]any{"name": "zalo_get_message_context", "arguments": map[string]any{"group_id": "g", "message_id": "9007199254740993"}}, domain.TransportHTTP, 1},
		{"resources/read", map[string]any{"uri": "zalo://capabilities"}, domain.TransportResource, 0},
	} {
		t.Run(test.method, func(t *testing.T) {
			test.params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
			body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "9007199254740993", "method": test.method, "params": test.params})
			request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", bytes.NewReader(body))
			request = request.WithContext(shared.WithTelemetry(ctx, observer))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set("MCP-Protocol-Version", "2026-07-28")
			request.Header.Set("Mcp-Method", test.method)
			if name, ok := test.params["name"].(string); ok {
				request.Header.Set("Mcp-Name", name)
			} else {
				request.Header.Set("Mcp-Name", test.params["uri"].(string))
			}
			response := httptest.NewRecorder()
			before := observer.executions
			// Act: the counting writer sees exactly the bytes accepted by the underlying wire writer.
			handler.ServeHTTP(response, request)
			// Assert: one observation, no duplicate execution, and unchanged exact string ID.
			var output struct {
				ID    string `json:"id"`
				Error any    `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &output); err != nil || response.Code != 200 || output.Error != nil || output.ID != "9007199254740993" {
				t.Fatalf("unexpected SDK reply: %s", response.Body.String())
			}
			sample := observer.samples[len(observer.samples)-1]
			if sample.bytes != int64(response.Body.Len()) || sample.transport != test.transport || observer.executions-before != test.executions {
				t.Fatal("wire byte measurement/execution count changed")
			}
			if test.method == "tools/call" && sample.tool != "zalo_get_message_context" {
				t.Fatal("tool response name lost")
			}
		})
	}
	if len(observer.samples) != 2 {
		t.Fatal("response observations duplicated")
	}
}

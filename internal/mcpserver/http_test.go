package mcpserver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type localReceiverTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (tr localReceiverTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	u := *r.URL
	u.Scheme, u.Host = tr.target.Scheme, tr.target.Host
	copy.URL = &u
	return tr.base.RoundTrip(copy)
}

func httpRPC(t *testing.T, endpoint, token, method string, params map[string]any) map[string]any {
	t.Helper()
	params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	r, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2026-07-28")
	r.Header.Set("Mcp-Method", method)
	if name, ok := params["name"].(string); ok {
		r.Header.Set("Mcp-Name", name)
	}
	if uri, ok := params["uri"].(string); ok {
		r.Header.Set("Mcp-Name", uri)
	}
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	wire, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if response.StatusCode != 200 || json.Unmarshal(wire, &result) != nil || result["error"] != nil {
		t.Fatalf("%s status=%d response=%s", method, response.StatusCode, wire)
	}
	return result["result"].(map[string]any)
}

func TestHTTPEventsLifecycleWithSignedTLSReceiverAndFullText(t *testing.T) {
	// Arrange: callback requests traverse a real local TLS socket; only the test
	// transport rewrites the public callback authority to the synthetic receiver.
	ctx := context.Background()
	dir := t.TempDir()
	store, err := storage.Open(ctx, filepath.Join(dir, "messages.sqlite"), []string{"group", "other"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := []byte("synthetic-key-for-webhook-32bytes!")
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	received := make(chan events.MessageEvent, 4)
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			http.Error(w, "bad body", 400)
			return
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(r.Header.Get("webhook-id") + "." + r.Header.Get("webhook-timestamp") + "."))
		mac.Write(body)
		sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("webhook-signature"), "v1,"))
		if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
			t.Error("invalid signature")
			http.Error(w, "invalid", 400)
			return
		}
		var challenge map[string]any
		if err := json.Unmarshal(body, &challenge); err != nil {
			t.Error(err)
			return
		}
		if challenge["type"] == "verification" {
			json.NewEncoder(w).Encode(map[string]any{"challenge": challenge["challenge"]})
			return
		}
		var event events.MessageEvent
		if err := json.Unmarshal(body, &event); err != nil {
			t.Error(err)
			return
		}
		received <- event
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	target, _ := url.Parse(receiver.URL)
	callbackClient := &http.Client{Transport: localReceiverTransport{target: target, base: receiver.Client().Transport}}
	token := strings.Repeat("t", 32)
	service, err := NewHTTP(store, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	service.Events.Client = callbackClient
	endpoint := httptest.NewServer(service)
	defer endpoint.Close()
	worker, err := events.NewWorker(store)
	if err != nil {
		t.Fatal(err)
	}
	worker.Client = callbackClient
	if err := store.Put(ctx, domain.Message{GroupID: "group", ID: "old", SenderID: "author", SentAt: time.Now(), Text: "prior corpus", Source: "live"}); err != nil {
		t.Fatal(err)
	}
	// Act: discover and subscribe through HTTP, not Go methods.
	discovery := httpRPC(t, endpoint.URL+"/mcp", token, "server/discover", map[string]any{})
	catalog := httpRPC(t, endpoint.URL+"/mcp", token, "events/list", map[string]any{})
	params := map[string]any{"name": events.MessageCreated, "arguments": map[string]any{"group_id": "group"}, "delivery": map[string]any{"mode": "webhook", "url": "https://receiver.example/events", "secret": secret}, "ttlMs": nil}
	sub := httpRPC(t, endpoint.URL+"/mcp", token, "events/subscribe", params)
	fullText := strings.Repeat("Ремонт 界 ", 600)
	for _, group := range []string{"group", "other"} {
		if err := store.Put(ctx, domain.Message{GroupID: group, ID: "new", SenderID: "author", SentAt: time.Now().Add(-time.Hour), Text: fullText, Source: "replay"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.FanoutEvents(ctx, time.Now(), worker.Policy, worker.Encoder.Encode); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.DeliverOne(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Assert: only the newly inserted record in the subscribed group arrived.
	if _, ok := discovery["capabilities"].(map[string]any)["events"]; !ok || len(catalog["events"].([]any)) != 3 || sub["refreshBefore"] != nil {
		t.Fatal("discovery or subscription contract mismatch")
	}
	var event events.MessageEvent
	select {
	case event = <-received:
	default:
		t.Fatal("no actual callback received")
	}
	if event.Data.MessageID != "new" || event.Data.GroupID != "group" || !event.Data.TextTruncated || event.Data.TextResourceURI == nil {
		t.Fatal("wrong event or missing full text reference")
	}
	// Act.
	resource := httpRPC(t, endpoint.URL+"/mcp", token, "resources/read", map[string]any{"uri": *event.Data.TextResourceURI})
	// Assert.
	if resource["contents"].([]any)[0].(map[string]any)["text"] != fullText {
		t.Fatal("full resource text differs from source")
	}
	// Act: cancel, insert another message and verify that delivery stops.
	params["delivery"] = map[string]any{"mode": "webhook", "url": "https://receiver.example/events"}
	delete(params, "ttlMs")
	httpRPC(t, endpoint.URL+"/mcp", token, "events/unsubscribe", params)
	if err := store.Put(ctx, domain.Message{GroupID: "group", ID: "after-cancel", SenderID: "author", SentAt: time.Now(), Text: "later", Source: "live"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FanoutEvents(ctx, time.Now(), worker.Policy, worker.Encoder.Encode); err != nil {
		t.Fatal(err)
	}
	handled, err := worker.DeliverOne(ctx, time.Now())
	// Assert.
	if err != nil || handled || len(received) != 0 {
		t.Fatal("cancelled or unrelated group delivered")
	}
}

func TestHTTPRejectsUnauthorizedOriginsAndProtocolMismatch(t *testing.T) {
	// Arrange.
	dir := t.TempDir()
	store, err := storage.Open(context.Background(), filepath.Join(dir, "messages.sqlite"), []string{"group"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token := strings.Repeat("t", 32)
	service, err := NewHTTP(store, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"events/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`
	for _, tc := range []struct {
		name, auth, origin, host, method, version string
		status                                    int
		code                                      float64
	}{
		{"missing-auth", "", "", "localhost", "events/list", "2026-07-28", 401, 0},
		{"wrong-token", "Bearer wrong", "", "localhost", "events/list", "2026-07-28", 401, 0},
		{"browser-origin", "Bearer " + token, "https://evil.example", "localhost", "events/list", "2026-07-28", 403, 0},
		{"rebound-host", "Bearer " + token, "", "evil.example", "events/list", "2026-07-28", 403, 0},
		{"mismatched-method", "Bearer " + token, "", "localhost", "events/subscribe", "2026-07-28", 400, float64(mcp.CodeHeaderMismatch)},
		{"mismatched-version", "Bearer " + token, "", "localhost", "events/list", "2025-03-26", 400, float64(mcp.CodeHeaderMismatch)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://localhost/mcp", strings.NewReader(body))
			r.Host = tc.host
			r.Header.Set("Authorization", tc.auth)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept", "application/json, text/event-stream")
			r.Header.Set("Mcp-Method", tc.method)
			r.Header.Set("MCP-Protocol-Version", tc.version)
			response := httptest.NewRecorder()
			// Act.
			service.ServeHTTP(response, r)
			// Assert.
			if response.Code != tc.status {
				t.Fatalf("status=%d", response.Code)
			}
			if tc.code != 0 {
				var result map[string]any
				if json.Unmarshal(response.Body.Bytes(), &result) != nil || result["error"].(map[string]any)["code"] != tc.code {
					t.Fatalf("unexpected protocol error: %s", response.Body.String())
				}
			}
		})
	}
}

func TestEventsHTTPTransportGuards(t *testing.T) {
	// Arrange: valid Events requests except for the transport field under test.
	dir := t.TempDir()
	store, err := storage.Open(context.Background(), filepath.Join(dir, "messages.sqlite"), []string{"group"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token := strings.Repeat("t", 32)
	service, err := NewHTTP(store, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, content, accept, header, meta, method string
		status                                      int
		code                                        int
	}{
		{"missing-content", "", "*/*", "2026-07-28", "2026-07-28", "events/list", 415, 0},
		{"wrong-content", "text/plain", "*/*", "2026-07-28", "2026-07-28", "events/list", 415, 0},
		{"missing-accept", "application/json", "", "2026-07-28", "2026-07-28", "events/list", 400, 0},
		{"json-only", "application/json", "application/json", "2026-07-28", "2026-07-28", "events/list", 400, 0},
		{"missing-version-header", "application/json", "*/*", "", "2026-07-28", "events/list", 400, mcp.CodeHeaderMismatch},
		{"missing-version-meta", "application/json", "*/*", "2026-07-28", "", "events/list", 400, -32602},
		{"unsupported-version", "application/json", "*/*", "2099-01-01", "2099-01-01", "events/list", 400, mcp.CodeUnsupportedProtocolVersion},
		{"unknown-method", "application/json", "*/*", "2026-07-28", "2026-07-28", "events/unknown", 404, -32601},
		{"parameterized-content", "application/json; charset=utf-8", "application/*, text/*", "2026-07-28", "2026-07-28", "events/list", 200, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": tc.method, "params": map[string]any{"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": tc.meta, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}}})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("POST", "http://localhost/mcp", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", tc.content)
			req.Header.Set("Accept", tc.accept)
			req.Header.Set("MCP-Protocol-Version", tc.header)
			req.Header.Set("Mcp-Method", tc.method)
			response := httptest.NewRecorder()
			// Act.
			service.ServeHTTP(response, req)
			// Assert.
			if response.Code != tc.status {
				t.Fatalf("HTTP %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
			if tc.code != 0 {
				var wire struct {
					Error struct {
						Code int            `json:"code"`
						Data map[string]any `json:"data"`
					} `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &wire); err != nil {
					t.Fatal(err)
				}
				if wire.Error.Code != tc.code {
					t.Fatalf("RPC %d, want %d", wire.Error.Code, tc.code)
				}
				if tc.code == mcp.CodeUnsupportedProtocolVersion && (wire.Error.Data["requested"] != tc.meta || wire.Error.Data["supported"] == nil) {
					t.Fatal("missing supported/requested protocol data")
				}
			}
		})
	}
}

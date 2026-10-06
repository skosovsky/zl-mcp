package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
)

type conversationServiceSource struct {
	authListener
	input chan domain.Message
}

func (s *conversationServiceSource) ListenConversations(ctx context.Context, message func(domain.Message) error, _ func(domain.ConversationRef, string) error, connected func() error) error {
	if err := connected(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m := <-s.input:
			if err := message(m); err != nil {
				return err
			}
		}
	}
}

type conversationServiceTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (tr conversationServiceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	u := *r.URL
	u.Scheme, u.Host = tr.target.Scheme, tr.target.Host
	clone.URL = &u
	return tr.base.RoundTrip(clone)
}
func serviceConversationRPC(t *testing.T, endpoint, token, method string, params map[string]any) map[string]any {
	t.Helper()
	params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", method)
	if name, ok := params["name"].(string); ok {
		req.Header.Set("Mcp-Name", name)
	}
	if uri, ok := params["uri"].(string); ok {
		req.Header.Set("Mcp-Name", uri)
	}
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var value map[string]any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || value["error"] != nil {
		t.Fatalf("RPC failed: %s", method)
	}
	result := value["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("tool failed: %s", params["name"])
	}
	return result
}

func TestUnifiedConversationServiceCollectionMCPEventsRestartAndCancel(t *testing.T) {
	for _, profile := range []struct {
		name    string
		version int
	}{
		{domain.ConversationMessageCreatedV2, 2},
	} {
		t.Run(profile.name, func(t *testing.T) { testConversationServiceProfile(t, profile.name, profile.version) })
	}
}

func testConversationServiceProfile(t *testing.T, profile string, version int) {
	// Arrange: real service/storage/HTTP/worker, a synthetic Zalo source and an
	// independently verifying TLS receiver. No account or installed state is used.
	dir, err := os.MkdirTemp("/tmp", "zl-conversation-service-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	var c config.Config
	c.StateDir = dir
	c.Collection.Mode = "all"
	c.Storage.RetentionDays = 90
	c.MCP.Listen = "127.0.0.1:0"
	c.MCP.TokenFile = filepath.Join(dir, "token")
	token := strings.Repeat("a", 64)
	if err := os.WriteFile(c.MCP.TokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("k", 32))
	received := make(chan map[string]any, 8)
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(r.Header.Get("webhook-id") + "." + r.Header.Get("webhook-timestamp") + "."))
		mac.Write(body)
		sig, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("webhook-signature"), "v1,"))
		if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
			t.Error("signature mismatch")
			w.WriteHeader(400)
			return
		}
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if value["type"] == "verification" {
			json.NewEncoder(w).Encode(map[string]any{"challenge": value["challenge"]})
			return
		}
		received <- value
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	target, _ := url.Parse(receiver.URL)
	callback := &http.Client{Transport: conversationServiceTransport{target: target, base: receiver.Client().Transport}}
	start := func() (string, *conversationServiceSource, func()) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		bound := make(chan net.Addr, 1)
		source := &conversationServiceSource{input: make(chan domain.Message, 8)}
		go func() {
			done <- runConfigured(ctx, c, func(context.Context, string) (collector.ListenerUpstream, error) { return source, nil }, func(collector.ListenerUpstream) error { return nil }, func(addr net.Addr) { bound <- addr }, func(server *mcpserver.HTTPService, worker *events.Worker) {
				server.Events.Client = callback
				worker.Client = callback
			})
		}()
		stop := func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(5 * time.Second):
				t.Error("service did not stop")
			}
		}
		var endpoint string
		select {
		case address := <-bound:
			endpoint = "http://" + address.String() + "/mcp"
		case err := <-done:
			cancel()
			t.Fatalf("startup failed: %v", err)
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("startup timed out")
		}
		return endpoint, source, stop
	}
	endpoint, source, stop := start()
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	args := map[string]any{"scope": "all"}
	if version == 2 {
		args["direction"] = "incoming"
	}
	delivery := map[string]any{"mode": "webhook", "url": "https://receiver.example/events", "secret": "whsec_" + base64.StdEncoding.EncodeToString(key)}
	// Act: activate through MCP, then ingest both types through the collector.
	sub := serviceConversationRPC(t, endpoint, token, "events/subscribe", map[string]any{"name": profile, "arguments": args, "delivery": delivery})
	text := "searchable " + strings.Repeat("界", 2049)
	at := time.Now().UTC().Add(-time.Hour)
	emit := func(id, sourceName string) {
		for _, kind := range []string{"direct", "group"} {
			source.input <- domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: "same"}, ID: id, SenderID: "owner", SentAt: at, Text: text, Source: sourceName, Direction: "incoming"}
		}
		if version == 2 {
			source.input <- domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "same"}, ID: "own-" + id, SenderID: "owner", SentAt: at, Text: "outgoing filtered", Source: sourceName, Direction: "outgoing"}
		}
	}
	emit("first", "live")
	readEvents := func(firstIncoming bool) {
		t.Helper()
		seen := map[string]bool{}
		for range 2 {
			select {
			case event := <-received:
				data := event["data"].(map[string]any)
				kind := data["conversation_type"].(string)
				if seen[kind] || event["name"] != profile || data["text_truncated"] != true {
					t.Fatal("invalid typed callback")
				}
				if data["schema_version"] != float64(version) {
					t.Fatal("incorrect schema version")
				}
				if version == 2 {
					if data["direction"] != "incoming" {
						t.Fatal("outgoing passed incoming filter")
					}
					if kind == "direct" && data["first_incoming"] != firstIncoming {
						t.Fatal("first incoming fact lost across restart")
					}
				}
				seen[kind] = true
				full := serviceConversationRPC(t, endpoint, token, "resources/read", map[string]any{"uri": data["text_resource_uri"]})
				if full["contents"].([]any)[0].(map[string]any)["text"] != text {
					t.Fatal("full text missing")
				}
				contextResult := serviceConversationRPC(t, endpoint, token, "tools/call", map[string]any{"name": "zalo_get_conversation_message_context", "arguments": map[string]any{"conversation_type": kind, "conversation_id": "same", "message_id": data["message_id"]}})["structuredContent"].(map[string]any)
				if contextResult["anchor"].(map[string]any)["sender_id"] != "owner" {
					t.Fatal("collector lost author")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("callback was not delivered by service worker")
			}
		}
		if !seen["direct"] || !seen["group"] {
			t.Fatal("missing conversation type")
		}
	}
	// Assert: source -> collector -> storage -> worker -> TLS -> MCP full reads.
	readEvents(true)
	hits := serviceConversationRPC(t, endpoint, token, "tools/call", map[string]any{"name": "zalo_search_conversation_messages", "arguments": map[string]any{"query": "searchable"}})["structuredContent"].(map[string]any)["messages"].([]any)
	if len(hits) != 2 || sub["refreshBefore"] != nil {
		t.Fatal("search or indefinite subscription failed")
	}
	// Act: restart the whole service on the same account/state; replay originals
	// and a first insertion with an old sent_at without refreshing the subscription.
	stop()
	stop = nil
	endpoint, source, stop = start()
	emit("first", "replay")
	emit("second", "replay")
	// Assert: durable subscription delivers only new typed identities after restart.
	readEvents(false)
	hits = serviceConversationRPC(t, endpoint, token, "tools/call", map[string]any{"name": "zalo_search_conversation_messages", "arguments": map[string]any{"query": "searchable"}})["structuredContent"].(map[string]any)["messages"].([]any)
	if len(hits) != 4 {
		t.Fatal("restart lost messages or generated duplicates")
	}
	// Act: cancel over MCP, then continue collecting both types.
	serviceConversationRPC(t, endpoint, token, "events/unsubscribe", map[string]any{"name": profile, "arguments": args, "delivery": map[string]any{"mode": "webhook", "url": "https://receiver.example/events"}})
	emit("after-cancel", "live")
	deadline := time.Now().Add(5 * time.Second)
	for {
		hits = serviceConversationRPC(t, endpoint, token, "tools/call", map[string]any{"name": "zalo_search_conversation_messages", "arguments": map[string]any{"query": "searchable"}})["structuredContent"].(map[string]any)["messages"].([]any)
		if len(hits) == 6 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancellation stopped collection")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Assert: cancellation stops callbacks, not the collector. Allow a complete
	// worker polling interval so an erroneous fanout would reach the receiver.
	select {
	case <-received:
		t.Fatal("cancelled callback delivered")
	case <-time.After(600 * time.Millisecond):
	}
}

package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
)

type messageCommand struct {
	message domain.Message
	done    chan error
}
type replayListener struct {
	authListener
	messages chan messageCommand
}

func (a *replayListener) Listen(ctx context.Context, message func(domain.Message) error, _ func(string, string) error, connected func() error) error {
	a.calls.Add(1)
	if err := connected(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case command := <-a.messages:
			err := message(command.message)
			command.done <- err
			if err != nil {
				return err
			}
		}
	}
}

func (*replayListener) Groups(context.Context) ([]domain.Group, error) {
	return []domain.Group{{ID: "g", Name: "Test group"}}, nil
}
func (*replayListener) Group(context.Context, string) (domain.Group, *string, error) {
	return domain.Group{ID: "g", Name: "Test group"}, nil, nil
}

type receiverRoute struct {
	target    *url.URL
	transport http.RoundTripper
}

func (r receiverRoute) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	u := *request.URL
	u.Scheme, u.Host = r.target.Scheme, r.target.Host
	copy.URL = &u
	return r.transport.RoundTrip(copy)
}
func serviceRPC(t *testing.T, endpoint, token, method string, params map[string]any) map[string]any {
	t.Helper()
	params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", method)
	if name, ok := params["name"].(string); ok {
		request.Header.Set("Mcp-Name", name)
	}
	if uri, ok := params["uri"].(string); ok {
		request.Header.Set("Mcp-Name", uri)
	}
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var reply map[string]any
	if err := json.NewDecoder(response.Body).Decode(&reply); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || reply["error"] != nil {
		t.Fatalf("%s failed: %d %v", method, response.StatusCode, reply)
	}
	return reply["result"].(map[string]any)
}

func TestServiceEventsSurviveRestartWithStableDeliveryAndBoundary(t *testing.T) {
	// Arrange: real service lifecycle, SQLite, MCP HTTP and signed TLS callbacks.
	// Only the Zalo source and callback authority routing are synthetic.
	dir, err := os.MkdirTemp("/tmp", "zl-events-service-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	var c config.Config
	c.StateDir = dir
	c.Collection.GroupIDs = []string{"g", "other"}
	c.Storage.RetentionDays = 90
	c.MCP.Listen = "127.0.0.1:0"
	c.MCP.TokenFile = filepath.Join(dir, "token")
	token := strings.Repeat("t", 64)
	if err := os.WriteFile(c.MCP.TokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	key := []byte("independent-synthetic-signing-key!")
	secret := "whsec_" + base64.StdEncoding.EncodeToString(key)
	type receipt struct {
		event events.MessageEvent
		body  []byte
	}
	receipts := make(chan receipt, 8)
	var rejectFirst atomic.Bool
	rejectFirst.Store(true)
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			http.Error(w, "body", 400)
			return
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(r.Header.Get("webhook-id") + "." + r.Header.Get("webhook-timestamp") + "."))
		mac.Write(body)
		signature, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("webhook-signature"), "v1,"))
		if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
			t.Error("invalid independent HMAC verification")
			http.Error(w, "signature", 400)
			return
		}
		var verification map[string]any
		if err := json.Unmarshal(body, &verification); err != nil {
			t.Error(err)
			return
		}
		if verification["type"] == "verification" {
			json.NewEncoder(w).Encode(map[string]any{"challenge": verification["challenge"]})
			return
		}
		var event events.MessageEvent
		if err := json.Unmarshal(body, &event); err != nil {
			t.Error(err)
			return
		}
		if event.EventID != r.Header.Get("webhook-id") {
			t.Error("event ID differs from signed ID")
		}
		if rejectFirst.Swap(false) {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(503)
		} else {
			w.WriteHeader(204)
		}
		receipts <- receipt{event: event, body: body}
	}))
	t.Cleanup(receiver.Close)
	target, _ := url.Parse(receiver.URL)
	callback := &http.Client{Transport: receiverRoute{target: target, transport: receiver.Client().Transport}, Timeout: 3 * time.Second}
	var runningCancel context.CancelFunc
	var runningDone chan error
	t.Cleanup(func() {
		if runningCancel != nil {
			runningCancel()
			select {
			case <-runningDone:
			case <-time.After(5 * time.Second):
				t.Error("cleanup timed out")
			}
		}
	})
	start := func() (string, *replayListener) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		runningCancel = cancel
		runningDone = make(chan error, 1)
		client := &replayListener{messages: make(chan messageCommand)}
		bound := make(chan net.Addr, 1)
		go func() {
			runningDone <- runConfigured(ctx, c, func(context.Context, string) (collector.ListenerUpstream, error) { return client, nil }, func(collector.ListenerUpstream) error { return nil }, func(addr net.Addr) { bound <- addr }, func(httpService *mcpserver.HTTPService, worker *events.Worker) {
				httpService.Events.Client = callback
				worker.Client = callback
			})
		}()
		select {
		case addr := <-bound:
			return "http://" + addr.String() + "/mcp", client
		case err := <-runningDone:
			t.Fatalf("startup: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("startup timed out")
		}
		return "", nil
	}
	stop := func() {
		t.Helper()
		runningCancel()
		select {
		case err := <-runningDone:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("shutdown timed out")
		}
		runningCancel = nil
	}
	send := func(client *replayListener, id, group, text string) {
		t.Helper()
		ack := make(chan error, 1)
		command := messageCommand{message: domain.Message{ID: id, GroupID: group, SenderID: "author", SentAt: time.Now().Add(-time.Hour), Text: text, Source: "replay"}, done: ack}
		select {
		case client.messages <- command:
		case <-time.After(3 * time.Second):
			t.Fatal("listener not ready")
		}
		select {
		case err := <-ack:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("message not persisted")
		}
	}
	take := func() receipt {
		t.Helper()
		select {
		case r := <-receipts:
			return r
		case <-time.After(5 * time.Second):
			t.Fatal("callback absent")
		}
		return receipt{}
	}
	endpoint, client := start()
	send(client, "old", "g", "old corpus")
	discovery := serviceRPC(t, endpoint, token, "server/discover", map[string]any{})
	if _, ok := discovery["capabilities"].(map[string]any)["events"]; !ok {
		t.Fatal("Events capability absent")
	}
	params := func() map[string]any {
		return map[string]any{"name": events.MessageCreated, "arguments": map[string]any{"group_id": "g"}, "delivery": map[string]any{"mode": "webhook", "url": "https://receiver.example/events", "secret": secret}, "ttlMs": nil}
	}
	sub := serviceRPC(t, endpoint, token, "events/subscribe", params())
	fullText := "restart " + strings.Repeat("界я", 1500)
	// Act: persist a new message, reject its first callback and stop the whole service.
	send(client, "first", "g", fullText)
	first := take()
	db, err := sql.Open("sqlite", filepath.Join(dir, "messages.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	wait := func(predicate func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !predicate() {
			if time.Now().After(deadline) {
				t.Fatal("durable state did not settle")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait(func() bool {
		var state string
		return db.QueryRow("SELECT state FROM event_deliveries WHERE event_id=?", first.event.EventID).Scan(&state) == nil && state == "pending"
	})
	var boundary int64
	if err := db.QueryRow("SELECT start_seq FROM event_subscriptions WHERE id=?", sub["id"]).Scan(&boundary); err != nil {
		t.Fatal(err)
	}
	stop()
	endpoint, client = start()
	refreshed := serviceRPC(t, endpoint, token, "events/subscribe", params())
	send(client, "old", "g", "old corpus")
	send(client, "first", "g", fullText)
	send(client, "late", "g", "late available replay")
	send(client, "outside", "other", "unsubscribed group")
	seen := map[string]receipt{}
	for range 2 {
		r := take()
		seen[r.event.Data.MessageID] = r
	}
	// Assert: restart and refresh preserve the original boundary and retry bytes.
	var after int64
	if err := db.QueryRow("SELECT start_seq FROM event_subscriptions WHERE id=?", sub["id"]).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if refreshed["id"] != sub["id"] || refreshed["refreshBefore"] != nil || boundary != after || !bytes.Equal(first.body, seen["first"].body) || first.event.EventID != seen["first"].event.EventID || seen["late"].event.Data.MessageID != "late" {
		t.Fatal("restart lost boundary, stable payload, or late replay")
	}
	if !first.event.Data.TextTruncated || len([]rune(first.event.Data.Text)) != 2048 || first.event.Data.TextResourceURI == nil {
		t.Fatal("invalid bounded text contract")
	}
	resource := serviceRPC(t, endpoint, token, "resources/read", map[string]any{"uri": *first.event.Data.TextResourceURI})
	if resource["contents"].([]any)[0].(map[string]any)["text"] != fullText {
		t.Fatal("resource lost full text across restart")
	}
	// Act: cancel via MCP, persist another record and wait until fanout consumes it.
	serviceRPC(t, endpoint, token, "events/unsubscribe", map[string]any{"name": events.MessageCreated, "arguments": map[string]any{"group_id": "g"}, "delivery": map[string]any{"mode": "webhook", "url": "https://receiver.example/events"}})
	send(client, "cancelled", "g", "after cancellation")
	wait(func() bool {
		var remaining int
		return db.QueryRow("SELECT count(*) FROM message_events").Scan(&remaining) == nil && remaining == 0
	})
	// Assert: cancelled, historical, duplicate and unrelated records created no jobs.
	var jobs int
	if err := db.QueryRow("SELECT count(*) FROM event_deliveries").Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 2 || len(receipts) != 0 {
		t.Fatalf("unexpected deliveries: jobs=%d callbacks=%d", jobs, len(receipts))
	}
	stop()
}

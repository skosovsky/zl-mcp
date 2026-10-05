package events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func readyWorker(t *testing.T, transport http.RoundTripper) (*Worker, *storage.Store, time.Time) {
	t.Helper()
	ctx := context.Background()
	s, err := storage.Open(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), []string{"group"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	at := time.Now().UTC()
	revision, err := s.SubscriptionRevision(ctx, "subscription")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ActivateSubscription(ctx, storage.EventSubscription{ID: "subscription", Principal: "owner", GroupID: "group", Callback: "https://callback.example/events", Secret: "whsec_" + base64.StdEncoding.EncodeToString([]byte("synthetic-key-for-webhook-32bytes!"))}, revision, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, domain.Message{GroupID: "group", ID: "message", SenderID: "author", Text: strings.Repeat("界", 2050), SentAt: at, Source: "live"}); err != nil {
		t.Fatal(err)
	}
	w, err := NewWorker(s)
	if err != nil {
		t.Fatal(err)
	}
	w.Client = &http.Client{Transport: transport}
	if _, err := s.FanoutEvents(ctx, at, w.Policy, w.Encoder.Encode); err != nil {
		t.Fatal(err)
	}
	return w, s, at
}

func TestSignedDeliveryRetryAfterAndStablePayload(t *testing.T) {
	// Arrange: independently check signature and envelope at the receiving side.
	var bodies [][]byte
	var ids []string
	transport := callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var event MessageEvent
		if err := json.Unmarshal(body, &event); err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, []byte("synthetic-key-for-webhook-32bytes!"))
		mac.Write([]byte(r.Header.Get("webhook-id") + "." + r.Header.Get("webhook-timestamp") + "."))
		mac.Write(body)
		signature, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("webhook-signature"), "v1,"))
		if err != nil || !hmac.Equal(signature, mac.Sum(nil)) || r.Header.Get("webhook-id") != event.EventID || r.Header.Get("X-MCP-Subscription-Id") != "subscription" || !event.Data.TextTruncated || event.Data.TextResourceURI == nil {
			t.Fatal("invalid signed delivery")
		}
		bodies = append(bodies, body)
		ids = append(ids, event.EventID)
		code := 200
		if len(bodies) == 1 {
			code = 503
		}
		return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"60"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	w, s, at := readyWorker(t, transport)
	// Act.
	for _, offset := range []time.Duration{0, 30 * time.Second, 61 * time.Second} {
		if _, err := w.DeliverOne(context.Background(), at.Add(offset)); err != nil {
			t.Fatal(err)
		}
	}
	// Assert: no early retry, and exact payload is reused after transient failure.
	if len(bodies) != 2 || ids[0] != ids[1] || string(bodies[0]) != string(bodies[1]) {
		t.Fatal("retry identity, timing or body changed")
	}
	var state string
	var attempts int
	if err := s.DB.QueryRow("SELECT state,attempts FROM event_deliveries").Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	if state != "delivered" || attempts != 2 {
		t.Fatal("receipt was not persisted")
	}
}

func TestDeliveryTerminalHTTPResponses(t *testing.T) {
	for _, code := range []int{400, 410, 413} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			// Arrange.
			calls := 0
			w, s, at := readyWorker(t, callbackRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
			}))
			// Act.
			if _, err := w.DeliverOne(context.Background(), at); err != nil {
				t.Fatal(err)
			}
			if _, err := w.DeliverOne(context.Background(), at.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			// Assert.
			var active int
			if err := s.DB.QueryRow("SELECT active FROM event_subscriptions").Scan(&active); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || (active == 0) != (code == 410) {
				t.Fatalf("calls=%d active=%d code=%d", calls, active, code)
			}
		})
	}
}

func TestRetryPreservesSubscriptionOrder(t *testing.T) {
	// Arrange: the first message fails once; later messages must wait for its retry.
	ctx := context.Background()
	var received []string
	w, s, at := readyWorker(t, callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		var event MessageEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatal(err)
		}
		received = append(received, event.EventID)
		code := http.StatusOK
		if len(received) == 1 {
			code = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: code, Header: http.Header{"Retry-After": []string{"60"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	if err := s.Put(ctx, domain.Message{GroupID: "group", ID: "later", SenderID: "author", Text: "second synthetic message", SentAt: at.Add(time.Second), Source: "live"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FanoutEvents(ctx, at, w.Policy, w.Encoder.Encode); err != nil {
		t.Fatal(err)
	}
	// Act: the delayed retry blocks only later jobs belonging to this subscription.
	for _, offset := range []time.Duration{0, time.Second, 61 * time.Second, 62 * time.Second} {
		delivered, err := w.DeliverOne(ctx, at.Add(offset))
		if err != nil || delivered != (offset != time.Second) {
			t.Fatalf("delivery at %v: worked=%v error=%v", offset, delivered, err)
		}
	}
	// Assert: the retry retains its ID and precedes the second message.
	if len(received) != 3 || received[0] != received[1] || received[0] == received[2] {
		t.Fatalf("unexpected receipt order: %v", received)
	}
	var completed int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM event_deliveries WHERE state='delivered'").Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != 2 {
		t.Fatalf("completed deliveries=%d, want 2", completed)
	}
}

func TestDeliveryTraceCorrelatesBodyWithoutPrivateContent(t *testing.T) {
	// Arrange: the receiver returns a useful trace ID and an unsafe header.
	var logs bytes.Buffer
	var sent []byte
	w, _, at := readyWorker(t, callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		sent, _ = io.ReadAll(r.Body)
		return &http.Response{StatusCode: 202, Header: http.Header{"X-Request-Id": []string{"req_synthetic-123"}, "Openai-Request-Id": []string{"private value must not leak"}}, Body: io.NopCloser(strings.NewReader("private response body"))}, nil
	}))
	w.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	// Act.
	handled, err := w.DeliverOne(context.Background(), at)
	// Assert: persisted receipt can be correlated with the exact outbound body.
	if err != nil || !handled {
		t.Fatalf("delivery failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected start and finish: %d", len(lines))
	}
	digest := sha256.Sum256(sent)
	for i, line := range lines {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatal(err)
		}
		if v["body_sha256"] != fmt.Sprintf("%x", digest) || v["body_bytes"] != float64(len(sent)) || v["event_id"] == "" || v["subscription_id"] != "subscription" {
			t.Fatal("missing correlation metadata")
		}
		if i == 1 && (v["http_status"] != float64(202) || v["request_id"] != "req_synthetic-123" || v["openai_request_id"] != "" || v["receipt_persisted"] != true || v["outcome"] != "delivered") {
			t.Fatal("missing receipt metadata")
		}
	}
	for _, private := range []string{"界", "callback.example", "whsec_", "synthetic-key", "private response", "private value"} {
		if strings.Contains(logs.String(), private) {
			t.Fatalf("private content logged: %q", private)
		}
	}
}

func TestFullTraceRetainsEventAndBoundsRedactedReceiverBody(t *testing.T) {
	// Arrange.
	var trace bytes.Buffer
	w, _, at := readyWorker(t, callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("whsec_" + base64.StdEncoding.EncodeToString([]byte("synthetic-key-for-webhook-32bytes!")) + " https://callback.example/events " + strings.Repeat("x", 70000)))}, nil
	}))
	w.Logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	w.TraceLogger = slog.New(slog.NewJSONHandler(&trace, nil))
	// Act.
	_, err := w.DeliverOne(context.Background(), at)
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(trace.String()), "\n")
	if len(lines) != 3 {
		t.Fatal("missing full request/response/receipt trace")
	}
	var request, response map[string]any
	if json.Unmarshal([]byte(lines[0]), &request) != nil || json.Unmarshal([]byte(lines[1]), &response) != nil {
		t.Fatal("invalid trace JSON")
	}
	body := request["body"].(map[string]any)
	if body["data"].(map[string]any)["text"] != strings.Repeat("界", 2048) || body["eventId"] != request["event_id"] {
		t.Fatal("request trace lost exact event data")
	}
	if response["body_truncated"] != true || response["body_read_failed"] != false || response["http_status"] != float64(200) {
		t.Fatal("missing bounded response evidence")
	}
	for _, private := range []string{"whsec_", "callback.example", "synthetic-key-for-webhook"} {
		if strings.Contains(trace.String(), private) {
			t.Fatalf("credential material logged: %s", private)
		}
	}
}

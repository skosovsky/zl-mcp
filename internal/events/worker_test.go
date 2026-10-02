package events

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
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

func TestRetryDoesNotBlockLaterMessages(t *testing.T) {
	// Arrange: the first message fails once; the receiver accepts the next one.
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
	// Act: deliver the next ready job while the earlier job waits for its retry.
	for _, offset := range []time.Duration{0, time.Second, 61 * time.Second} {
		delivered, err := w.DeliverOne(ctx, at.Add(offset))
		if err != nil || !delivered {
			t.Fatalf("delivery at %v: worked=%v error=%v", offset, delivered, err)
		}
	}
	// Assert: out-of-order receipt is supported, and the retried event keeps its ID.
	if len(received) != 3 || received[0] == received[1] || received[0] != received[2] {
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

package events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

//nolint:cyclop,gocognit,gocyclo // One AAA scenario follows business readiness, retry, reminder and ack suppression in order.
func TestRecoveryWorkerSignedHTTPRetryReminderAndAckSuppression(t *testing.T) {
	// Arrange: actual HTTP receiver checks business/control HMAC independently.
	ctx := context.Background()
	var mu sync.Mutex
	var bodies [][]byte
	var ids []string
	key := []byte("synthetic-key-for-webhook-32bytes!")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var envelope map[string]any
		if err = json.Unmarshal(body, &envelope); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if envelope["type"] == "verification" {
			_ = json.NewEncoder(w).Encode(map[string]any{"challenge": envelope["challenge"]})
			return
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(r.Header.Get("Webhook-Id") + "." + r.Header.Get("Webhook-Timestamp") + "."))
		mac.Write(body)
		actual, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(r.Header.Get("Webhook-Signature"), "v1,"))
		if err != nil || !hmac.Equal(actual, mac.Sum(nil)) {
			t.Error("invalid HMAC")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if envelope["name"] == domain.JournalReadyV1 {
			schema, compileErr := contracts.Compile("journal_ready_v1", "payload")
			if compileErr != nil {
				t.Error(compileErr)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if err = schema.Validate(envelope["data"]); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			mu.Lock()
			bodies = append(bodies, body)
			ids = append(ids, r.Header.Get("Webhook-Id"))
			n := len(bodies)
			mu.Unlock()
			if n == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	w, s, at := readyWorker(t, callbackRoundTrip(recoveryChallenge))
	var logs, trace bytes.Buffer
	w.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	w.TraceLogger = slog.New(slog.NewJSONHandler(&trace, nil))
	policy, err := NewCallbackPolicy([]string{receiver.URL + "/business", receiver.URL + "/control"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(
		"UPDATE event_subscriptions SET callback=? WHERE id='subscription'",
		receiver.URL+"/business",
	); err != nil {
		t.Fatal(err)
	}
	w.Callbacks = policy
	w.Client = policy.Client()
	m, err := NewSubscriptionManager(s, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	m.Callbacks = policy
	m.Client = policy.Client()
	var gen, secret string
	if err = s.DB.QueryRow("SELECT generation,secret FROM event_subscriptions WHERE id='subscription'").
		Scan(&gen, &secret); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Call(
		ctx,
		methodEventsSubscribe,
		"owner",
		recoveryRequest(t, "subscription", gen, receiver.URL+"/control", secret),
	); err != nil {
		t.Fatal(err)
	}
	// Act: pending business cannot emit a control readiness notification.
	if err = w.sweepRecovery(ctx, at); err != nil {
		t.Fatal(err)
	}
	if handled, deliveryErr := w.DeliverRecoveryOne(ctx, at); deliveryErr != nil || handled {
		t.Fatalf("premature wakeup handled=%v err=%v", handled, deliveryErr)
	}
	if handled, deliveryErr := w.DeliverOne(ctx, at); deliveryErr != nil || !handled {
		t.Fatalf("business delivery handled=%v err=%v", handled, deliveryErr)
	}
	if err = w.sweepRecovery(ctx, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// Assert / Act: first control request fails, retry retains ID and body.
	for _, when := range []time.Time{at.Add(time.Second), at.Add(5 * time.Second)} {
		if handled, deliveryErr := w.DeliverRecoveryOne(ctx, when); deliveryErr != nil || !handled {
			t.Fatalf("control handled=%v err=%v", handled, deliveryErr)
		}
	}
	mu.Lock()
	if len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) || ids[0] != ids[1] {
		t.Error("retry changed identity/body")
	}
	mu.Unlock()
	// Assert: an already accepted control cannot produce a reminder before its rate bound.
	if err = w.sweepRecovery(ctx, at.Add(10*time.Second)); err != nil {
		t.Fatal(err)
	}
	if handled, deliveryErr := w.DeliverRecoveryOne(ctx, at.Add(10*time.Second)); deliveryErr != nil || handled {
		t.Fatalf("early reminder handled=%v err=%v", handled, deliveryErr)
	}
	// Act: a later reminder has a fresh transport ID but same recovery episode.
	if err = w.sweepRecovery(ctx, at.Add(35*time.Second)); err != nil {
		t.Fatal(err)
	}
	if handled, deliveryErr := w.DeliverRecoveryOne(ctx, at.Add(35*time.Second)); deliveryErr != nil || !handled {
		t.Fatalf("reminder handled=%v err=%v", handled, deliveryErr)
	}
	mu.Lock()
	if len(ids) != 3 || ids[2] == ids[1] {
		t.Error("reminder identity not fresh")
	}
	mu.Unlock()
	var controlEnvelope struct {
		Data struct {
			Reference string `json:"recovery_reference"`
		} `json:"data"`
	}
	mu.Lock()
	decodeErr := json.Unmarshal(bodies[0], &controlEnvelope)
	mu.Unlock()
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	status, err := s.RecoveryStatus(ctx, "owner", "subscription")
	if err != nil {
		t.Fatal(err)
	}
	if status["reference"] != controlEnvelope.Data.Reference {
		t.Fatal("status and control episode reference differ")
	}
	assertRecoveryLogReferences(t, logs.String(), controlEnvelope.Data.Reference, secret, receiver.URL, gen)
	assertRecoveryLogReferences(t, trace.String(), controlEnvelope.Data.Reference, secret, receiver.URL, gen)
	page, err := s.ReadSubscriptionEvents(domain.WithEventDiagnostics(ctx), "owner", "subscription", 10)
	if err != nil {
		t.Fatal(err)
	}
	if page["recovery_reference"] != controlEnvelope.Data.Reference {
		t.Fatal("journal and logged control episode reference differ")
	}
	entries := page["events"].([]map[string]any)
	if len(entries) != 1 {
		t.Fatalf("business journal polluted: %v", page)
	}
	token := entries[0]["receipt"].(string)
	if _, err = s.AckSubscriptionEvents(ctx, "owner", "subscription", token); err != nil {
		t.Fatal(err)
	}
	// Assert: ack cancels future control reminders, never recursively adding business events.
	if err = w.sweepRecovery(ctx, at.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if handled, deliveryErr := w.DeliverRecoveryOne(ctx, at.Add(2*time.Minute)); deliveryErr != nil || handled {
		t.Fatalf("ack left wakeup handled=%v err=%v", handled, deliveryErr)
	}
	var count int
	if err = s.DB.QueryRow("SELECT count(*) FROM event_deliveries").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("control recursively added business deliveries=%d", count)
	}
}

//nolint:gocognit // Assert all safe diagnostic fields and retry/reminder relationships in each sink.
func assertRecoveryLogReferences(t *testing.T, raw, episode string, private ...string) {
	t.Helper()
	for _, value := range private {
		if value != "" && strings.Contains(raw, value) {
			t.Fatal("private callback/credential/generation leaked into logs")
		}
	}
	var references []string
	correlations := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if record["delivery_kind"] != "journal_recovery" {
			continue
		}
		if record["recovery_reference"] != episode {
			t.Fatal("missing control episode correlation")
		}
		ref, ok := record["delivery_reference"].(string)
		if !ok || !strings.HasPrefix(ref, "rdlv_") || len(ref) != 48 {
			t.Fatal("invalid control delivery reference")
		}
		if record["msg"] == "mcp_event_delivery_started" || record["msg"] == "mcp_event_request" {
			references = append(references, ref)
			id, ok := record["correlation_id"].(string)
			if !ok || correlations[id] {
				t.Fatal("attempt correlation reused")
			}
			correlations[id] = true
		}
		for _, key := range []string{"payload", "subscription_id", "event_id", "callback", "secret", "message_id", "episode_id"} {
			if _, present := record[key]; present {
				t.Fatalf("private diagnostic field %s", key)
			}
		}
	}
	if len(references) != 3 || references[0] != references[1] || references[2] == references[1] {
		t.Fatal("control retry/reminder references invalid")
	}
}

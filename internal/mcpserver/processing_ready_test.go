package mcpserver

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// These are synthetic-host proofs over actual MCP SDK HTTP and signed TLS
// callbacks. The authorized "action" is only a local counter, never a Zalo write.
type processingAttempt struct {
	eventID                       string
	webhookID                     string
	body                          []byte
	legacy, persisted             map[string]any
	stateBeforeAck, stateAfterAck string
	payloadAfterAck               int
	err                           error
}

func persistedJournal(h *recoveryReadyHarness) (map[string]any, error) {
	return h.call(
		"zalo_read_subscription_events",
		map[string]any{"subscription_id": h.business, "readiness_mode": "persisted", "limit": 20},
	)
}
func defaultJournal(h *recoveryReadyHarness) (map[string]any, error) {
	return h.call("zalo_read_subscription_events", map[string]any{"subscription_id": h.business, "limit": 20})
}
func processingDeliveryState(h *recoveryReadyHarness) (string, int, error) {
	var state string
	var size int
	err := h.store.DB.QueryRow("SELECT state,length(payload) FROM event_deliveries WHERE subscription_id=?", h.business).
		Scan(&state, &size)
	return state, size, err
}
func processingObservation(h *recoveryReadyHarness, body []byte, event map[string]any, ack bool) processingAttempt {
	result := processingAttempt{eventID: event["eventId"].(string), body: append([]byte(nil), body...)}
	result.legacy, result.err = defaultJournal(h)
	if result.err != nil {
		return result
	}
	result.persisted, result.err = persistedJournal(h)
	if result.err != nil {
		return result
	}
	result.stateBeforeAck, _, result.err = processingDeliveryState(h)
	if result.err != nil {
		return result
	}
	if ack {
		result.err = h.process(result.persisted)
		if result.err != nil {
			return result
		}
	}
	result.stateAfterAck, result.payloadAfterAck, result.err = processingDeliveryState(h)
	return result
}
func takeProcessingAttempt(t *testing.T, attempts <-chan processingAttempt) processingAttempt {
	t.Helper()
	select {
	case got := <-attempts:
		if got.err != nil {
			t.Fatal(got.err)
		}
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("processing callback timeout")
		return processingAttempt{}
	}
}
func assertProcessingSendingReady(t *testing.T, attempt processingAttempt) {
	t.Helper()
	if attempt.legacy["blocked_on_delivery"] != true || len(attempt.legacy["events"].([]any)) != 0 {
		t.Fatal("legacy mode changed blocked semantics")
	}
	for _, field := range []string{"readiness_mode", "processing_state", "retry_after_ms", "recovery_mode"} {
		if _, ok := attempt.legacy[field]; ok {
			t.Fatalf("legacy result added opt-in key %s", field)
		}
	}
	if attempt.persisted["readiness_mode"] != "persisted" || attempt.persisted["blocked_on_delivery"] != false {
		t.Fatal("persisted mode blocked on HTTP")
	}
	entries := attempt.persisted["events"].([]any)
	if len(entries) != 1 {
		t.Fatal("persisted exact event missing")
	}
	entry := entries[0].(map[string]any)
	if entry["processing_state"] != "ready" || entry["delivery_state"] != "sending" || entry["gap_reason"] != nil ||
		entry["event"].(map[string]any)["eventId"] != attempt.eventID {
		t.Fatal("processing/transport identity or states conflated")
	}
	if attempt.stateBeforeAck != "sending" || attempt.stateAfterAck != "sending" || attempt.payloadAfterAck == 0 {
		t.Fatal("early processing deleted or delivered unfinished outbox")
	}
}
func assertProcessingCheckpointEmpty(t *testing.T, h *recoveryReadyHarness) {
	t.Helper()
	for _, mode := range []string{"delivery_terminal", "persisted"} {
		page, err := h.call(
			"zalo_read_subscription_events",
			map[string]any{"subscription_id": h.business, "readiness_mode": mode},
		)
		if err != nil || len(page["events"].([]any)) != 0 || page["blocked_on_delivery"] != false {
			t.Fatalf("mode %s did not share acknowledged checkpoint: %v", mode, err)
		}
	}
}

//nolint:gocognit // Two ordered AAA paths differ only in the exact callback/receipt-commit window used to acknowledge.
func TestSyntheticHostPersistedJournalAcknowledgesBothCallbackRaceWindowsWithoutCompanion(t *testing.T) {
	for _, ackBefore2xx := range []bool{true, false} {
		t.Run(fmt.Sprintf("ack-before2xx-%v", ackBefore2xx), func(t *testing.T) {
			// Arrange: a normal business subscription, no companion and no changed permissions.
			h := newRecoveryReadyHarness(t)
			h.assertNoNativeActions()
			attempts := make(chan processingAttempt, 2)
			h.mu.Lock()
			h.businessHook = func(w http.ResponseWriter, r *http.Request, event map[string]any, body []byte) {
				attempt := processingObservation(h, body, event, ackBefore2xx)
				attempt.webhookID = r.Header.Get("Webhook-Id")
				attempts <- attempt
				w.WriteHeader(http.StatusNoContent)
			}
			h.mu.Unlock()
			h.put("processable-before-receipt")
			barrier := &recoveryFinishBarrier{
				Store:   h.store,
				entered: make(chan struct{}),
				release: make(chan struct{}),
			}
			h.worker.Queue = barrier
			t.Cleanup(func() {
				select {
				case <-barrier.release:
				default:
					close(barrier.release)
				}
			})
			finished := make(chan error, 1)
			go func() {
				handled, err := h.worker.DeliverOne(t.Context(), time.Now())
				if err == nil && !handled {
					err = errors.New("synthetic callback was not dispatched")
				}
				finished <- err
			}()
			// Act: callback itself reads through actual SDK before its2xx response.
			attempt := takeProcessingAttempt(t, attempts)
			assertProcessingSendingReady(t, attempt)
			select {
			case <-barrier.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("post2xx/precommit barrier not reached")
			}
			if !ackBefore2xx {
				// The other causal window is after successful HTTP but before worker commit.
				pending, err := persistedJournal(h)
				if err != nil {
					t.Fatal(err)
				}
				entries := pending["events"].([]any)
				if len(entries) != 1 || entries[0].(map[string]any)["delivery_state"] != "sending" {
					t.Fatal("post2xx window incorrectly waited for delivered")
				}
				if err = h.process(pending); err != nil {
					t.Fatal(err)
				}
			}
			// Assert: early ack moved only processing; unfinished payload/lease still belong to worker.
			state, size, err := processingDeliveryState(h)
			if err != nil || state != "sending" || size == 0 {
				t.Fatal("processing ack corrupted delivery state", err)
			}
			assertProcessingCheckpointEmpty(t, h)
			close(barrier.release)
			if err = <-finished; err != nil {
				t.Fatal(err)
			}
			state, _, err = processingDeliveryState(h)
			if err != nil || state != "delivered" {
				t.Fatal("2xx receipt did not independently mark delivery", err)
			}
			h.mu.Lock()
			count := h.actions[attempt.eventID]
			h.mu.Unlock()
			if count != 1 {
				t.Fatal("synthetic action not exactly once in this authorized harness")
			}
			h.deliverControl(time.Now().Add(time.Minute), false)
			h.assertNoNativeActions()
		})
	}
}

//nolint:gocognit // Ordered AAA paths verify independent durable processing and retry identity across transient HTTP failure and owned DB restart.
func TestSyntheticHostPersistedEarlyAckSurvives503AndTimeoutRetryAfterRestart(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprintf("timeout-%v", timeout), func(t *testing.T) {
			// Arrange: first callback processes+acks before a503 or an actual HTTP client timeout.
			h := newRecoveryReadyHarness(t)
			h.assertNoNativeActions()
			attempts := make(chan processingAttempt, 3)
			callNumber := 0
			h.mu.Lock()
			h.businessHook = func(w http.ResponseWriter, r *http.Request, event map[string]any, body []byte) {
				callNumber++
				attempt := processingObservation(h, body, event, true)
				attempt.webhookID = r.Header.Get("Webhook-Id")
				attempts <- attempt
				if callNumber == 1 {
					if timeout {
						<-r.Context().Done()
						return
					}
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}
			h.mu.Unlock()
			if timeout {
				client := *h.callbackClient
				client.Timeout = time.Second
				h.worker.Client = &client
			}
			h.put("acknowledged-before-failure")
			// Act: early ack succeeds while actual delivery has not received acceptance.
			handled, err := h.worker.DeliverOne(t.Context(), time.Now())
			if err != nil || !handled {
				t.Fatal("first callback", err)
			}
			first := takeProcessingAttempt(t, attempts)
			assertProcessingSendingReady(t, first)
			state, size, err := processingDeliveryState(h)
			if err != nil || state != "pending" || size == 0 {
				t.Fatal("failed callback lost unfinished original payload", err)
			}
			assertProcessingCheckpointEmpty(t, h)
			// Restart real temporary DB owner, service, MCP SDK session and worker. No new message is inserted.
			h.stop()
			h.open()
			assertProcessingCheckpointEmpty(t, h)
			handled, err = h.worker.DeliverOne(t.Context(), time.Now().Add(5*time.Second))
			if err != nil || !handled {
				t.Fatal("callback retry after restart", err)
			}
			second := takeProcessingAttempt(t, attempts)
			// Assert: original delivery ID/body survive retry, but processing is already complete.
			if first.eventID != second.eventID || first.webhookID == "" || first.webhookID != second.webhookID ||
				!bytes.Equal(first.body, second.body) ||
				len(second.persisted["events"].([]any)) != 0 ||
				len(second.legacy["events"].([]any)) != 0 {
				t.Fatal("retry identity changed or acknowledged event reprocessed")
			}
			state, _, err = processingDeliveryState(h)
			if err != nil || state != "delivered" {
				t.Fatal("retry acceptance not committed", err)
			}
			h.mu.Lock()
			count := h.actions[first.eventID]
			h.mu.Unlock()
			if count != 1 {
				t.Fatal("callback retry repeated synthetic action")
			}
			h.assertNoNativeActions()
		})
	}
}

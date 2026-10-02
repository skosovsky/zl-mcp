package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func TestDeliveryDiagnosticsAreBoundedAndHidePrivateValues(t *testing.T) {
	// Arrange: one queued delivery and two capacity failures.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	queueSubscription(t, s, "sub", at)
	for _, id := range []string{"one", "two", "three"} {
		putTest(t, s, id, "g1", "PRIVATE_MESSAGE_BODY", at)
	}
	policy := DefaultDeliveryPolicy()
	policy.MaxJobs = 1
	if _, err := s.FanoutEvents(ctx, at, policy, queueEncoder); err != nil {
		t.Fatal(err)
	}
	// Act
	diagnostics, err := s.DeliveryDiagnostics(ctx, at)
	// Assert: actual capacity failures are visible with no payload or callback data.
	if err != nil || diagnostics.ActiveSubscriptions != 1 || diagnostics.JournalPending != 0 || diagnostics.StateCounts["pending"] != 1 || diagnostics.StateCounts["failed"] != 2 || diagnostics.ReasonCounts["queue_capacity"] != 2 || diagnostics.PendingPayloadBytes <= 0 {
		t.Fatalf("diagnostics=%+v err=%v", diagnostics, err)
	}
	// Arrange: raw corrupted reason/state must never become response keys.
	if _, err := s.DB.Exec("UPDATE event_deliveries SET last_reason='PRIVATE_CALLBACK_SECRET',state='PRIVATE_STATE' WHERE id=(SELECT max(id) FROM event_deliveries)"); err != nil {
		t.Fatal(err)
	}
	// Act
	diagnostics, err = s.DeliveryDiagnostics(ctx, at)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := contracts.Compile("events_diagnostics", "output")
	if err != nil {
		t.Fatal(err)
	}
	var wire any
	json.Unmarshal(body, &wire)
	// Assert: unknown data is folded to a bounded category and schema remains valid.
	if err := schema.Validate(wire); err != nil {
		t.Fatal(err)
	}
	if diagnostics.StateCounts["other"] != 1 || diagnostics.ReasonCounts["other"] != 1 || strings.Contains(string(body), "PRIVATE_") || strings.Contains(string(body), "callback.example") || strings.Contains(string(body), "synthetic") {
		t.Fatalf("unsafe diagnostics: %s", body)
	}
}

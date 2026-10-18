package storage

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestJournalDiagnosticsAreOptInClosedAndStableAcrossRestart(t *testing.T) {
	// Arrange: a real durable delivery and a reason that must not be disclosed.
	ctx := context.Background()
	s, path, _, at := journalFixture(t)
	d, err := s.ClaimDelivery(ctx, at, DefaultDeliveryPolicy())
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	ref := s.DeliveryReference(d.ID)
	if err = s.FinishDelivery(ctx, *d, DeliveryOutcome{State: "failed", Reason: "PRIVATE_REASON"}, at); err != nil {
		t.Fatal(err)
	}
	// Act: old clients keep their exact shape; opted-in readers receive diagnostics.
	legacy := journalRecords(t, s, 20)
	page, err := s.ReadSubscriptionEvents(domain.WithEventDiagnostics(ctx), "owner", "sub", 20)
	if err != nil {
		t.Fatal(err)
	}
	entries := page["events"].([]map[string]any)
	// Assert: no raw reason/identity leaks, stable references bind to the durable row.
	if _, exists := legacy[0]["delivery_reason"]; exists {
		t.Fatal("legacy shape changed")
	}
	if entries[0]["delivery_reason"] != "unknown" || entries[0]["delivery_reference"] != ref ||
		page["retry_after_ms"] != 1000 {
		t.Fatal(page)
	}
	for _, reason := range []string{"queue_capacity", "deadline", "attempts", "payload_too_large", "http_rejected"} {
		if safeDeliveryReason(reason) != reason {
			t.Fatal(reason)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.DeliveryReference(d.ID) != ref || s.DeliveryReference(d.ID+1) == ref {
		t.Fatal("unstable diagnostic reference")
	}
	body, _ := json.Marshal(entries[0])
	var decoded map[string]any
	if json.Unmarshal(body, &decoded) != nil || decoded["gap_reason"] != "delivery_failed" {
		t.Fatal(string(body))
	}
}

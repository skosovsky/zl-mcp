package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestPersistedProjectionRunsBeforeBudgetWithoutMutatingOutbox(t *testing.T) {
	// Arrange: a corrupt nested extension is larger than the journal response budget.
	s, _, _, _ := processingFixture(t)
	defer s.Close()
	ctx := domain.WithPersistedEventReadiness(context.Background())
	corrupt := oversizedProcessingPayload(t, s)
	if _, err := s.ReadSubscriptionEvents(ctx, "owner", "sub", 2); err == nil {
		t.Fatal("oversized unprojected fixture unexpectedly fit")
	}
	calls := 0
	project := func(entry map[string]any) error {
		calls++
		if entry["processing_state"] == "ready" {
			data := entry["event"].(map[string]any)["data"].(map[string]any)
			if _, invalid := data["invalid_extension"]; invalid {
				entry["event"] = nil
				entry["gap_reason"] = reasonInvalidPayload
				entry["processing_state"] = "gap"
			}
		}
		return nil
	}
	// Act: application validation erases only the response projection, before counting bytes.
	page, err := s.ReadSubscriptionEventsProjected(ctx, "owner", "sub", 2, project)
	if err != nil {
		t.Fatal(err)
	}
	entries := page["events"].([]map[string]any)
	// Assert: ordered gap plus valid tail fit; raw outbox bytes and transport state remain intact.
	if calls != 2 || len(entries) != 2 || entries[0]["event"] != nil ||
		entries[0]["gap_reason"] != reasonInvalidPayload ||
		entries[1]["processing_state"] != "ready" {
		t.Fatalf("projection=%v calls=%d", page, calls)
	}
	assertUnfinishedProcessingBytes(t, s, 1, statePendingText, corrupt)
	if _, err = s.AckSubscriptionEvents(ctx, "owner", "sub", entries[0]["receipt"].(string)); err != nil {
		t.Fatal(err)
	}
	next, err := s.ReadSubscriptionEventsProjected(ctx, "owner", "sub", 1, project)
	if err != nil {
		t.Fatal(err)
	}
	if next["events"].([]map[string]any)[0]["event"].(map[string]any)["data"].(map[string]any)["message_id"] != "two" {
		t.Fatal("valid tail stranded")
	}
}

func oversizedProcessingPayload(t *testing.T, s *Store) []byte {
	t.Helper()
	var raw []byte
	if err := s.DB.QueryRow(`SELECT payload FROM event_deliveries WHERE id=1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	event["data"].(map[string]any)["invalid_extension"] = strings.Repeat("x", 100000)
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	setProcessingBody(t, s, string(body))
	return body
}

func TestPersistedProjectionFailureKeepsCheckpointAndResponseBudget(t *testing.T) {
	// Arrange.
	s, _, _, _ := processingFixture(t)
	defer s.Close()
	ctx := domain.WithPersistedEventReadiness(context.Background())
	infrastructure := errors.New("synthetic projector infrastructure failure")
	// Act.
	_, err := s.ReadSubscriptionEventsProjected(
		ctx,
		"owner",
		"sub",
		1,
		func(map[string]any) error { return infrastructure },
	)
	// Assert: an infrastructure fault is not acknowledged, hidden or converted to a data gap.
	if !errors.Is(err, infrastructure) {
		t.Fatalf("projection error=%v", err)
	}
	var ack int64
	if err = s.DB.QueryRow(`SELECT COALESCE((SELECT ack_id FROM event_processing WHERE subscription_id='sub'),0)`).
		Scan(&ack); err != nil ||
		ack != 0 {
		t.Fatalf("checkpoint=%d %v", ack, err)
	}
	oversizedProcessingPayload(t, s)
	if _, err = s.ReadSubscriptionEventsProjected(
		ctx,
		"owner",
		"sub",
		1,
		func(map[string]any) error { return nil },
	); err == nil {
		t.Fatal("pure projection bypassed response budget")
	}
}

func TestJournalProjectionNeverChangesLegacyView(t *testing.T) {
	// Arrange.
	s, _, _, at := processingFixture(t)
	defer s.Close()
	ctx := context.Background()
	calls := 0
	project := func(map[string]any) error { calls++; return errors.New("legacy projector must not execute") }
	// Act: both pending and terminal legacy representations use the historical path.
	pending, err := s.ReadSubscriptionEventsProjected(ctx, "owner", "sub", 1, project)
	if err != nil || pending["blocked_on_delivery"] != true || len(pending) != 4 {
		t.Fatalf("legacy pending=%v %v", pending, err)
	}
	finishJournal(t, s, at)
	terminal, err := s.ReadSubscriptionEventsProjected(ctx, "owner", "sub", 1, project)
	// Assert.
	if err != nil || calls != 0 || len(terminal) != 4 || terminal["blocked_on_delivery"] != false {
		t.Fatalf("legacy terminal=%v calls=%d err=%v", terminal, calls, err)
	}
	entry := terminal["events"].([]map[string]any)[0]
	if len(entry) != 4 || entry["processing_state"] != nil {
		t.Fatal("legacy entry keys changed")
	}
}

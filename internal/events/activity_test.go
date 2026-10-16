package events

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:gocognit,govet // Keep this AAA sequence and local error checks adjacent for end-to-end assertions.
func TestActivityLedgerHasIndependentBoundaryAndNoMessages(t *testing.T) {
	// Arrange: no target message, no existing conversation, no message sequence.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.sqlite")
	s, err := storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	at := time.Now().UTC()
	a := domain.Activity{
		Family:           "reaction",
		SchemaVersion:    1,
		ConversationType: "direct",
		ConversationID:   "peer",
		ActorID:          "peer",
		OccurredAt:       at,
		Direction:        "incoming",
		SourceID:         "before",
		IdentityBasis:    "upstream_action_id",
		Source:           "live",
		Reaction:         &domain.ReactionObservation{TargetMessageIDs: []string{"unknown-target"}, Action: "unknown"},
	}
	if err := s.PutActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []string{domain.ReactionObservedV1, domain.CallObservedV1, domain.ConversationMessageCreatedV3} {
		sub := storage.EventSubscription{
			ID:               profile,
			Profile:          profile,
			Principal:        "owner",
			Scope:            "direct",
			ConversationType: "direct",
			Callback:         "https://receiver.example",
			Secret:           "synthetic",
		}
		revision, err := s.SubscriptionRevision(ctx, sub.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ActivateSubscription(ctx, sub, revision, at); err != nil {
			t.Fatal(err)
		}
	}
	// Act: repeated old observation is suppressed; new one survives a restart.
	if err := s.PutActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	a.SourceID = "after"
	if err := s.PutActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	a.Family = "call"
	a.SourceID = "call"
	a.Reaction = nil
	a.Call = &domain.CallObservation{Status: "unknown", Mode: "unknown"}
	if err := s.PutActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewWorker(s)
	if err != nil {
		t.Fatal(err)
	}
	n, err := worker.fanout(ctx, at)
	// Assert: exactly two sources AFTER the activity boundary, no fake message.
	if err != nil || n != 3 {
		t.Fatal(n, err)
	}
	for _, table := range []string{"messages", "message_events", "message_identities", "peer_first_incoming"} {
		var count int
		if err := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("activity changed message state", table, count)
		}
	}
	for range 2 {
		delivery, err := s.ClaimDelivery(ctx, at, worker.Policy)
		if err != nil || delivery == nil {
			t.Fatal(delivery, err)
		}
		var body map[string]any
		if err := json.Unmarshal(delivery.Payload, &body); err != nil {
			t.Fatal(err)
		}
		if body["data"].(map[string]any)["source_id"] == "before" {
			t.Fatal("replayed pre-activation activity")
		}
		if err := s.FinishDelivery(
			ctx,
			*delivery,
			storage.DeliveryOutcome{State: "delivered", Reason: "accepted"},
			at,
		); err != nil {
			t.Fatal(err)
		}
		page, err := s.ReadSubscriptionEvents(ctx, "owner", delivery.SubscriptionID, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(page["events"].([]map[string]any)) != 1 {
			t.Fatal("message-free journal lost activity")
		}
	}
	if delivery, err := s.ClaimDelivery(ctx, at, worker.Policy); err != nil || delivery != nil {
		t.Fatal("cross-family delivery", delivery, err)
	}
}

func TestActivityEncodersMatchDraftContracts(t *testing.T) {
	// Arrange.
	encoder, err := NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	a := domain.Activity{
		Family:           "call",
		SchemaVersion:    1,
		ConversationType: "direct",
		ConversationID:   "peer",
		ActorID:          "peer",
		OccurredAt:       time.Now().UTC(),
		Direction:        "incoming",
		SourceID:         "action",
		IdentityBasis:    "upstream_action_id",
		Source:           "live",
		Call:             &domain.CallObservation{Status: "unknown", Mode: "unknown"},
	}
	// Act.
	wire, err := encoder.EncodeActivity(a.Profile(), "act-synthetic", a)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	json.Unmarshal(wire, &value)
	schema, err := contracts.Compile("call_v1_event", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	// Assert.
	if err := schema.Validate(value); err != nil {
		t.Fatal(err)
	}
}

//nolint:gocognit,govet // Keep this AAA sequence and local error checks adjacent for end-to-end assertions.
func TestActivityFanoutIsAtomicAndRetentionCannotReplay(t *testing.T) {
	// Arrange: a fresh post-activation observation.
	ctx := context.Background()
	s, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Now().UTC()
	sub := storage.EventSubscription{
		ID:        "reaction",
		Profile:   domain.ReactionObservedV1,
		Principal: "owner",
		Scope:     "all",
		Callback:  "https://receiver.example",
		Secret:    "synthetic",
	}
	rev, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivateSubscription(ctx, sub, rev, at); err != nil {
		t.Fatal(err)
	}
	a := domain.Activity{
		Family:           "reaction",
		SchemaVersion:    1,
		ConversationType: "direct",
		ConversationID:   "peer",
		ActorID:          "peer",
		OccurredAt:       at,
		Direction:        "incoming",
		SourceID:         "action",
		IdentityBasis:    "upstream_action_id",
		Source:           "live",
		Reaction:         &domain.ReactionObservation{TargetMessageIDs: []string{}, Action: "unknown"},
	}
	if err := s.PutActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	worker, err := NewWorker(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(
		`CREATE TRIGGER reject_activity_fanout BEFORE UPDATE ON activity_fanout BEGIN SELECT RAISE(ABORT,'synthetic');END`,
	); err != nil {
		t.Fatal(err)
	}
	// Act: fail after the outbox insertion but before committing the watermark.
	_, err = worker.fanout(ctx, at)
	// Assert: no partial outbox row or watermark survives.
	if err == nil {
		t.Fatal("watermark failure ignored")
	}
	var count, watermark int
	s.DB.QueryRow(`SELECT count(*) FROM event_deliveries`).Scan(&count)
	s.DB.QueryRow(`SELECT seq FROM activity_fanout WHERE id=1`).Scan(&watermark)
	if count != 0 || watermark != 0 {
		t.Fatal("fanout escaped transaction", count, watermark)
	}
	if _, err := s.DB.Exec(`DROP TRIGGER reject_activity_fanout`); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.fanout(ctx, at); err != nil {
		t.Fatal(err)
	}
	// Act: expire before delivery, then replay identical upstream observation.
	if err := s.PruneActivities(ctx, at.AddDate(0, 0, 2)); err != nil {
		t.Fatal(err)
	}
	if err := s.PutActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.fanout(ctx, at); err != nil {
		t.Fatal(err)
	}
	if delivery, err := s.ClaimDelivery(ctx, at, worker.Policy); err != nil || delivery != nil {
		t.Fatal("retained source replayed", delivery, err)
	}
	page, err := s.ReadSubscriptionEvents(ctx, "owner", sub.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	entry := page["events"].([]map[string]any)[0]
	// Assert: acknowledged gap is explicit, not a fake message/event.
	if entry["event"] != nil || entry["gap_reason"] != "delivery_cancelled" {
		t.Fatal("retention gap hidden", entry["gap_reason"])
	}
	if _, err := s.AckSubscriptionEvents(ctx, "owner", sub.ID, entry["receipt"].(string)); err != nil {
		t.Fatal(err)
	}
	var retained int
	s.DB.QueryRow(`SELECT count(*) FROM activity_observations WHERE expired=1 AND payload_json=''`).Scan(&retained)
	if retained != 1 {
		t.Fatal("deduplication tombstone lost")
	}
}

func TestMaximumEscapedReactionFitsRecoveryAndAcknowledgement(t *testing.T) {
	// Arrange: every bounded free-text field uses JSON's six-byte escaping case.
	ctx := t.Context()
	s, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Now().UTC()
	sub := storage.EventSubscription{
		ID:        "reaction",
		Profile:   domain.ReactionObservedV1,
		Principal: "owner",
		Scope:     "all",
		Callback:  "https://receiver.example",
		Secret:    "synthetic",
	}
	rev, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateSubscription(ctx, sub, rev, at); err != nil {
		t.Fatal(err)
	}
	escaped := strings.Repeat("\x00", 256)
	icon := strings.Repeat("\x00", 128)
	targets := make([]string, 16)
	for i := range targets {
		targets[i] = strings.Repeat("\x00", 62) + fmt.Sprintf("%02d", i)
	}
	a := domain.Activity{
		Family:           "reaction",
		SchemaVersion:    1,
		ConversationType: "direct",
		ConversationID:   escaped,
		ConversationName: &escaped,
		ActorID:          escaped,
		ActorName:        &escaped,
		OccurredAt:       at,
		Direction:        "incoming",
		SourceID:         escaped,
		IdentityBasis:    "upstream_action_id",
		Source:           "live",
		Reaction:         &domain.ReactionObservation{TargetMessageIDs: targets, Reaction: &icon, Action: "unknown"},
	}
	// Act: persist, encode, deliver, and read the real bounded journal response.
	if err = s.PutActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	worker, err := NewWorker(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = worker.fanout(ctx, at); err != nil {
		t.Fatal(err)
	}
	delivery, err := s.ClaimDelivery(ctx, at, worker.Policy)
	if err != nil || delivery == nil {
		t.Fatal("missing maximum-size delivery", err)
	}
	if err = s.FinishDelivery(
		ctx,
		*delivery,
		storage.DeliveryOutcome{State: "delivered", Reason: "accepted"},
		at,
	); err != nil {
		t.Fatal(err)
	}
	page, err := s.ReadSubscriptionEvents(ctx, "owner", sub.ID, 20)
	// Assert: a legal activity cannot wedge recovery behind an unreadable entry.
	if err != nil {
		t.Fatal(err)
	}
	entries := page["events"].([]map[string]any)
	if len(entries) != 1 || entries[0]["event"] == nil {
		t.Fatal("maximum activity unavailable")
	}
	if _, err = s.AckSubscriptionEvents(ctx, "owner", sub.ID, entries[0]["receipt"].(string)); err != nil {
		t.Fatal(err)
	}
}

package storage

import (
	"context"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestLegacyRollbackRefusesNewProfilesAndUnprocessedActivities(t *testing.T) {
	// Arrange: freshly migrated state remains safe for an old explicit-column reader.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	check := func(want bool) {
		t.Helper()
		report, err := CheckLegacyRollback(ctx, s.DB)
		if err != nil || report.Compatible != want {
			t.Fatal(report, err)
		}
	}
	check(true)
	sub := EventSubscription{
		ID:        "new",
		Profile:   domain.ConversationMessageCreatedV3,
		Principal: "owner",
		Scope:     "all",
		Callback:  "https://receiver.example",
		Secret:    "synthetic",
	}
	revision, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Act / Assert: active v3 blocks downgrade even when its queue is empty.
	if _, err = s.ActivateSubscription(ctx, sub, revision, at); err != nil {
		t.Fatal(err)
	}
	check(false)
	if err = s.CancelSubscription(ctx, sub.ID, "owner", at); err != nil {
		t.Fatal(err)
	}
	check(true)
	// Act: a pending source with no subscribers still needs its watermark completed.
	a := domain.Activity{
		Family:           "reaction",
		SchemaVersion:    1,
		ConversationType: "group",
		ConversationID:   "g1",
		ActorID:          "peer",
		OccurredAt:       at,
		Direction:        "incoming",
		SourceID:         "new",
		IdentityBasis:    "upstream_action_id",
		Source:           "live",
		Reaction:         &domain.ReactionObservation{TargetMessageIDs: []string{}, Action: "unknown"},
	}
	if err = s.PutActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	check(false)
	if _, err = s.FanoutActivityEvents(
		ctx,
		at,
		DefaultDeliveryPolicy(),
		func(_, _ string, _ domain.Activity) ([]byte, error) {
			t.Fatal("no subscriber should need encoding")
			return nil, nil
		},
	); err != nil {
		t.Fatal(err)
	}
	// Fanout alone cannot authorize an old runtime that ignores activity retention.
	check(false)
	report, err := CheckLegacyRollback(ctx, s.DB)
	if err != nil || report.RetainedActivityPayloads != 1 {
		t.Fatal(report, err)
	}
	if err = s.PruneActivities(ctx, at.Add(91*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	check(true)
}

func TestLegacyRollbackRefusesActivityOutboxAfterSourcePruning(t *testing.T) {
	// Arrange: a terminal, processed/orphaned delivery can still retain content.
	s := openTest(t)
	ctx := t.Context()
	_, err := s.DB.ExecContext(
		ctx,
		`INSERT INTO event_deliveries(event_id,subscription_id,generation,payload,state,next_attempt_at,deadline,activity_seq) VALUES('activity','absent','generation',X'0102','delivered','2030-01-01','2030-01-02',1)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	report, err := CheckLegacyRollback(ctx, s.DB)
	// Assert: a missing subscription or source cannot hide retained activity bytes.
	if err != nil || report.Compatible || report.RetainedActivityPayloads != 1 {
		t.Fatal(report, err)
	}
}

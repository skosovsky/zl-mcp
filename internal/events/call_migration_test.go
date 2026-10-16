package events

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:gocognit,gocyclo,cyclop // Keep the AAA migration/restart assertions adjacent.
func TestCallReplayOfLegacyMessageCreatesOnlyDurableTombstone(t *testing.T) {
	for _, sameNamespace := range []bool{true, false} {
		name := "same namespace"
		if !sameNamespace {
			name = "group collision"
		}
		t.Run(name, func(t *testing.T) {
			// Arrange: an older classifier already retained this global message ID.
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "calls.sqlite")
			s, err := storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			kind := "direct"
			if !sameNamespace {
				kind = "group"
			}
			at := time.Now().UTC()
			old := domain.Message{
				Conversation: domain.ConversationRef{Type: kind, ID: "peer"},
				GroupID:      "peer",
				ID:           "legacy-call",
				SenderID:     "peer",
				SentAt:       at,
				Text:         "legacy projection",
				Source:       "live",
				Direction:    "incoming",
			}
			if err = s.Put(ctx, old); err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			for _, table := range []string{"messages", "message_identities", "peer_first_incoming", "message_events"} {
				var count int
				if err = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				counts[table] = count
			}
			sub := storage.EventSubscription{
				ID:               "calls",
				Profile:          domain.CallObservedV1,
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
			if _, err = s.ActivateSubscription(ctx, sub, revision, at); err != nil {
				t.Fatal(err)
			}
			a := domain.Activity{
				Family:            "call",
				SchemaVersion:     1,
				ConversationType:  "direct",
				ConversationID:    "peer",
				ActorID:           "peer",
				OccurredAt:        at,
				Direction:         "incoming",
				SourceID:          "stable-observation",
				IdentityBasis:     "observed_fingerprint",
				Source:            "replay",
				Call:              &domain.CallObservation{Status: "unknown", Mode: "unknown"},
				ProtocolMessageID: old.ID,
			}
			// Act: repeated replay spans a real database reopen.
			if err = s.PutActivity(ctx, a); err != nil {
				t.Fatal(err)
			}
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.PutActivity(ctx, a); err != nil {
				t.Fatal(err)
			}
			worker, err := NewWorker(s)
			if err != nil {
				t.Fatal(err)
			}
			jobs, err := s.FanoutActivityEvents(
				ctx,
				at,
				worker.Policy,
				worker.Encoder.EncodeActivity,
			)
			if err != nil {
				t.Fatal(err)
			}
			// Assert: legacy direct source stays silent; same ID in group is independent.
			want := 1
			if sameNamespace {
				want = 0
			}
			if jobs != 1 {
				t.Fatal("activity watermark did not account for source", jobs)
			}
			var deliveries int
			if err = s.DB.QueryRow("SELECT count(*) FROM event_deliveries WHERE subscription_id='calls'").
				Scan(&deliveries); err != nil {
				t.Fatal(err)
			}
			if deliveries != want {
				t.Fatal("cross-ledger replay duplicated or lost call", deliveries, want)
			}
			var expired int
			var body string
			if err = s.DB.QueryRow("SELECT expired,payload_json FROM activity_observations").
				Scan(&expired, &body); err != nil {
				t.Fatal(err)
			}
			if sameNamespace && (expired != 1 || body != "") {
				t.Fatal("legacy call lacks silent tombstone")
			}
			for table, wantCount := range counts {
				var count int
				if err = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != wantCount {
					t.Fatal("call modified message or novelty ledger", table)
				}
			}
			if sameNamespace {
				a.SourceID = "fresh-observation"
				a.ProtocolMessageID = "fresh-record"
				if err = s.PutActivity(ctx, a); err != nil {
					t.Fatal(err)
				}
				if err = s.PutActivity(ctx, a); err != nil {
					t.Fatal(err)
				}
				jobs, err = s.FanoutActivityEvents(
					ctx,
					at,
					worker.Policy,
					worker.Encoder.EncodeActivity,
				)
				if err != nil || jobs != 1 {
					t.Fatal("fresh call not delivered exactly once to outbox", jobs, err)
				}
				if err = s.DB.QueryRow("SELECT count(*) FROM event_deliveries WHERE subscription_id='calls'").
					Scan(&deliveries); err != nil ||
					deliveries != 1 {
					t.Fatal("fresh replay created wrong delivery count", deliveries, err)
				}
			}
		})
	}
}

//nolint:gocognit // Keep the retained callback/ack regression in one AAA sequence.
func TestCallUpgradePreservesPriorMessagePayloadAndAcknowledgement(t *testing.T) {
	// Arrange: old message was delivered and processed before the call classifier.
	ctx := context.Background()
	s, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "old.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Now().UTC()
	sub := storage.EventSubscription{
		ID:               "old",
		Profile:          domain.ConversationMessageCreatedV2,
		Principal:        "owner",
		Scope:            "direct",
		ConversationType: "direct",
		Callback:         "https://receiver.example",
		Secret:           "synthetic",
	}
	rev, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateSubscription(ctx, sub, rev, at); err != nil {
		t.Fatal(err)
	}
	m := domain.Message{
		Conversation: domain.ConversationRef{Type: "direct", ID: "peer"},
		ID:           "legacy-call",
		SenderID:     "peer",
		SentAt:       at,
		Text:         "legacy projection",
		Source:       "live",
		Direction:    "incoming",
	}
	if err = s.Put(ctx, m); err != nil {
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
		t.Fatal("legacy delivery missing", err)
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
	if err != nil {
		t.Fatal(err)
	}
	entries := page["events"].([]map[string]any)
	if len(entries) != 1 {
		t.Fatal("legacy journal missing")
	}
	if _, err = s.AckSubscriptionEvents(ctx, "owner", sub.ID, entries[0]["receipt"].(string)); err != nil {
		t.Fatal(err)
	}
	var beforePayload string
	var beforeAck, beforePruned int64
	if err = s.DB.QueryRow("SELECT CAST(payload AS TEXT) FROM event_deliveries WHERE subscription_id='old'").
		Scan(&beforePayload); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT ack_id,pruned_id FROM event_processing WHERE subscription_id='old'").
		Scan(&beforeAck, &beforePruned); err != nil {
		t.Fatal(err)
	}
	// Act: the newly recognized old source becomes a silent call tombstone.
	a := domain.Activity{
		Family:            "call",
		SchemaVersion:     1,
		ConversationType:  "direct",
		ConversationID:    "peer",
		ActorID:           "peer",
		OccurredAt:        at,
		Direction:         "incoming",
		SourceID:          "stable-observation",
		IdentityBasis:     "observed_fingerprint",
		Source:            "replay",
		Call:              &domain.CallObservation{Status: "unknown", Mode: "unknown"},
		ProtocolMessageID: m.ID,
	}
	if err = s.PutActivity(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err = s.FanoutActivityEvents(ctx, at, worker.Policy, worker.Encoder.EncodeActivity); err != nil {
		t.Fatal(err)
	}
	// Assert: previously signed body and processing checkpoint are byte/value equal.
	var afterPayload string
	var afterAck, afterPruned int64
	var count int
	if err = s.DB.QueryRow("SELECT CAST(payload AS TEXT) FROM event_deliveries WHERE subscription_id='old'").
		Scan(&afterPayload); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT ack_id,pruned_id FROM event_processing WHERE subscription_id='old'").
		Scan(&afterAck, &afterPruned); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM event_deliveries").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if beforePayload != afterPayload || beforeAck != afterAck || beforePruned != afterPruned || count != 1 {
		t.Fatal("call replay changed legacy callback or checkpoint")
	}
}

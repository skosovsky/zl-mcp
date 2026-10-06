package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func journalFixture(t *testing.T) (*Store, string, string, time.Time) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s, err := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	sub := EventSubscription{ID: "sub", Principal: "owner", Profile: domain.ConversationMessageCreatedV2, Scope: "all", Callback: "https://receiver.example", Secret: "synthetic"}
	r, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	sub, err = s.ActivateSubscription(ctx, sub, r, at)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two", "three"} {
		if err = s.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: id, SenderID: "peer", Direction: "incoming", Text: id, SentAt: at, Source: "live"}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.FanoutProfileEvents(ctx, at, DefaultDeliveryPolicy(), func(_, id string, m domain.Message) ([]byte, error) {
		return json.Marshal(map[string]string{"eventId": id, "text": m.Text})
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, path, sub.Generation, at
}
func finishJournal(t *testing.T, s *Store, at time.Time) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		d, err := s.ClaimDelivery(ctx, at, DefaultDeliveryPolicy())
		if err != nil || d == nil {
			t.Fatal(d, err)
		}
		if err = s.FinishDelivery(ctx, *d, DeliveryOutcome{State: "delivered", Reason: "accepted"}, at); err != nil {
			t.Fatal(err)
		}
	}
}
func journalRecords(t *testing.T, s *Store, limit int) []map[string]any {
	t.Helper()
	v, err := s.ReadSubscriptionEvents(context.Background(), "owner", "sub", limit)
	if err != nil {
		t.Fatal(err)
	}
	return v["events"].([]map[string]any)
}
func TestJournalRecoveryCheckpointRestartAndConcurrentReceipts(t *testing.T) {
	// Arrange: no callback payload in the agent; exact durable queue is the source.
	ctx := context.Background()
	s, path, _, at := journalFixture(t)
	finishJournal(t, s, at)
	entries := journalRecords(t, s, 2)
	if len(entries) != 2 || entries[0]["event"].(map[string]any)["text"] != "one" {
		t.Fatal(entries)
	}
	// Act: reading does not acknowledge; out-of-order completion is rejected.
	if len(journalRecords(t, s, 20)) != 3 {
		t.Fatal("read advanced cursor")
	}
	if _, err := s.AckSubscriptionEvents(ctx, "owner", "sub", entries[1]["receipt"].(string)); err == nil {
		t.Fatal("skipped prefix")
	}
	if _, err := s.AckSubscriptionEvents(ctx, "other", "sub", entries[0]["receipt"].(string)); err == nil {
		t.Fatal("foreign owner accepted")
	}
	first := entries[0]["receipt"].(string)
	if _, err := s.AckSubscriptionEvents(ctx, "owner", "sub", first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AckSubscriptionEvents(ctx, "owner", "sub", first); err != nil {
		t.Fatal("retry not idempotent", err)
	}
	if _, err := s.AckSubscriptionEvents(ctx, "owner", "sub", first+"x"); err == nil {
		t.Fatal("tampered receipt accepted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Assert: restart resumes the exact next envelope without lost records.
	remaining := journalRecords(t, s, 20)
	if len(remaining) != 2 || remaining[0]["event"].(map[string]any)["text"] != "two" {
		t.Fatal(remaining)
	}
	for _, e := range remaining {
		if _, err = s.AckSubscriptionEvents(ctx, "owner", "sub", e["receipt"].(string)); err != nil {
			t.Fatal(err)
		}
	}
	if len(journalRecords(t, s, 20)) != 0 {
		t.Fatal("completed records replayed")
	}
}
func TestJournalPendingDeletionRetentionAndCancellation(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	s, _, _, at := journalFixture(t)
	defer s.Close()
	// Act / Assert: unfinished earlier delivery blocks later envelopes.
	page, err := s.ReadSubscriptionEvents(ctx, "owner", "sub", 20)
	if err != nil || page["blocked_on_delivery"] != true || len(page["events"].([]map[string]any)) != 0 {
		t.Fatal(page, err)
	}
	finishJournal(t, s, at)
	if err = s.DeleteConversation(ctx, domain.ConversationRef{Type: "direct", ID: "peer"}, "one"); err != nil {
		t.Fatal(err)
	}
	entries := journalRecords(t, s, 20)
	if entries[0]["event"] != nil || entries[0]["gap_reason"] == nil {
		t.Fatal("deleted message leaked")
	}
	if err = s.PruneDeliveries(ctx, at.Add(8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	entries = journalRecords(t, s, 20)
	if len(entries) != 1 || entries[0]["delivery_state"] != "pruned" {
		t.Fatal(entries)
	}
	if _, err = s.AckSubscriptionEvents(ctx, "owner", "sub", entries[0]["receipt"].(string)); err != nil {
		t.Fatal(err)
	}
	if len(journalRecords(t, s, 20)) != 0 {
		t.Fatal("retention gap repeated")
	}
	if err = s.CancelSubscription(ctx, "sub", "owner", at); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadSubscriptionEvents(ctx, "owner", "sub", 20); err == nil {
		t.Fatal("cancelled journal accessible")
	}
}
func TestRemovedV1MigrationCancelsOnlyV1AndRejectsOldReceipts(t *testing.T) {
	// Arrange: an old persisted profile, alongside the live v2 subscription.
	ctx := context.Background()
	s, path, gen, at := journalFixture(t)
	finishJournal(t, s, at)
	receipt := journalRecords(t, s, 1)[0]["receipt"].(string)
	if _, err := s.DB.Exec(`INSERT INTO event_subscriptions(id,principal,group_id,callback,secret,active,generation,start_seq,created_at,profile,scope,conversation_type,direction,first_incoming_only) SELECT 'old',principal,group_id,callback,secret,active,'old-generation',start_seq,created_at,'zalo.conversation.message.created',scope,conversation_type,direction,first_incoming_only FROM event_subscriptions WHERE id='sub'; INSERT INTO event_deliveries(event_id,subscription_id,generation,payload,state,next_attempt_at,deadline) VALUES('old-event','old','old-generation',X'0102','pending','2030-01-01','2030-01-02')`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	// Act.
	s, err := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Assert: no format conversion or mutation of v2 generation/boundary.
	var active bool
	var g, state string
	var size int
	if err = s.DB.QueryRow(`SELECT active,generation FROM event_subscriptions WHERE id='sub'`).Scan(&active, &g); err != nil || !active || g != gen {
		t.Fatal("v2 changed", err)
	}
	if err = s.DB.QueryRow(`SELECT state,length(payload) FROM event_deliveries WHERE subscription_id='old'`).Scan(&state, &size); err != nil || state != "cancelled" || size != 0 {
		t.Fatal(state, size, err)
	}
	if err = s.CancelSubscription(ctx, "sub", "owner", at); err != nil {
		t.Fatal(err)
	}
	r, _ := s.SubscriptionRevision(ctx, "sub")
	_, err = s.ActivateSubscription(ctx, EventSubscription{ID: "sub", Principal: "owner", Profile: domain.ConversationMessageCreatedV2, Scope: "all", Callback: "https://receiver.example", Secret: "synthetic"}, r, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AckSubscriptionEvents(ctx, "owner", "sub", receipt); err == nil {
		t.Fatal("old generation receipt accepted")
	}
}

func TestJournalRetentionNeverSkipsEarlierRetainedRecord(t *testing.T) {
	// Arrange: a later terminal row expired before an earlier retained callback.
	ctx := context.Background()
	s, _, _, at := journalFixture(t)
	defer s.Close()
	finishJournal(t, s, at)
	if _, err := s.DB.Exec(`UPDATE event_deliveries SET completed_at=? WHERE id=(SELECT max(id) FROM event_deliveries)`, at.Add(-8*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	// Act.
	if err := s.PruneDeliveries(ctx, at); err != nil {
		t.Fatal(err)
	}
	// Assert: no watermark may mask a retained unprocessed prefix.
	records := journalRecords(t, s, 20)
	if len(records) != 3 || records[0]["delivery_state"] != "delivered" || records[0]["event"].(map[string]any)["text"] != "one" {
		t.Fatal(records)
	}
}

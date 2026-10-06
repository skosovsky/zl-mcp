package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func queueEncoder(id string, m domain.Message) ([]byte, error) {
	return json.Marshal(map[string]string{"eventId": id, "text": m.Text})
}

func TestBroadConversationFanoutCapacityPreservesCollectedCorpus(t *testing.T) {
	// Arrange: overlapping all/direct scopes across newly discovered dialogues.
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Now().UTC()
	for _, scope := range []string{"all", "direct"} {
		revision, err := s.SubscriptionRevision(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		sub := EventSubscription{ID: scope, Principal: "owner", Profile: domain.ConversationMessageCreatedV2, Scope: scope, Callback: "https://callback.example", Secret: "synthetic"}
		if scope == "direct" {
			sub.ConversationType = "direct"
		}
		if _, err := s.ActivateSubscription(ctx, sub, revision, at); err != nil {
			t.Fatal(err)
		}
	}
	for n := range 10 {
		for _, kind := range []string{"direct", "group"} {
			if err := s.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: fmt.Sprint(n)}, ID: "m", SenderID: "peer", SentAt: at, Text: "synthetic", Source: "live"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	p := DefaultDeliveryPolicy()
	p.MaxJobs = 3
	// Act: twenty journal records fan out to thirty targets under one global quota.
	n, err := s.FanoutProfileEvents(ctx, at, p, func(_ string, id string, m domain.Message) ([]byte, error) { return queueEncoder(id, m) })
	if err != nil || n != 20 {
		t.Fatalf("fanout=%d error=%v", n, err)
	}
	// Assert: every excess delivery is diagnosed; collection and watermark survive.
	var pending, failed, messages, journal int
	for query, target := range map[string]*int{
		"SELECT COUNT(*) FROM event_deliveries WHERE state='pending'":                                                       &pending,
		"SELECT COUNT(*) FROM event_deliveries WHERE state='failed' AND last_reason='queue_capacity' AND length(payload)=0": &failed,
		"SELECT COUNT(*) FROM messages":       &messages,
		"SELECT COUNT(*) FROM message_events": &journal,
	} {
		if err := s.DB.QueryRowContext(ctx, query).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if pending != 3 || failed != 27 || messages != 20 || journal != 0 {
		t.Fatalf("pending=%d failed=%d messages=%d journal=%d", pending, failed, messages, journal)
	}
}

func TestConversationDeliveryRevocationAfterRestart(t *testing.T) {
	// Arrange: persist broad and exact subscriptions while both types are allowed.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s, err := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	at := time.Now().UTC()
	for _, sub := range []EventSubscription{
		{ID: "all", Profile: domain.ConversationMessageCreatedV2, Scope: "all"},
		{ID: "direct", Profile: domain.ConversationMessageCreatedV2, Scope: "direct", ConversationType: "direct"},
		{ID: "exact", Profile: domain.ConversationMessageCreatedV2, Scope: "conversation", ConversationType: "direct", GroupID: "same"},
	} {
		sub.Principal, sub.Callback, sub.Secret = "owner", "https://callback.example", "synthetic"
		revision, err := s.SubscriptionRevision(ctx, sub.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ActivateSubscription(ctx, sub, revision, at); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"direct", "group"} {
		if err := s.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: "same"}, ID: "m", SenderID: "peer", SentAt: at, Text: "synthetic", Source: "live"}); err != nil {
			t.Fatal(err)
		}
	}
	p := DefaultDeliveryPolicy()
	if _, err := s.FanoutProfileEvents(ctx, at, p, func(_ string, id string, m domain.Message) ([]byte, error) { return queueEncoder(id, m) }); err != nil {
		t.Fatal(err)
	}
	// Act: reopen with only the group permitted, retaining all durable queues.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{{Type: "group", ID: "same"}: true}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimDelivery(ctx, at, p)
	if err != nil || job == nil || job.SubscriptionID != "all" {
		t.Fatalf("permitted job=%v error=%v", job, err)
	}
	if err := s.FinishDelivery(ctx, *job, DeliveryOutcome{State: "delivered"}, at); err != nil {
		t.Fatal(err)
	}
	next, err := s.ClaimDelivery(ctx, at, p)
	// Assert: neither broad nor exact scopes bypass revocation or type isolation.
	if err != nil || next != nil {
		t.Fatalf("revoked job=%v error=%v", next, err)
	}
	var cleared int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_deliveries WHERE state='cancelled' AND last_reason='access_revoked' AND length(payload)=0").Scan(&cleared); err != nil {
		t.Fatal(err)
	}
	if cleared != 3 {
		t.Fatalf("cleared revoked payloads=%d, want 3", cleared)
	}
}

func queueSubscription(t *testing.T, s *Store, id string, at time.Time) EventSubscription {
	t.Helper()
	sub := EventSubscription{ID: id, Principal: "owner", GroupID: "g1", Callback: "https://callback.example", Secret: "synthetic"}
	revision, err := s.SubscriptionRevision(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	sub, err = s.ActivateSubscription(context.Background(), sub, revision, at)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestFanoutFiltersHistoryAndCommitsWatermarkWithTasks(t *testing.T) {
	// Arrange.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	putTest(t, s, "history", "g1", "old", at)
	queueSubscription(t, s, "subscription", at)
	putTest(t, s, "late", "g1", "late replay", at.Add(-time.Hour))
	putTest(t, s, "other", "g2", "not subscribed", at)
	if _, err := s.DB.ExecContext(ctx, `CREATE TRIGGER reject_task BEFORE INSERT ON event_deliveries BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	// Act: interruption during fan-out rolls back earlier watermark updates too.
	_, err := s.FanoutEvents(ctx, at, DefaultDeliveryPolicy(), queueEncoder)
	// Assert.
	if err == nil {
		t.Fatal("fanout failure ignored")
	}
	var watermark, count int
	if err := s.DB.QueryRowContext(ctx, "SELECT seq FROM event_fanout").Scan(&watermark); err != nil {
		t.Fatal(err)
	}
	if watermark != 0 {
		t.Fatal("failed transaction advanced watermark")
	}
	// Arrange.
	if _, err := s.DB.ExecContext(ctx, "DROP TRIGGER reject_task"); err != nil {
		t.Fatal(err)
	}
	// Act.
	n, err := s.FanoutEvents(ctx, at, DefaultDeliveryPolicy(), queueEncoder)
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := s.FanoutEvents(ctx, at, DefaultDeliveryPolicy(), queueEncoder)
	// Assert: no history, other group or duplicate task.
	if err != nil || n != 3 || repeat != 0 {
		t.Fatalf("fanout n=%d repeat=%d err=%v", n, repeat, err)
	}
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM event_deliveries").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("incorrect delivery set: %d", count)
	}
}

func TestDeliveryLeaseSurvivesRestartAndRejectsStaleCompletion(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s, err := Open(ctx, path, []string{"g1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	at := time.Now().UTC()
	p := DefaultDeliveryPolicy()
	queueSubscription(t, s, "sub", at)
	putTest(t, s, "message", "g1", "body", at)
	if _, err := s.FanoutEvents(ctx, at, p, queueEncoder); err != nil {
		t.Fatal(err)
	}
	// Act.
	first, err := s.ClaimDelivery(ctx, at, p)
	if err != nil || first == nil {
		t.Fatalf("claim: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path, []string{"g1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	busy, err := s.ClaimDelivery(ctx, at.Add(time.Second), p)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.ClaimDelivery(ctx, at.Add(p.Lease+time.Second), p)
	// Assert: no parallel callback, then same persisted payload and event identity.
	if err != nil || busy != nil || retry == nil || retry.EventID != first.EventID || string(retry.Payload) != string(first.Payload) || retry.Attempts != 2 {
		t.Fatalf("lease/retry mismatch: %v", err)
	}
	// Act: a response from the first request cannot complete the reclaimed lease.
	if err := s.FinishDelivery(ctx, *first, DeliveryOutcome{State: "delivered"}, at); err != nil {
		t.Fatal(err)
	}
	// Assert.
	var state string
	if err := s.DB.QueryRowContext(ctx, "SELECT state FROM event_deliveries").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "sending" {
		t.Fatal("stale response completed current lease")
	}
	// Act.
	if err := s.FinishDelivery(ctx, *retry, DeliveryOutcome{State: "delivered"}, at); err != nil {
		t.Fatal(err)
	}
	// Assert.
	if err := s.DB.QueryRowContext(ctx, "SELECT state FROM event_deliveries").Scan(&state); err != nil || state != "delivered" {
		t.Fatal("current lease did not complete")
	}
}

func TestQueueCapacityIsVisibleAndDoesNotStopCollection(t *testing.T) {
	// Arrange.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	queueSubscription(t, s, "sub", at)
	putTest(t, s, "one", "g1", "first", at)
	putTest(t, s, "two", "g1", "second", at)
	p := DefaultDeliveryPolicy()
	p.MaxJobs = 1
	// Act.
	_, err := s.FanoutEvents(ctx, at, p, queueEncoder)
	if err != nil {
		t.Fatal(err)
	}
	putTest(t, s, "three", "g1", "collection continues", at)
	// Assert.
	var pending, failed int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM event_deliveries WHERE state='pending'").Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM event_deliveries WHERE state='failed' AND last_reason='queue_capacity'").Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if pending != 1 || failed != 1 {
		t.Fatal("queue overflow not bounded and recorded")
	}
}

func TestRemovingMessageAfterFanoutPurgesPayload(t *testing.T) {
	// Arrange.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	queueSubscription(t, s, "sub", at)
	putTest(t, s, "message", "g1", "private text", at)
	if _, err := s.FanoutEvents(ctx, at, DefaultDeliveryPolicy(), queueEncoder); err != nil {
		t.Fatal(err)
	}
	// Act: journal snapshots were already consumed by fan-out.
	if err := s.Delete(ctx, "g1", "message"); err != nil {
		t.Fatal(err)
	}
	// Assert: message_seq still binds queue to the removed record.
	var body []byte
	var state string
	if err := s.DB.QueryRowContext(ctx, "SELECT payload,state FROM event_deliveries").Scan(&body, &state); err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 || state != "cancelled" {
		t.Fatal("queue retained removed text")
	}
}

func TestCancelledGenerationCannotBeResurrectedByResponse(t *testing.T) {
	// Arrange.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	queueSubscription(t, s, "sub", at)
	putTest(t, s, "message", "g1", "body", at)
	p := DefaultDeliveryPolicy()
	if _, err := s.FanoutEvents(ctx, at, p, queueEncoder); err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimDelivery(ctx, at, p)
	if err != nil || d == nil {
		t.Fatalf("claim: %v", err)
	}
	if err := s.CancelSubscription(ctx, "sub", "owner", at); err != nil {
		t.Fatal(err)
	}
	queueSubscription(t, s, "sub", at)
	// Act.
	if err := s.FinishDelivery(ctx, *d, DeliveryOutcome{State: "pending", RetryAt: at, Gone: true}, at); err != nil {
		t.Fatal(err)
	}
	// Assert: late 410 from old generation cannot revoke the new activation.
	var active int
	if err := s.DB.QueryRowContext(ctx, "SELECT active FROM event_subscriptions WHERE id='sub'").Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatal("stale response revoked new subscription")
	}
}

func TestRetryPreservesSubscriptionOrderWithoutBlockingOtherSubscriptions(t *testing.T) {
	// Arrange
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	p := DefaultDeliveryPolicy()
	queueSubscription(t, s, "a", at)
	queueSubscription(t, s, "b", at)
	putTest(t, s, "first", "g1", "one", at)
	putTest(t, s, "second", "g1", "two", at.Add(time.Second))
	if _, err := s.FanoutEvents(ctx, at, p, queueEncoder); err != nil {
		t.Fatal(err)
	}
	first, err := s.ClaimDelivery(ctx, at, p)
	if err != nil || first == nil {
		t.Fatal("first job unavailable")
	}
	if err = s.FinishDelivery(ctx, *first, DeliveryOutcome{State: "pending", Reason: "network", RetryAt: at.Add(time.Minute)}, at); err != nil {
		t.Fatal(err)
	}
	// Act: the other subscriber advances, but the retrying subscriber's second job cannot overtake its first.
	other, err := s.ClaimDelivery(ctx, at, p)
	if err != nil || other == nil || other.SubscriptionID == first.SubscriptionID {
		t.Fatal("independent subscriber blocked")
	}
	if err = s.FinishDelivery(ctx, *other, DeliveryOutcome{State: "delivered"}, at); err != nil {
		t.Fatal(err)
	}
	otherNext, err := s.ClaimDelivery(ctx, at, p)
	if err != nil || otherNext == nil || otherNext.SubscriptionID == first.SubscriptionID {
		t.Fatal("second independent job blocked")
	}
	if err = s.FinishDelivery(ctx, *otherNext, DeliveryOutcome{State: "delivered"}, at); err != nil {
		t.Fatal(err)
	}
	blocked, err := s.ClaimDelivery(ctx, at, p)
	// Assert
	if err != nil || blocked != nil {
		t.Fatal("later job overtook a pending retry")
	}
	retry, err := s.ClaimDelivery(ctx, at.Add(time.Minute), p)
	if err != nil || retry == nil || retry.EventID != first.EventID {
		t.Fatal("retry did not preserve first identity")
	}
}

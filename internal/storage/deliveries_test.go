package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func queueEncoder(id string, m domain.Message) ([]byte, error) {
	return json.Marshal(map[string]string{"eventId": id, "text": m.Text})
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

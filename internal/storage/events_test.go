package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestMessageAndEventAreAtomicAndReplayIsDeduplicated(t *testing.T) {
	// Arrange.
	s := openTest(t)
	ctx := context.Background()
	putTest(t, s, "first", "g1", "original", time.Now())
	// Act.
	putTest(t, s, "first", "g1", "duplicate", time.Now())
	var messages, events int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM messages").Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM message_events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	// Assert.
	if messages != 1 || events != 1 {
		t.Fatalf("messages=%d events=%d", messages, events)
	}

	// Arrange: a storage failure occurs precisely between message and event writes.
	if _, err := s.DB.ExecContext(ctx, `CREATE TRIGGER reject_event BEFORE INSERT ON message_events BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	// Act.
	err := s.Put(ctx, testEventMessage("failed", time.Now()))
	// Assert: neither message nor FTS entry escapes the rolled-back transaction.
	if err == nil {
		t.Fatal("event failure ignored")
	}
	if _, err := s.Message(ctx, "g1", "failed"); err == nil {
		t.Fatal("message survived failed event transaction")
	}
	hits, _, _, err := s.Search(ctx, Search{Query: "failed"})
	if err != nil || len(hits) != 0 {
		t.Fatal("FTS survived rollback")
	}
}

func TestSubscriptionBoundarySurvivesRefreshRestartAndCancellation(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s, err := Open(ctx, path, []string{"g1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	putTest(t, s, "old", "g1", "old corpus", time.Now())
	sub := EventSubscription{ID: "id", Principal: "owner", GroupID: "g1", Callback: "https://callback.example", Secret: "synthetic-secret"}
	rev, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	active, err := s.ActivateSubscription(ctx, sub, rev, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	putTest(t, s, "late", "g1", "late replay", time.Now().Add(-time.Hour))
	refreshed, err := s.ActivateSubscription(ctx, sub, rev, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Assert: sent_at does not move the insertion boundary; refresh retains it.
	var lateSeq int64
	if err := s.DB.QueryRowContext(ctx, "SELECT seq FROM messages WHERE message_id='late'").Scan(&lateSeq); err != nil {
		t.Fatal(err)
	}
	if active.StartSeq >= lateSeq || refreshed.StartSeq != active.StartSeq || refreshed.Generation != active.Generation {
		t.Fatal("activation boundary shifted on replay or refresh")
	}

	// Arrange / Act: close and reopen the real SQLite database.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path, []string{"g1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := s.ActivateSubscription(ctx, sub, rev, time.Now())
	// Assert.
	if err != nil || restored.Generation != active.Generation || restored.StartSeq != active.StartSeq {
		t.Fatal("restart reset subscription")
	}

	// Act: cancellation invalidates a verification begun with the old revision.
	if err := s.CancelSubscription(ctx, sub.ID, sub.Principal, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err = s.ActivateSubscription(ctx, sub, rev, time.Now())
	// Assert.
	if !errors.Is(err, ErrSubscriptionCancelled) {
		t.Fatalf("pending verification reactivated cancellation: %v", err)
	}

	// Arrange: a genuinely new subscribe captures the new revision.
	rev, err = s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	newSub, err := s.ActivateSubscription(ctx, sub, rev, time.Now())
	// Assert: new activation excludes messages accepted during the old generation.
	if err != nil || newSub.Generation == active.Generation || newSub.StartSeq < lateSeq {
		t.Fatal("new subscription reused old boundary or generation")
	}
}

func TestSubscriptionBoundaryDoesNotResetWhenCorpusIsDeleted(t *testing.T) {
	// Arrange.
	s := openTest(t)
	ctx := context.Background()
	putTest(t, s, "old", "g1", "old", time.Now())
	if err := s.Delete(ctx, "g1", "old"); err != nil {
		t.Fatal(err)
	}
	sub := EventSubscription{ID: "id", Principal: "owner", GroupID: "g1", Callback: "https://callback.example", Secret: "synthetic-secret"}
	rev, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	active, err := s.ActivateSubscription(ctx, sub, rev, time.Now())
	// Assert: sqlite_sequence, rather than max(visible messages), defines the boundary.
	if err != nil || active.StartSeq < 1 {
		t.Fatalf("deleted corpus reset boundary: %+v %v", active, err)
	}
}

func testEventMessage(id string, at time.Time) domain.Message {
	return domain.Message{GroupID: "g1", ID: id, SenderID: "author", SentAt: at, Text: id, Source: "live"}
}

func TestReplayAfterRetentionDoesNotCreateAnotherEvent(t *testing.T) {
	// Arrange.
	s := openTest(t)
	ctx := context.Background()
	putTest(t, s, "old", "g1", "old text", time.Now().AddDate(0, 0, -100))
	if err := s.Retain(ctx); err != nil {
		t.Fatal(err)
	}
	// Act: the source replays an identity whose text was already removed.
	putTest(t, s, "old", "g1", "replayed text", time.Now())
	// Assert.
	var count int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM message_events WHERE group_id='g1'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("retention erased deduplication identity: events=%d", count)
	}
}

func TestRemovalPurgesEventAndQueuedText(t *testing.T) {
	// Arrange.
	s := openTest(t)
	ctx := context.Background()
	putTest(t, s, "message", "g1", "private body", time.Now())
	_, err := s.DB.ExecContext(ctx, `INSERT INTO event_deliveries(event_id,subscription_id,generation,payload,state,next_attempt_at,deadline) SELECT event_id,'sub','generation',?,'pending',?,? FROM message_events`, []byte("private body"), now(), now())
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	err = s.Delete(ctx, "g1", "message")
	// Assert: pending deliveries and snapshots cannot retain removed text.
	if err != nil {
		t.Fatal(err)
	}
	var state, reason string
	var payload []byte
	if err := s.DB.QueryRowContext(ctx, "SELECT state,last_reason,payload FROM event_deliveries").Scan(&state, &reason, &payload); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM message_events").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if state != "cancelled" || reason != "record_deleted" || len(payload) != 0 || count != 0 {
		t.Fatal("removed text retained by events")
	}
}

func TestFiniteSubscriptionRefreshKeepsBoundaryAndExpiryStartsNewGeneration(t *testing.T) {
	// Arrange.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	revision, err := s.SubscriptionRevision(ctx, "finite")
	if err != nil {
		t.Fatal(err)
	}
	expires := at.Add(time.Minute)
	sub := EventSubscription{ID: "finite", Principal: "owner", GroupID: "g1", Callback: "https://callback.example", Secret: "synthetic", ExpiresAt: &expires}
	first, err := s.ActivateSubscription(ctx, sub, revision, at)
	if err != nil {
		t.Fatal(err)
	}
	putTest(t, s, "after-first", "g1", "synthetic", at)
	// Act: refresh before expiry, then activate again after the refreshed expiry.
	refreshedExpiry := at.Add(2 * time.Minute)
	sub.ExpiresAt = &refreshedExpiry
	refreshed, err := s.ActivateSubscription(ctx, sub, revision, at.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	newExpiry := at.Add(4 * time.Minute)
	sub.ExpiresAt = &newExpiry
	afterExpiry, err := s.ActivateSubscription(ctx, sub, revision, at.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	// Assert.
	if refreshed.Generation != first.Generation || refreshed.StartSeq != first.StartSeq || !refreshed.ExpiresAt.Equal(refreshedExpiry) {
		t.Fatal("refresh changed boundary or failed to extend expiry")
	}
	if afterExpiry.Generation == first.Generation || afterExpiry.StartSeq <= first.StartSeq {
		t.Fatal("expired subscription reused its old boundary")
	}
}

func TestActiveSubscriptionCapacityAllowsRefreshAndReleasesCancelledSlot(t *testing.T) {
	// Arrange: reach the documented account limit with distinct subscriptions.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	activate := func(id string) (EventSubscription, error) {
		revision, err := s.SubscriptionRevision(ctx, id)
		if err != nil {
			return EventSubscription{}, err
		}
		return s.ActivateSubscription(ctx, EventSubscription{ID: id, Principal: "owner", GroupID: "g1", Callback: "https://callback.example", Secret: "synthetic"}, revision, at)
	}
	var first EventSubscription
	for i := 0; i < 100; i++ {
		sub, err := activate(fmt.Sprintf("sub-%d", i))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = sub
		}
	}
	// Act.
	_, overflowErr := activate("overflow")
	refreshed, refreshErr := activate("sub-0")
	if err := s.CancelSubscription(ctx, "sub-0", "owner", at); err != nil {
		t.Fatal(err)
	}
	_, replacementErr := activate("overflow")
	// Assert: refresh does not consume a slot, and cancellation frees one.
	if overflowErr == nil || refreshErr != nil || replacementErr != nil {
		t.Fatalf("overflow=%v refresh=%v replacement=%v", overflowErr, refreshErr, replacementErr)
	}
	if refreshed.Generation != first.Generation || refreshed.StartSeq != first.StartSeq {
		t.Fatal("refresh changed identity at capacity")
	}
	var active int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM event_subscriptions WHERE active=1").Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 100 {
		t.Fatalf("active=%d", active)
	}
}

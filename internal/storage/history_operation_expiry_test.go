package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func expiringOperation(t *testing.T) (*Store, HistoryOperation, string, ExpiringHistoryRecord) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s := historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	op, err := s.PrepareHistoryOperation(ctx, historyRequest())
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	message := historicalFixture(historyRequest().Ref(), "atomic-expiry")
	message.SentAt = time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	return s, op, path, ExpiringHistoryRecord{Message: message, ExpiresAtMS: time.Now().Add(time.Hour).UnixMilli()}
}
func assertNoExpiringPage(t *testing.T, s *Store, original HistoryOperation) {
	t.Helper()
	for _, table := range []string{"messages", "message_identities", "history_message_expiry", "message_events"} {
		var n int
		if err := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("page escaped rollback", table, n, err)
		}
	}
	op, err := s.HistoryOperation(context.Background(), original.Status.OperationID)
	if err != nil || op.Revision != original.Revision || op.Status.PagesObserved != original.Status.PagesObserved || op.Cursor != original.Cursor {
		t.Fatal("checkpoint changed", err)
	}
}
func TestExpiringOperationPageRollsBackMessagesTTLAndCheckpoint(t *testing.T) {
	// Arrange: an injected durable checkpoint failure after page/TTL inserts.
	s, op, _, record := expiringOperation(t)
	defer s.Close()
	if _, err := s.DB.Exec(`CREATE TRIGGER reject_expiring_checkpoint BEFORE UPDATE ON history_operations BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	more, cursor := true, "123"
	// Act.
	_, err := s.CommitExpiringHistoryOperationPage(context.Background(), op.Status.OperationID, op.Revision, domain.HistoryPage{HasMore: &more, Cursor: &cursor}, []ExpiringHistoryRecord{record}, time.Millisecond)
	// Assert: message, expiry, identity and source progress are all absent.
	if err == nil {
		t.Fatal("failed checkpoint reported success")
	}
	assertNoExpiringPage(t, s, op)
}
func TestExpiringOperationRestartDuplicateAndCancellation(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	s, op, path, record := expiringOperation(t)
	more, cursor := true, "123"
	// Act: exact page commit and restart.
	next, err := s.CommitExpiringHistoryOperationPage(ctx, op.Status.OperationID, op.Revision, domain.HistoryPage{HasMore: &more, Cursor: &cursor}, []ExpiringHistoryRecord{record}, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	defer s.Close()
	loaded, err := s.HistoryOperation(ctx, op.Status.OperationID)
	if err != nil || loaded.Cursor != cursor || loaded.Revision != next.Revision || loaded.Status.InsertedCount != 1 {
		t.Fatal("lost atomic checkpoint", err)
	}
	// Assert: a duplicate on a later source page cannot extend its TTL.
	cursor = "456"
	duplicate := record
	duplicate.ExpiresAtMS += int64(time.Hour / time.Millisecond)
	next, err = s.CommitExpiringHistoryOperationPage(ctx, next.Status.OperationID, next.Revision, domain.HistoryPage{HasMore: &more, Cursor: &cursor}, []ExpiringHistoryRecord{duplicate}, time.Millisecond)
	if err != nil || next.Status.DuplicateCount != 1 {
		t.Fatal(err)
	}
	var deadline int64
	if err = s.DB.QueryRow(`SELECT expires_ms FROM history_message_expiry WHERE message_id=?`, record.Message.ID).Scan(&deadline); err != nil || deadline != record.ExpiresAtMS {
		t.Fatal("duplicate extended expiry", err)
	}
	var events int
	if err = s.DB.QueryRow(`SELECT count(*) FROM message_events`).Scan(&events); err != nil || events != 0 {
		t.Fatal("silent import created event", err)
	}
	// Act / Assert: stale/cancelled work cannot write another page.
	_, err = s.CancelHistoryOperation(ctx, next.Status.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	record.Message.ID = "after-cancel"
	cursor = "789"
	if _, err = s.CommitExpiringHistoryOperationPage(ctx, next.Status.OperationID, next.Revision, domain.HistoryPage{HasMore: &more, Cursor: &cursor}, []ExpiringHistoryRecord{record}, time.Millisecond); !errors.Is(err, ErrHistoryState) {
		t.Fatal("cancelled worker persisted", err)
	}
	var n int
	if err = s.DB.QueryRow(`SELECT count(*) FROM messages WHERE message_id='after-cancel'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("cancelled page escaped", err)
	}
}
func TestExpiringOperationRejectsAmbiguousAndInvalidOutOfIntervalEvidence(t *testing.T) {
	// Arrange.
	s, op, _, good := expiringOperation(t)
	defer s.Close()
	more, cursor := true, "123"
	bad := good
	bad.Message.ID = "outside"
	bad.Message.SentAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	bad.ExpiresAtMS = -1
	// Act / Assert: whole-page validation cannot ignore bad out-of-range TTL.
	_, err := s.CommitExpiringHistoryOperationPage(context.Background(), op.Status.OperationID, op.Revision, domain.HistoryPage{HasMore: &more, Cursor: &cursor}, []ExpiringHistoryRecord{good, bad}, time.Millisecond)
	if err == nil {
		t.Fatal("invalid out-of-range evidence accepted")
	}
	assertNoExpiringPage(t, s, op)
	_, err = s.CommitExpiringHistoryOperationPage(context.Background(), op.Status.OperationID, op.Revision, domain.HistoryPage{Messages: []domain.Message{good.Message}, HasMore: &more, Cursor: &cursor}, []ExpiringHistoryRecord{good}, time.Millisecond)
	if err == nil {
		t.Fatal("ambiguous dual input accepted")
	}
	assertNoExpiringPage(t, s, op)
}
func TestExpiringOperationAppliesDueDeadlineInsidePageTransaction(t *testing.T) {
	// Arrange: positive original TTL already due, source timestamp predates it.
	s, op, _, record := expiringOperation(t)
	defer s.Close()
	record.Message.SentAt = time.Now().Add(-2 * time.Hour)
	record.ExpiresAtMS = time.Now().Add(-time.Hour).UnixMilli()
	more := false
	// Act.
	next, err := s.CommitExpiringHistoryOperationPage(context.Background(), op.Status.OperationID, op.Revision, domain.HistoryPage{HasMore: &more}, []ExpiringHistoryRecord{record}, time.Millisecond)
	// Assert: source progress persists, while an expired message is irreversibly hidden.
	if err != nil || next.Status.PagesObserved != 1 || next.Status.RecordsObserved != 1 {
		t.Fatal(err)
	}
	var expired, n int
	if err = s.DB.QueryRow(`SELECT expired FROM history_message_expiry WHERE message_id=?`, record.Message.ID).Scan(&expired); err != nil || expired != 1 {
		t.Fatal("expiry not atomically applied", err)
	}
	if err = s.DB.QueryRow(`SELECT count(*) FROM visible_messages WHERE message_id=?`, record.Message.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal("expired record visible", err)
	}
}

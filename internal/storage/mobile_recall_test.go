package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func mobileRecallPage(t *testing.T) (*Store, HistoryOperation, string, domain.MobileHistoryPage) {
	t.Helper()
	s, op, path, page := mobileOperationRequest(t, func(r *domain.HistoryImportRequest) { r.PageSize = 5 })
	page.Snapshot.ControlRows = 1
	page.Snapshot.SourceRows = 5
	page.Snapshot.PeriodRows = 5
	page.Counts.Examined = 4
	page.Counts.OwnRecalls = 1
	m := &page.Records[0].Message
	m.SenderID = "synthetic-account"
	m.Direction = "outgoing"
	page.Recalls = []domain.MobileHistoryRecall{{Conversation: m.Ref(), MessageID: m.ID, SenderID: "synthetic-account", RecordAtMS: m.SentAt.UnixMilli()}}
	return s, op, path, page
}

func TestMobileRecallAndCheckpointCommitAtomicallyWithoutEvents(t *testing.T) {
	// Arrange: a same-page old message and exact own recall, with one later row.
	s, op, path, page := mobileRecallPage(t)
	ctx := context.Background()
	// Act: recalls must suppress same-page records before checkpoint publication.
	next, err := s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, time.Millisecond)
	// Assert: a committed tombstone, no restored message/event, exact control proof.
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	if next.Status.InsertedCount != 0 || next.Status.MobileCoverage.OwnRecalls != 1 || next.Status.MobileCoverage.SourceControls != 1 {
		s.Close()
		t.Fatal("recall progress mismatch")
	}
	var count int
	if err = s.DB.QueryRow("SELECT count(*) FROM message_tombstones WHERE conversation_type='direct' AND conversation_id='12' AND message_id='13'").Scan(&count); err != nil || count != 1 {
		s.Close()
		t.Fatal("tombstone missing")
	}
	for _, table := range []string{"messages", "message_events", "event_deliveries"} {
		if err = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			s.Close()
			t.Fatal("recall produced records/events", table)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	defer s.Close()
	checkpoint, err := s.MobileHistoryCheckpoint(ctx, op.Status.OperationID)
	if err != nil || checkpoint.Snapshot != page.Snapshot {
		t.Fatal("recall proof lost on restart", err)
	}
	// A later historical replay of the recalled record stays suppressed.
	final := domain.MobileHistoryPage{Snapshot: page.Snapshot, Records: page.Records, Counts: domain.MobileHistoryCounts{Examined: 1}}
	done, err := s.CommitMobileHistoryPage(ctx, next.Status.OperationID, next.Revision, final, time.Millisecond)
	if err != nil || done.Status.InsertedCount != 0 || done.Status.MobileCoverage.OwnRecalls != 1 {
		t.Fatal("later page restored recalled message", err)
	}
	if _, err = s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, time.Millisecond); !errors.Is(err, ErrHistoryState) {
		t.Fatal("stale page accepted")
	}
	assertHistoryStatusSchema(t, done)
}

func TestMobileRecallRollbackPreservesCorpusAndProgress(t *testing.T) {
	for _, point := range []string{"source", "operation"} {
		t.Run(point, func(t *testing.T) {
			// Arrange: inject failure after tombstone/removal but before durable checkpoint.
			s, op, _, page := mobileRecallPage(t)
			defer s.Close()
			trigger := `CREATE TRIGGER fail_recall BEFORE INSERT ON history_mobile_sources BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`
			if point == "operation" {
				trigger = `CREATE TRIGGER fail_recall BEFORE UPDATE ON history_operations BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`
			}
			if _, err := s.DB.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			// Act.
			_, err := s.CommitMobileHistoryPage(context.Background(), op.Status.OperationID, op.Revision, page, time.Millisecond)
			// Assert: no tombstone or checkpoint prefix escapes.
			if err == nil {
				t.Fatal("checkpoint failure accepted")
			}
			assertMobileEmpty(t, s, op)
			var n int
			if err = s.DB.QueryRow("SELECT count(*) FROM message_tombstones").Scan(&n); err != nil || n != 0 {
				t.Fatal("tombstone escaped rollback")
			}
		})
	}
}

func TestMobileRecallRejectsIncompleteControlProofAndForeignTargets(t *testing.T) {
	for _, mode := range []string{"missing", "count", "foreign-sender", "foreign-peer", "group", "duplicate", "out-of-period", "wal"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: untrusted/incomplete source controls must precede all record writes.
			s, op, _, page := mobileRecallPage(t)
			defer s.Close()
			switch mode {
			case "missing":
				page.Recalls = nil
				page.Counts.OwnRecalls = 0
				page.Counts.DeferredControls = 1
			case "count":
				page.Snapshot.ControlRows = 2
			case "foreign-sender":
				page.Recalls[0].SenderID = "12"
			case "foreign-peer":
				page.Recalls[0].Conversation.ID = "99"
			case "group":
				page.Recalls[0].Conversation.Type = "group"
			case "duplicate":
				page.Recalls = append(page.Recalls, page.Recalls[0])
			case "out-of-period":
				page.Recalls[0].RecordAtMS = 0
			case "wal":
				page.Snapshot.WALMode = true
			}
			// Act.
			_, err := s.CommitMobileHistoryPage(context.Background(), op.Status.OperationID, op.Revision, page, time.Millisecond)
			// Assert.
			if err == nil {
				t.Fatal("invalid recall accepted")
			}
			assertMobileEmpty(t, s, op)
			var n int
			if err = s.DB.QueryRow("SELECT count(*) FROM message_tombstones").Scan(&n); err != nil || n != 0 {
				t.Fatal("invalid recall left tombstone")
			}
		})
	}
}

func TestMobileRecallRollbackRestoresExistingMessage(t *testing.T) {
	// Arrange: the original exists before the recall transaction.
	s, op, _, page := mobileRecallPage(t)
	defer s.Close()
	ctx := context.Background()
	original := page.Records[0].Message
	if _, err := s.PutHistoryPage(ctx, original.Ref(), []domain.Message{original}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_existing_recall BEFORE UPDATE ON history_operations BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	// Act: fail after message removal, before checkpoint publication.
	if _, err := s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, 0); err == nil {
		t.Fatal("checkpoint failure accepted")
	}
	// Assert: original content and operation revision survive; no tombstone exists.
	retained, err := s.ConversationMessage(ctx, original.Ref(), original.ID)
	if err != nil || retained.Text != original.Text {
		t.Fatal("original lost during rollback", err)
	}
	current, err := loadHistoryOperation(ctx, s.DB, op.Status.OperationID)
	if err != nil || current.Revision != op.Revision || current.Status.RecordsObserved != 0 {
		t.Fatal("checkpoint escaped rollback", err)
	}
	var n int
	for _, table := range []string{"message_tombstones", "history_mobile_sources", "message_events", "event_deliveries"} {
		if err := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("rollback side effect", table, n, err)
		}
	}
}

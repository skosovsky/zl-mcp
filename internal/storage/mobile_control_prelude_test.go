package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func controlPreludeFixture(t *testing.T) (*Store, HistoryOperation, domain.MobileHistoryPage, domain.MobileHistoryPage) {
	t.Helper()
	s, op, _, ordinary := mobileOperation(t)
	source := ordinary.Snapshot
	source.SourceRows++
	source.ControlRows = 1
	source.ControlPreludeComplete = true
	source.EarliestMS = ordinary.Records[0].Message.SentAt.Add(-24 * time.Hour).UnixMilli()
	ordinary.Snapshot = source
	prelude := domain.MobileHistoryPage{Snapshot: source, ControlPrelude: true, HasMore: true, Recalls: []domain.MobileHistoryRecall{{Conversation: op.Status.Ref(), MessageID: ordinary.Records[0].Message.ID, SenderID: "synthetic-account", RecordAtMS: source.EarliestMS}}}
	return s, op, prelude, ordinary
}

func TestMobileControlPreludeCommitsOutsidePeriodBeforeOrdinaryPages(t *testing.T) {
	// Arrange: a whole-source recall precedes the requested ordinary-message interval.
	s, op, prelude, ordinary := controlPreludeFixture(t)
	defer s.Close()
	ctx := context.Background()
	// Act.
	next, err := s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, prelude, 0)
	// Assert: tombstone/proof precede messages and consume no ordinary counts.
	if err != nil || next.Status.PagesObserved != 0 || next.Status.RecordsObserved != 0 || next.Status.MobileCoverage.SourceRecalls != 1 || !next.Status.MobileCoverage.ControlPreludeComplete {
		t.Fatal("prelude not atomically committed", err)
	}
	checkpoint, err := s.MobileHistoryCheckpoint(ctx, op.Status.OperationID)
	if err != nil || checkpoint.Next != nil || checkpoint.Snapshot != prelude.Snapshot {
		t.Fatal("prelude checkpoint lost", err)
	}
	if _, err = s.CommitMobileHistoryPage(ctx, op.Status.OperationID, next.Revision, prelude, 0); err == nil {
		t.Fatal("prelude repeated")
	}
	final, err := s.CommitMobileHistoryPage(ctx, next.Status.OperationID, next.Revision, ordinary, 0)
	if err != nil || final.Status.InsertedCount != 0 || final.Status.MobileCoverage.SourceRecalls != 1 || final.Status.MobileCoverage.OwnRecalls != 0 {
		t.Fatal("ordinary page restored recalled target", err)
	}
	for _, table := range []string{"messages", "message_events", "event_deliveries"} {
		var n int
		if err = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("prelude produced ordinary effects", table, n, err)
		}
	}
	assertHistoryStatusSchema(t, final)
}

func TestMobileControlPreludeRollbackAndInvalidInputsLeaveNoPrefix(t *testing.T) {
	for _, mode := range []string{"source-failure", "operation-failure", "counts", "ordinary-record", "cursor", "missing-proof", "missing-target", "duplicate", "foreign-sender", "outside-source", "wal"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			s, op, page, ordinary := controlPreludeFixture(t)
			defer s.Close()
			query := ""
			switch mode {
			case "source-failure":
				query = `CREATE TRIGGER fail_prelude BEFORE INSERT ON history_mobile_sources BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`
			case "operation-failure":
				query = `CREATE TRIGGER fail_prelude BEFORE UPDATE ON history_operations BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`
			case "counts":
				page.Counts.Examined = 1
			case "ordinary-record":
				page.Records = ordinary.Records
			case "cursor":
				page.Next = &domain.MobileHistoryPosition{TimestampMS: page.Snapshot.EarliestMS, RowID: 1}
			case "missing-proof":
				page.Snapshot.ControlPreludeComplete = false
			case "missing-target":
				page.Recalls = nil
			case "duplicate":
				page.Recalls = append(page.Recalls, page.Recalls[0])
				page.Snapshot.ControlRows = 2
			case "foreign-sender":
				page.Recalls[0].SenderID = "12"
			case "outside-source":
				page.Recalls[0].RecordAtMS = page.Snapshot.EarliestMS - 1
			case "wal":
				page.Snapshot.WALMode = true
			}
			if query != "" {
				if _, err := s.DB.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			// Act.
			_, err := s.CommitMobileHistoryPage(context.Background(), op.Status.OperationID, op.Revision, page, 0)
			// Assert.
			if err == nil {
				t.Fatal("invalid prelude accepted")
			}
			assertMobileEmpty(t, s, op)
			var n int
			if err = s.DB.QueryRow("SELECT count(*) FROM message_tombstones").Scan(&n); err != nil || n != 0 {
				t.Fatal("tombstone escaped rollback", err)
			}
		})
	}
}

func TestMobileOrdinaryPageCannotForgePreludeEvidence(t *testing.T) {
	// Arrange: source claims a prelude that the journal never accepted.
	s, op, _, ordinary := controlPreludeFixture(t)
	defer s.Close()
	// Act.
	_, err := s.CommitMobileHistoryPage(context.Background(), op.Status.OperationID, op.Revision, ordinary, 0)
	// Assert.
	if !errors.Is(err, ErrMobileHistorySource) {
		t.Fatal("uncommitted prelude trusted", err)
	}
	assertMobileEmpty(t, s, op)
}

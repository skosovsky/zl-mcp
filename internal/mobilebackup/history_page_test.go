package mobilebackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func historyPageFixture(t *testing.T) (SelectedArchive, domain.MobileBackupRequest) {
	t.Helper()
	r, err := selectedFetchRequest().Normalize()
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC).UnixMilli()
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for i := 1; i <= 5; i++ {
			sender, ttl, kind := "901", int64(0), int64(0)
			if i == 1 {
				sender = "01"
			}
			if i == 2 {
				ttl = 1
			}
			if i == 4 {
				kind = 999
			}
			if i == 5 {
				ttl = time.Now().Add(time.Hour).UnixMilli() - stamp
			}
			metadata := []byte{0, 0, 0, 9, 0, 0, 0, 1, 'x'}
			_, e := db.Exec("INSERT INTO ChatContent(rowid,SenderId,GlbMsgId,CliMsgId,MsgContent,TimeStamp,TTL,MsgType,MsgStatus,BinNet) VALUES(?,?,?,?,?,?,?,?,?,?)", int64(9007199254740992)+int64(i), sender, fmt.Sprint(i), fmt.Sprint(i), "synthetic history", stamp, ttl, kind, 1, metadata)
			if e != nil {
				t.Fatal(e)
			}
		}
	})
	return SelectedArchive{requestID: r.RequestID, requestFingerprint: r.Fingerprint(), ref: r.Ref(), File: ArchiveFile{Name: "902.db", Data: data}}, r
}

func historyPageMapper() domain.MobileIdentitySource {
	return identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		return []domain.MobileIdentityPair{{Plain: "901", Session: "12"}}, nil
	})
}

func TestSnapshotReaderConverterJournalResumeTogether(t *testing.T) {
	// Arrange: one selected image, with a rejected/expired-only first page.
	ctx := context.Background()
	selected, r := historyPageFixture(t)
	defer selected.Clear()
	dir := filepath.Join(t.TempDir(), "snapshots")
	snapshot := openSnapshotStore(t, dir, 1<<20)
	if err := snapshot.Save(ctx, selected, r, "10"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	open := func() *storage.Store {
		s, err := storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.BindAccount(ctx, "10"); err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open()
	defer func() { s.Close() }()
	op, err := s.PrepareMobileHistoryOperation(ctx, domain.HistoryImportRequest{Source: domain.HistorySourceMobileArchive, RequestID: r.RequestID, ConversationType: r.ConversationType, ConversationID: r.ConversationID, Since: r.Since, Until: r.Until, MaxMessages: r.MaxMessages, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	page, err := snapshot.ReadHistoryPage(ctx, r, "10", t.TempDir(), 2, 0, nil, nil, historyPageMapper())
	if err != nil || len(page.Records) != 0 || page.Counts.Examined != 2 || page.Counts.Rejected != 1 || page.Counts.Expired != 1 || page.Next == nil || page.Next.RowID != 9007199254740994 {
		t.Fatal("gap-only reader page did not advance", err)
	}
	firstSource := page.Snapshot
	// Act: commit first page, restart both stores and resume exact durable progress.
	op, err = s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, 0)
	page.Clear()
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	snapshot.Close()
	s = open()
	snapshot = openSnapshotStore(t, dir, 1<<20)
	if err = s.RecoverInterruptedHistory(ctx); err != nil {
		t.Fatal(err)
	}
	op, err = s.HistoryOperation(ctx, op.Status.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	var expectedExpiry int64
	for op.Status.State == "running" {
		checkpoint, e := s.MobileHistoryCheckpoint(ctx, op.Status.OperationID)
		if e != nil || checkpoint.Snapshot != firstSource {
			t.Fatal("restart replaced source or renewed expiry", e)
		}
		page, e = snapshot.ReadHistoryPage(ctx, r, "10", t.TempDir(), 2, op.Status.RecordsObserved, &checkpoint.Snapshot, checkpoint.Next, historyPageMapper())
		if e != nil {
			t.Fatal(e)
		}
		for _, record := range page.Records {
			if record.Message.ID == "5" {
				expectedExpiry = record.Message.SentAt.UnixMilli() + int64(record.Message.QuoteMetadata.TTL)
				if record.ExpiresAtMS != expectedExpiry {
					t.Fatal("reader renewed original TTL")
				}
			}
		}
		op, e = s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, 0)
		page.Clear()
		if e != nil {
			t.Fatal(e)
		}
	}
	// Assert: raw-row coverage, two retained messages and no historical notification.
	if op.Status.State != "partial" || op.Status.StopReason == nil || *op.Status.StopReason != "source_gaps" || op.Status.RecordsObserved != 5 || op.Status.PagesObserved != 3 || op.Status.InsertedCount != 2 || op.Status.MobileCoverage == nil || op.Status.MobileCoverage.Expired != 1 || op.Status.MobileCoverage.UnsupportedTypes != 1 || op.Status.MobileCoverage.UnknownMetadataFields != 3 || op.Status.HistoryComplete {
		t.Fatal("integrated coverage differs from source")
	}
	for table, want := range map[string]int{"messages": 2, "message_identities": 2, "message_events": 0, "event_deliveries": 0} {
		var n int
		if err = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != want {
			t.Fatal("integrated history side effect", table, n, err)
		}
	}
	var storedExpiry int64
	if err = s.DB.QueryRow("SELECT expires_ms FROM history_message_expiry WHERE message_id='5'").Scan(&storedExpiry); err != nil || expectedExpiry == 0 || storedExpiry != expectedExpiry {
		t.Fatal("journal changed original TTL", err)
	}
}

func TestSnapshotHistoryPageRejectsBindingAndUnsafeSources(t *testing.T) {
	for _, mode := range []string{"account", "digest", "expiry", "coverage", "missing-source", "missing-position", "zero-progress", "limit", "wal", "controls", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: authenticated source and a valid first-page checkpoint.
			selected, r := historyPageFixture(t)
			defer selected.Clear()
			if mode == "wal" {
				selected.File.Data[18], selected.File.Data[19] = 2, 2
			}
			if mode == "controls" {
				selected.File.Data = sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
					_, e := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", "1", "1", "synthetic", time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC).UnixMilli(), 0, 33, 1, nil)
					if e != nil {
						t.Fatal(e)
					}
				})
			}
			s := openSnapshotStore(t, filepath.Join(t.TempDir(), "snapshots"), 1<<20)
			ctx := context.Background()
			if err := s.Save(ctx, selected, r, "10"); err != nil {
				t.Fatal(err)
			}
			var previous *domain.MobileHistorySnapshot
			var after *domain.MobileHistoryPosition
			examined, account := 0, "10"
			if mode != "wal" && mode != "controls" {
				first, err := s.ReadHistoryPage(ctx, r, account, t.TempDir(), 2, 0, nil, nil, historyPageMapper())
				if err != nil {
					t.Fatal(err)
				}
				binding, position := first.Snapshot, *first.Next
				previous, after, examined = &binding, &position, 2
				first.Clear()
			}
			switch mode {
			case "account":
				account = "11"
			case "digest":
				previous.Digest[0] ^= 1
			case "expiry":
				previous.ExpiresMS++
			case "coverage":
				previous.PeriodRows++
			case "missing-source":
				previous = nil
			case "missing-position":
				after = nil
			case "zero-progress":
				examined = 0
			case "limit":
				examined = r.MaxMessages
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			// Act: reject the whole writable result instead of returning a prefix.
			page, err := s.ReadHistoryPage(ctx, r, account, t.TempDir(), 2, examined, previous, after, historyPageMapper())
			// Assert: no record/source/continuation is handed to the journal.
			if err == nil || len(page.Records) != 0 || page.Next != nil || page.Snapshot.ID != "" {
				t.Fatal("unsafe snapshot returned writable page")
			}
			encoded, e := json.Marshal(page)
			if e != nil || string(encoded) != "{}" || fmt.Sprintf("%#v", page) != "mobile history page [redacted]" {
				t.Fatal("failed page exposed private data")
			}
		})
	}
}

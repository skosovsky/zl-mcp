package mobilebackup

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestRecallSnapshotConverterJournalResumeTogether(t *testing.T) {
	// Arrange: a complete own recall in the first source page, with a later message.
	ctx := context.Background()
	r, err := selectedFetchRequest().Normalize()
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC).UnixMilli()
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		if _, e := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", "13", "14", "untrusted recalled body", stamp, 0, 36, 3, nil); e != nil {
			t.Fatal(e)
		}
		if _, e := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "902", "15", "16", "synthetic later message", stamp+1, 0, 0, 1, []byte{0, 0, 0, 9, 0, 0, 0, 1, 'x'}); e != nil {
			t.Fatal(e)
		}
	})
	selected := SelectedArchive{requestID: r.RequestID, requestFingerprint: r.Fingerprint(), ref: r.Ref(), File: ArchiveFile{Name: "902.db", Data: data}}
	defer selected.Clear()
	dir := filepath.Join(t.TempDir(), "snapshots")
	snapshot := openSnapshotStore(t, dir, 1<<20)
	if err = snapshot.Save(ctx, selected, r, "10"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	open := func() *storage.Store {
		s, e := storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
		if e != nil {
			t.Fatal(e)
		}
		if e = s.BindAccount(ctx, "10"); e != nil {
			t.Fatal(e)
		}
		return s
	}
	s := open()
	defer func() { s.Close() }()
	old := domain.Message{Conversation: r.Ref(), ID: "13", SenderID: "10", Direction: "outgoing", SentAt: time.UnixMilli(stamp), Text: "synthetic retained original"}
	if _, err = s.PutHistoryPage(ctx, r.Ref(), []domain.Message{old}); err != nil {
		t.Fatal(err)
	}
	op, err := s.PrepareMobileHistoryOperation(ctx, domain.HistoryImportRequest{Source: domain.HistorySourceMobileArchive, RequestID: r.RequestID, ConversationType: r.ConversationType, ConversationID: r.ConversationID, Since: r.Since, Until: r.Until, MaxMessages: r.MaxMessages, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	mapper := identitySourceFunc(func(ctx context.Context, req domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		out := []domain.MobileIdentityPair{}
		for _, plain := range req.Direct {
			session := "12"
			if plain == "901" {
				session = "10"
			}
			out = append(out, domain.MobileIdentityPair{Plain: plain, Session: session})
		}
		return out, nil
	})
	// Act: classify/commit the recall, then restart both stores before the message page.
	page, err := snapshot.ReadHistoryPage(ctx, r, "10", t.TempDir(), 1, 0, nil, nil, mapper)
	if err != nil || len(page.Recalls) != 1 || len(page.Records) != 0 || page.Counts.OwnRecalls != 1 || page.Counts.DeferredControls != 0 {
		t.Fatal("source recall projection failed", err)
	}
	op, err = s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, 0)
	page.Clear()
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	snapshot.Close()
	s = open()
	snapshot = openSnapshotStore(t, dir, 1<<20)
	checkpoint, err := s.MobileHistoryCheckpoint(ctx, op.Status.OperationID)
	if err != nil || checkpoint.Snapshot.ControlRows != 1 {
		t.Fatal("source controls lost on restart", err)
	}
	page, err = snapshot.ReadHistoryPage(ctx, r, "10", t.TempDir(), 1, op.Status.RecordsObserved, &checkpoint.Snapshot, checkpoint.Next, mapper)
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, 0)
	page.Clear()
	if err != nil {
		t.Fatal(err)
	}
	// Assert: recalled record removed, later message retained, no historical events.
	var n int
	for table, want := range map[string]int{"messages": 1, "message_tombstones": 1, "message_events": 0, "event_deliveries": 0} {
		if e := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil || n != want {
			t.Fatal("source side effects", table, n, e)
		}
	}
	if _, err = s.ConversationMessage(ctx, r.Ref(), "13"); err == nil {
		t.Fatal("old recalled message visible")
	}
	if op.Status.MobileCoverage.OwnRecalls != 1 || op.Status.MobileCoverage.SourceControls != 1 || op.Status.RecordsObserved != 2 || op.Status.InsertedCount != 1 {
		t.Fatal("recall coverage mismatch")
	}
	// A later replay cannot restore a recalled ID.
	old.Source = "replay"
	if err = s.Put(ctx, old); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConversationMessage(ctx, r.Ref(), "13"); err == nil {
		t.Fatal("replay restored recalled message")
	}
}

func TestArchiveControlsOutsideFirstPageRemainUnsupported(t *testing.T) {
	// Arrange: one ordinary row precedes a later own recall in the same source.
	ctx := context.Background()
	r, _ := selectedFetchRequest().Normalize()
	stamp := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC).UnixMilli()
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for i, kind := range []int{0, 36} {
			status := 1
			if kind == 36 {
				status = 3
			}
			if _, e := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", 100+i, 200+i, "synthetic", stamp+int64(i), 0, kind, status, []byte{0, 0, 0, 9, 0, 0, 0, 1, 'x'}); e != nil {
				t.Fatal(e)
			}
		}
	})
	selected := SelectedArchive{requestID: r.RequestID, requestFingerprint: r.Fingerprint(), ref: r.Ref(), File: ArchiveFile{Name: "902.db", Data: data}}
	defer selected.Clear()
	snapshots := openSnapshotStore(t, filepath.Join(t.TempDir(), "snapshots"), 1<<20)
	if err := snapshots.Save(ctx, selected, r, "10"); err != nil {
		t.Fatal(err)
	}
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		return []domain.MobileIdentityPair{{Plain: "901", Session: "10"}}, nil
	})
	// Act.
	page, err := snapshots.ReadHistoryPage(ctx, r, "10", t.TempDir(), 1, 0, nil, nil, mapper)
	defer page.Clear()
	// Assert: no ordinary-record prefix before the unresolved later control.
	if !errors.Is(err, ErrSnapshotSourceUnsupported) || len(page.Records) != 0 || len(page.Recalls) != 0 {
		t.Fatal("unseen control permitted record prefix", err)
	}
}

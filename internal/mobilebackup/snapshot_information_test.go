package mobilebackup

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestSnapshotInformationWholeSourceClassificationAndPaging(t *testing.T) {
	// Arrange: the first page contains informational content; ordinary text follows.
	from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	info := attachmentField(6, attachmentField(45, []byte("msginfo.actionlist")))
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for _, row := range []struct {
			kind, stamp int64
			meta        []byte
		}{{20, from.UnixMilli(), info}, {0, from.UnixMilli() + 1, attachmentField(6, attachmentField(999, []byte("opaque")))}, {20, from.Add(-time.Hour).UnixMilli(), info}} {
			if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", "2", "3", "PRIVATE-SYNTHETIC-TEXT", row.stamp, 0, row.kind, 1, row.meta); err != nil {
				t.Fatal(err)
			}
		}
	})
	a := snapshotProjectionFixture(t, from)
	a.archive.Files[0].Data = data
	ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: "12"}
	// Act: read past an empty informational page using its genuine examined-row cursor.
	first, err := a.ReadSnapshotPage(context.Background(), retainedTestID, ref, t.TempDir(), from, from.Add(time.Hour), 1, nil, "asc", from.Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Clear()
	second, err := a.ReadSnapshotPage(context.Background(), retainedTestID, ref, t.TempDir(), from, from.Add(time.Hour), 1, first.Next, "asc", from.Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Clear()
	strict, err := ReadSQLiteRows(context.Background(), ArchiveFile{Name: "901.db", Data: data}, t.TempDir(), from, from.Add(time.Hour), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer strict.Clear()
	// Assert: content omission is explicit, outside-window classification is counted,
	// pagination loses no ordinary record, and the strict reader is unchanged.
	if len(first.Records) != 0 || first.Unsupported["native_information"] != 1 || first.SourceInformation != 2 || !first.HasMore || first.Next == nil || len(second.Records) != 1 || second.HasMore || second.SourceInformation != 2 || len(strict.Rows) != 2 || strict.DeferredControls["20"] != 2 {
		t.Fatalf("information classification mismatch: first=%d unsupported=%v source=%d more=%v cursor=%v second=%d second_more=%v second_source=%d strict_rows=%d strict_types=%v", len(first.Records), first.Unsupported, first.SourceInformation, first.HasMore, first.Next != nil, len(second.Records), second.HasMore, second.SourceInformation, len(strict.Rows), strict.DeferredControls)
	}
}

func TestSnapshotNativeExclusionAndUnclassifiedControlGuard(t *testing.T) {
	from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	known := attachmentField(6, attachmentField(45, []byte("msginfo.actionlist")))
	for _, tc := range []struct {
		name     string
		kind     int64
		metadata []byte
	}{
		{"missing", 20, nil}, {"malformed", 20, []byte{1}},
		{"oversized", 20, make([]byte, 262145)},
		{"different-action", 20, attachmentField(6, attachmentField(45, []byte("msginfo.actionlist.extra")))},
		{"multiple-attachments", 20, append(append([]byte(nil), known...), known...)},
		{"actual-undo", 36, known}, {"actual-delete", 33, known}, {"other-deferred", 25, known},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: an in-window ordinary record and an out-of-window unknown row.
			data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
				for _, row := range []struct {
					kind, stamp int64
					metadata    []byte
				}{{0, from.UnixMilli(), nil}, {tc.kind, from.Add(-time.Hour).UnixMilli(), tc.metadata}} {
					if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "12", "2", "3", "PRIVATE-TEXT", row.stamp, 0, row.kind, 1, row.metadata); err != nil {
						t.Fatal(err)
					}
				}
			})
			// Act.
			page, err := ReadSnapshotSQLitePage(context.Background(), ArchiveFile{Name: "901.db", Data: data}, t.TempDir(), from, from.Add(time.Hour), 1, nil, "asc")
			defer page.Clear()
			// Assert: no prefix or cursor escapes; action naming does not waive real controls.
			if tc.kind == 20 {
				if err != nil || page.SourceNativeExcluded != 1 || page.SourceInformation != 0 || len(page.Rows) != 1 || page.Rows[0].Type != 0 {
					t.Fatal("native exclusion did not match the pinned query", err)
				}
			} else if !errors.Is(err, ErrSnapshotControls) || len(page.Rows) != 0 || page.Next != nil {
				t.Fatal("unclassified control returned a prefix", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if snapshotInformation(ctx, known) {
		t.Fatal("cancelled classification accepted")
	}
}

func TestSnapshotInformationWholeSourceBudgetRejectsBeforePrefix(t *testing.T) {
	// Arrange: recognized information exceeds the fixed whole-file row budget,
	// entirely outside the requested ordinary-message interval.
	from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	metadata := attachmentField(6, attachmentField(45, []byte("msginfo.actionlist")))
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		for i := 0; i < 5001; i++ {
			if _, err := tx.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "12", "2", "3", "PRIVATE-TEXT", from.Add(-time.Hour).UnixMilli(), 0, 20, 1, metadata); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tx.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "12", "4", "5", "PRIVATE-TEXT", from.UnixMilli(), 0, 0, 1, nil); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	})
	// Act.
	page, err := ReadSnapshotSQLitePage(context.Background(), ArchiveFile{Name: "901.db", Data: data}, t.TempDir(), from, from.Add(time.Hour), 1, nil, "asc")
	defer page.Clear()
	// Assert: the ordinary prefix is withheld rather than partially classifying the file.
	if !errors.Is(err, ErrSnapshotControls) || len(page.Rows) != 0 || page.Next != nil {
		t.Fatal("oversized source classification returned a prefix", err)
	}
}

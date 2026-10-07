package mobilebackup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestSnapshotRecallsSuppressWholeSourceTargetsAcrossPagingAndNamespaces(t *testing.T) {
	for _, kind := range []string{"direct", "group"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange: recalled originals precede their out-of-window control. A zero-ID
			// copy matches by sender/client; another sender and another genuine ID do not.
			from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
			metadata := attachmentField(6, attachmentField(999, []byte("opaque")))
			data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
				rows := []struct {
					sender, global, client string
					typ, status, stamp     int64
				}{
					{"901", "2", "3", 0, 1, from.UnixMilli()},
					{"901", "0", "3", 0, 1, from.UnixMilli() + 1},
					{"900", "0", "3", 0, 1, from.UnixMilli() + 2},
					{"901", "4", "3", 0, 1, from.UnixMilli() + 3},
					{"901", "2", "3", 36, 3, from.Add(2 * time.Hour).UnixMilli()},
				}
				for _, r := range rows {
					if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", r.sender, r.global, r.client, "PRIVATE-SYNTHETIC-TEXT", r.stamp, 0, r.typ, r.status, metadata); err != nil {
						t.Fatal(err)
					}
				}
			})
			a := snapshotProjectionFixture(t, from)
			index := 0
			if kind == "group" {
				index = 1
			}
			a.archive.Files[index].Data = data
			ref := domain.ConversationRef{Type: kind, ID: "12"}
			// Act: empty suppressed pages must preserve examined-row continuation.
			var after *SQLiteCursor
			var visible []SnapshotRecord
			suppressed, pages := 0, 0
			for {
				p, err := a.ReadSnapshotPage(context.Background(), retainedTestID, ref, t.TempDir(), from, from.Add(time.Hour), 1, after, "asc", from.Add(time.Hour).UnixMilli())
				if err != nil {
					t.Fatal(err)
				}
				visible = append(visible, p.Records...)
				suppressed += p.SuppressedSource
				pages++
				if p.SourceRecallRows != 1 {
					t.Fatal("whole-source recall count was lost")
				}
				more := p.HasMore
				if p.Next != nil {
					c := *p.Next
					after = &c
				}
				p.Clear()
				if !more {
					break
				}
				if pages > 5 {
					t.Fatal("continuation did not advance")
				}
			}
			// Assert: only exact source targets are hidden; no namespace/global-ID alias.
			if len(visible) != 2 || suppressed != 2 || pages != 4 || visible[0].SenderID != "10" || visible[1].GlobalID != "4" {
				t.Fatal("source recall suppression mismatch")
			}
			other := domain.ConversationRef{Type: "group", ID: "12"}
			if kind == "group" {
				other.Type = "direct"
			}
			p, err := a.ReadSnapshotPage(context.Background(), retainedTestID, other, t.TempDir(), from, from.Add(time.Hour), 50, nil, "asc", from.Add(time.Hour).UnixMilli())
			if err != nil {
				t.Fatal(err)
			}
			defer p.Clear()
			if p.SourceRecallRows != 0 || p.SuppressedSource != 0 {
				t.Fatal("source targets crossed typed files")
			}
			strict, err := ReadSQLiteRows(context.Background(), a.archive.Files[index], t.TempDir(), from, from.Add(time.Hour), 50)
			if err != nil {
				t.Fatal(err)
			}
			defer strict.Clear()
			if strict.SourceControls != 1 || strict.SourceRecallRows != 0 || strict.recalls != nil {
				t.Fatal("strict import reader changed")
			}
		})
	}
}

func TestSnapshotRecallInvalidWholeSourceReturnsNoPrefix(t *testing.T) {
	from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name                   string
		sender, global, client any
		status, stamp, ttl     any
		duplicates             bool
	}{
		{"zero-global", "901", "0", "3", int64(3), from.UnixMilli(), int64(0), false},
		{"invalid-sender", "0", "2", "3", int64(3), from.UnixMilli(), int64(0), false},
		{"invalid-client", "901", "2", "0", int64(3), from.UnixMilli(), int64(0), false},
		{"other-status", "901", "2", "3", int64(1), from.UnixMilli(), int64(0), false},
		{"invalid-time", "901", "2", "3", int64(3), int64(-1), int64(0), false},
		{"invalid-ttl", "901", "2", "3", int64(3), from.UnixMilli(), int64(-1), false},
		{"duplicate-target", "901", "2", "3", int64(3), from.UnixMilli(), int64(0), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: a readable ordinary record and a bad whole-source recall.
			data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
				if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", "9", "8", "PRIVATE-TEXT", from.UnixMilli(), 0, 0, 1, nil); err != nil {
					t.Fatal(err)
				}
				count := 1
				if tc.duplicates {
					count = 2
				}
				for n := 0; n < count; n++ {
					if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", tc.sender, tc.global, tc.client, "PRIVATE-RECALLED-TEXT", tc.stamp, tc.ttl, 36, tc.status, nil); err != nil {
						t.Fatal(err)
					}
				}
			})
			// Act.
			p, err := ReadSnapshotSQLitePage(context.Background(), ArchiveFile{Name: "901.db", Data: data}, t.TempDir(), from.Add(time.Hour), from.Add(2*time.Hour), 1, nil, "asc")
			defer p.Clear()
			// Assert: even an otherwise empty interval cannot bypass source controls.
			if !errors.Is(err, ErrSnapshotControls) || len(p.Rows) != 0 || p.Next != nil {
				t.Fatal("invalid recall source escaped", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	conn, e := db.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	if result, e := classifySnapshotRecalls(ctx, conn, 1); !errors.Is(e, ErrSnapshotControls) || result != nil {
		t.Fatal("cancelled scan returned targets")
	}
	targets := &snapshotRecallTargets{global: map[string]struct{}{"PRIVATE-GLOBAL-ID": {}}, client: map[[2]string]struct{}{{"PRIVATE-SENDER", "PRIVATE-CLIENT"}: {}}}
	if fmt.Sprintf("%+v %#v", targets, targets) != "archive recall targets [redacted] archive recall targets [redacted]" {
		t.Fatal("target formatting exposed identities")
	}
}

func TestSnapshotRecallAndInformationShareWholeSourceBudget(t *testing.T) {
	// Arrange: each supported class fits separately but their combined set exceeds
	// the source budget. All controls are outside the requested interval.
	from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	metadata := attachmentField(6, attachmentField(45, []byte("msginfo.actionlist")))
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		for i := 0; i < 5001; i++ {
			kind, status := int64(20), int64(1)
			if i >= 2500 {
				kind, status = 36, 3
			}
			if _, err := tx.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", fmt.Sprint(i+1), fmt.Sprint(i+1), "PRIVATE-TEXT", from.Add(-time.Hour).UnixMilli(), 0, kind, status, metadata); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	})
	// Act.
	p, err := ReadSnapshotSQLitePage(context.Background(), ArchiveFile{Name: "901.db", Data: data}, t.TempDir(), from, from.Add(time.Hour), 1, nil, "asc")
	defer p.Clear()
	// Assert: no targets, prefix or continuation bypass the combined limit.
	if !errors.Is(err, ErrSnapshotControls) || len(p.Rows) != 0 || p.Next != nil || p.recalls != nil {
		t.Fatal("combined source control budget bypassed", err)
	}
}

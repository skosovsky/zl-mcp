package mobilebackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSnapshotSQLiteZeroIDsPagingAndImportIsolation(t *testing.T) {
	// Arrange: mixed storage types, tied timestamps and records rejected after zero-ID handling.
	from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	until := from.Add(time.Hour)
	data := sqliteFixture(t, `CREATE TABLE ChatContent(SenderId TEXT,GlbMsgId,CliMsgId TEXT,MsgContent TEXT,TimeStamp INTEGER,TTL INTEGER,MsgType INTEGER,MsgStatus INTEGER,BinNet BLOB)`, func(db *sql.DB) {
		for i, id := range []any{int64(0), "0", "2", "0", "00", nil} {
			client := fmt.Sprint(i + 1)
			if i == 3 {
				client = "0"
			}
			if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "12", id, client, "synthetic", from.UnixMilli(), 0, 0, 1, nil); err != nil {
				t.Fatal(err)
			}
		}
	})
	file := ArchiveFile{Name: "901.db", Data: data}
	ctx := context.Background()
	scratch := t.TempDir()
	// Act: snapshot pages and the strict reader see the exact same immutable source.
	first, err := ReadSnapshotSQLitePage(ctx, file, scratch, from, until, 2, nil, "asc")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Clear()
	second, err := ReadSnapshotSQLitePage(ctx, file, scratch, from, until, 2, first.Next, "asc")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Clear()
	third, err := ReadSnapshotSQLitePage(ctx, file, scratch, from, until, 2, second.Next, "asc")
	if err != nil {
		t.Fatal(err)
	}
	defer third.Clear()
	strict, err := ReadSQLiteRows(ctx, file, scratch, from, until, 50)
	if err != nil {
		t.Fatal(err)
	}
	defer strict.Clear()
	// Assert: physical identities stay separate; zero IDs never become fabricated upstream IDs.
	if len(first.Rows) != 2 || first.Rows[0].MessageID != "" || first.Rows[1].MessageID != "" || first.Rows[0].SourceRowID != 1 || first.Rows[1].SourceRowID != 2 || !first.HasMore || !first.Next.Snapshot {
		t.Fatal("snapshot identity or cursor lost")
	}
	if len(second.Rows) != 1 || second.Rows[0].MessageID != "2" || second.RejectedReasons["client_id"] != 1 || !second.HasMore {
		t.Fatal("remaining scalar validation waived")
	}
	if len(third.Rows) != 0 || third.Rejected != 2 || third.HasMore || third.RejectedReasons["message_id"] != 2 {
		t.Fatal("invalid IDs repaired or rejected page not advanced")
	}
	if len(strict.Rows) != 1 || strict.Rejected != 5 || strict.Rows[0].MessageID != "2" || strict.Rows[0].SourceRowID != 0 {
		t.Fatal("strict import reader changed")
	}
	if page, err := ReadSQLitePage(ctx, file, scratch, from, until, 2, first.Next); err == nil {
		page.Clear()
		t.Fatal("snapshot cursor admitted to strict reader")
	}
	encoded, _ := json.Marshal(first.Rows[0])
	if string(encoded) != "{}" {
		t.Fatal("private row automatically serialized")
	}
	for _, mode := range []string{"order", "source", "window"} {
		selected := file
		start := from
		order := "asc"
		switch mode {
		case "order":
			order = "desc"
		case "source":
			selected.Name = "902.db"
		case "window":
			start = from.Add(time.Millisecond)
		}
		if page, err := ReadSnapshotSQLitePage(ctx, selected, scratch, start, until, 2, first.Next, order); err == nil {
			page.Clear()
			t.Fatal("changed cursor binding accepted")
		}
	}
	reverse, err := ReadSnapshotSQLitePage(ctx, file, scratch, from, until, 2, nil, "desc")
	if err != nil {
		t.Fatal(err)
	}
	defer reverse.Clear()
	if len(reverse.Rows) != 0 || !reverse.HasMore || reverse.Next.RowID != 5 || !reverse.Next.Descending {
		t.Fatal("descending rejected-only page did not advance")
	}
	next, err := ReadSnapshotSQLitePage(ctx, file, scratch, from, until, 2, reverse.Next, "desc")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Clear()
	if len(next.Rows) != 1 || next.Rows[0].SourceRowID != 3 {
		t.Fatal("descending tie order failed")
	}
}

func TestSnapshotSQLiteBlocksEveryDeferredControlOutsideWindow(t *testing.T) {
	for kind := int64(0); kind <= 100; kind++ {
		if !deferredMobileControl(kind) {
			continue
		}
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			// Arrange: one ordinary in-window row and a control outside that interval.
			from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
			data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
				for _, row := range [][3]int64{{0, from.UnixMilli(), 1}, {kind, from.Add(-time.Hour).UnixMilli(), 2}} {
					if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "12", fmt.Sprint(row[2]), fmt.Sprint(row[2]), "synthetic", row[1], 0, row[0], 1, nil); err != nil {
						t.Fatal(err)
					}
				}
			})
			// Act: scan whole-file controls before returning the requested prefix.
			page, err := ReadSnapshotSQLitePage(context.Background(), ArchiveFile{Name: "901.db", Data: data}, t.TempDir(), from, from.Add(time.Hour), 1, nil, "asc")
			defer page.Clear()
			// Assert: no text, row or cursor escapes a source with unclassified controls.
			if !errors.Is(err, ErrSnapshotControls) || len(page.Rows) != 0 || page.Next != nil {
				t.Fatal("control-bearing snapshot returned prefix", err)
			}
		})
	}
}

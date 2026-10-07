package mobilebackup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRejectedMessageIDShapesDoNotRepairOrExposePrivateValues(t *testing.T) {
	// Arrange: invalid representations cannot turn into source or session IDs.
	cases := []struct {
		value any
		want  string
	}{
		{strings.Repeat("PRIVATE-ID", 1000), "oversized"}, {nil, "missing"}, {"", "empty"}, {int64(0), "zero"}, {"0", "zero"}, {int64(-1), "negative_integer"}, {"-12", "negative_text"}, {"001", "leading_zero"}, {"PRIVATE-ID", "nondigit"}, {"18446744073709551616", "overflow"}, {"999999999999999999999999999999", "overflow"}, {[]byte("PRIVATE-ID"), "wrong_storage_type"},
	}
	for _, test := range cases {
		// Act.
		shape := rejectedMessageIDShape(test.value)
		_, accepted := sqliteID(test.value)
		// Assert.
		if shape != test.want || accepted {
			t.Fatal("shape or original ID acceptance changed")
		}
	}
}
func TestRejectedMessageIDContextPartitionsPagedRowsWithoutContent(t *testing.T) {
	// Arrange: independent SQLite image; first page has only rejected global IDs.
	since := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		rows := [][9]any{
			{"1", nil, "2", "PRIVATE-TEXT", since.UnixMilli(), 0, 0, 1, nil},
			{"1", "0", nil, "", since.UnixMilli(), 0, 36, 0, nil},
			{"1", "PRIVATE-ID", "2", "PRIVATE-TEXT", since.UnixMilli(), 0, 999, 1, nil},
			{"1", "3", "2", "ok", since.UnixMilli(), 0, 0, 1, nil},
		}
		for _, row := range rows {
			if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", row[:]...); err != nil {
				t.Fatal(err)
			}
		}
	})
	file := ArchiveFile{Name: "1.db", Data: data}
	// Act.
	first, err := ReadSQLitePage(context.Background(), file, t.TempDir(), since, since.Add(time.Hour), 2, nil)
	defer first.Clear()
	if err != nil || first.Next == nil {
		t.Fatal("no rejection-page continuation", err)
	}
	second, err := ReadSQLitePage(context.Background(), file, t.TempDir(), since, since.Add(time.Hour), 2, first.Next)
	defer second.Clear()
	encoded, _ := json.Marshal(first.RejectedMessageIDContext)
	// Assert: each independent group partitions the same rows, without invented metadata/IDs or loss of progress.
	if err != nil || first.RejectedReasons["message_id"] != 2 || first.RejectedMessageIDShapes["missing"] != 1 || first.RejectedMessageIDShapes["zero"] != 1 || first.RejectedMessageIDContext["positive_status"] != 1 || first.RejectedMessageIDContext["nonpositive_status"] != 1 || first.RejectedMessageIDContext["known_payload_kind"] != 1 || first.RejectedMessageIDContext["source_control_kind"] != 1 || first.RejectedMessageIDContext["source_text_present"] != 1 || first.RejectedMessageIDContext["source_text_absent"] != 1 || first.RejectedMessageIDContext["client_id_valid"] != 1 || first.RejectedMessageIDContext["client_id_invalid"] != 1 || second.RejectedMessageIDShapes["nondigit"] != 1 || second.RejectedMessageIDContext["unknown_kind"] != 1 || len(second.Rows) != 1 || second.HasMore || bytes.Contains(encoded, []byte("PRIVATE")) {
		t.Fatal("diagnostic partition or pagination failed", err)
	}
	first.Clear()
	if first.RejectedMessageIDShapes != nil || first.RejectedMessageIDContext != nil {
		t.Fatal("diagnostic map retained")
	}
}

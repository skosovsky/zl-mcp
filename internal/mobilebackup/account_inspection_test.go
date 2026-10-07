package mobilebackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAccountArchiveOfflineCoveragePreservesAllFilesAndPeriods(t *testing.T) {
	// Arrange: one synthetic direct file, one group and one unsupported file.
	at := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	bytes := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for i := 0; i < 2; i++ {
			if _, e := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "10", "20", "30", "PRIVATE-TEXT-MARKER", at.Add(time.Duration(i)*24*time.Hour).UnixMilli(), 0, 0, 1, nil); e != nil {
				t.Fatal(e)
			}
		}
	})
	a := retainedFixture(t)
	a.archive.Files[0].Data = append([]byte(nil), bytes...)
	a.archive.Files[1].Data = append([]byte(nil), bytes...)
	ctx := context.Background()
	// Act: inspect different intervals and file offsets without any mapper/transport.
	first, more, e := a.InspectCoverage(ctx, t.TempDir(), at, at.Add(24*time.Hour), 0, 1)
	rest, last, e2 := a.InspectCoverage(ctx, t.TempDir(), at, at.Add(48*time.Hour), 1, 2)
	// Assert: typed ordinals, exact source/period coverage, unsupported file stays visible.
	if e != nil || e2 != nil || !more || last || first[0].SourceRows != 2 || first[0].PeriodRows != 1 || rest[0].ConversationType != "group" || rest[0].PeriodRows != 2 || rest[1].Status != "unreadable_sqlite" {
		t.Fatal("offline source coverage mismatch", e, e2)
	}
	encoded, _ := json.Marshal(rest)
	if strings.Contains(string(encoded), "PRIVATE-TEXT-MARKER") || strings.Contains(string(encoded), "9007199254740993") {
		t.Fatal("private source serialized")
	}
	if _, _, e = a.InspectCoverage(ctx, t.TempDir(), at, at, 0, 1); e == nil {
		t.Fatal("empty interval accepted")
	}
}

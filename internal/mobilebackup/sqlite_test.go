package mobilebackup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sqliteFixture(t *testing.T, schema string, populate func(*sql.DB)) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if populate != nil {
		populate(db)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

const backupSQLiteSchema = `CREATE TABLE ChatContent(SenderId TEXT,GlbMsgId TEXT,CliMsgId TEXT,MsgContent TEXT,TimeStamp INTEGER,TTL INTEGER,MsgType INTEGER,MsgStatus INTEGER,BinNet BLOB)`

func TestSQLiteRowsPeriodBoundsPrecisionAndCleanup(t *testing.T) {
	// Arrange: synthetic source rows include one invalid identity and an exclusive end row.
	since := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for i, offset := range []int64{-1, 0, 1, 2, until.UnixMilli() - since.UnixMilli()} {
			sender := "9007199254740993"
			if i == 2 {
				sender = "01"
			}
			_, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", sender, "18446744073709551615", "9007199254740997", "synthetic text", since.UnixMilli()+offset, 0, 0, 1, []byte("opaque synthetic BinNet"))
			if err != nil {
				t.Fatal(err)
			}
		}
	})
	before := append([]byte(nil), data...)
	scratch := t.TempDir()
	// Act: two examined rows plus a has_more probe.
	batch, err := ReadSQLiteRows(context.Background(), ArchiveFile{Name: "9007199254740993.db", Data: data}, scratch, since, until, 2)
	// Assert: no rounding, inclusive/exclusive window, invalid row counted, no source mutation or leftovers.
	if err != nil || batch.Examined != 2 || batch.Rejected != 1 || !batch.HasMore || len(batch.Rows) != 1 || batch.Rows[0].MessageID != "18446744073709551615" || batch.Rows[0].TimestampMS != since.UnixMilli() {
		t.Fatal("bounded SQLite read failed", err)
	}
	if !bytes.Equal(data, before) {
		t.Fatal("source bytes changed")
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatal("private scratch retained")
	}
	encoded, _ := json.Marshal(batch.Rows[0])
	if string(encoded) != "{}" {
		t.Fatal("row serialization exposed private values")
	}
	batch, err = ReadSQLiteRows(context.Background(), ArchiveFile{Name: "1.db", Data: data}, scratch, since, until, 10)
	if err != nil || batch.Examined != 3 || batch.Rejected != 1 || batch.HasMore || len(batch.Rows) != 2 {
		t.Fatal("complete candidate batch mismatch", err)
	}
}

func TestSQLiteRejectsHeaderAndUntrustedSchema(t *testing.T) {
	since := time.Now()
	until := since.Add(time.Hour)
	valid := sqliteFixture(t, backupSQLiteSchema, nil)
	badPage := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(badPage[16:18], 513)
	mixedVersion := append([]byte(nil), valid...)
	mixedVersion[18] = 2
	mixedVersion[19] = 1
	badCount := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(badCount[28:32], uint32(len(valid)/4096+1))
	for _, data := range [][]byte{valid[:100], badPage, mixedVersion, badCount, append(append([]byte(nil), valid...), 0), sqliteFixture(t, `CREATE VIEW ChatContent AS SELECT 1 AS SenderId`, nil), sqliteFixture(t, `CREATE TABLE ChatContent(SenderId TEXT)`, nil), sqliteFixture(t, `CREATE TABLE ChatContent(SenderId TEXT,GlbMsgId TEXT,CliMsgId TEXT,MsgContent TEXT,TimeStamp INTEGER,TTL INTEGER,MsgType INTEGER,MsgStatus INTEGER,BinNet BLOB,hiddenValue AS (1))`, nil)} {
		// Act / Assert: no partial results, and cleanup after schema failure.
		scratch := t.TempDir()
		batch, err := ReadSQLiteRows(context.Background(), ArchiveFile{Name: "1.db", Data: data}, scratch, since, until, 10)
		if !errors.Is(err, ErrSQLite) || batch.Rows != nil {
			t.Fatal("invalid SQLite accepted")
		}
		entries, _ := os.ReadDir(scratch)
		if len(entries) != 0 {
			t.Fatal("failed inspection retained files")
		}
	}
}

func TestSQLiteRejectsCancelledAndInvalidRequests(t *testing.T) {
	// Arrange.
	data := sqliteFixture(t, backupSQLiteSchema, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scratch := t.TempDir()
	since := time.Now()
	// Act / Assert.
	if _, err := ReadSQLiteRows(ctx, ArchiveFile{Name: "1.db", Data: data}, scratch, since, since.Add(time.Hour), 10); !errors.Is(err, ErrSQLite) {
		t.Fatal("cancel bypass")
	}
	for _, limit := range []int{0, 5001} {
		if _, err := ReadSQLiteRows(context.Background(), ArchiveFile{Name: "1.db", Data: data}, scratch, since, since.Add(time.Hour), limit); !errors.Is(err, ErrSQLite) {
			t.Fatal("row budget bypass")
		}
	}
	entries, _ := os.ReadDir(scratch)
	if len(entries) != 0 {
		t.Fatal("invalid request created scratch")
	}
}

func TestSQLitePaginationTiesRejectedRowsAndBoundCursor(t *testing.T) {
	// Arrange: five rows share a timestamp; invalid middle rows must still advance.
	since := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for i := 1; i <= 5; i++ {
			sender := "1"
			if i == 2 || i == 4 {
				sender = "01"
			}
			if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", sender, i, i, "synthetic text", since.UnixMilli(), 0, 0, 1, nil); err != nil {
				t.Fatal(err)
			}
		}
	})
	file := ArchiveFile{Name: "1.db", Data: data}
	scratch := t.TempDir()
	var cursor *SQLiteCursor
	var ids []string
	examined, rejected, pages := 0, 0, 0
	// Act: changing page size must preserve the same bound filter.
	for {
		size := 2
		if pages == 1 {
			size = 1
		}
		batch, err := ReadSQLitePage(context.Background(), file, scratch, since, until, size, cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		examined += batch.Examined
		rejected += batch.Rejected
		for _, row := range batch.Rows {
			ids = append(ids, row.MessageID)
		}
		if !batch.HasMore {
			if batch.Next != nil {
				t.Fatal("terminal cursor emitted")
			}
			break
		}
		if batch.Next == nil || batch.Examined == 0 {
			t.Fatal("continuation failed to advance")
		}
		cursor = batch.Next
		if pages > 5 {
			t.Fatal("cursor loop")
		}
	}
	// Assert: every source row examined once; no valid message lost or repeated.
	if examined != 5 || rejected != 2 || pages != 3 || len(ids) != 3 || ids[0] != "1" || ids[1] != "3" || ids[2] != "5" {
		t.Fatal("pagination gaps or repeats")
	}
	encoded, _ := json.Marshal(cursor)
	if string(encoded) != "{}" {
		t.Fatal("cursor exposed")
	}
	first, err := ReadSQLitePage(context.Background(), file, scratch, since, until, 1, nil)
	if err != nil || first.Next == nil {
		t.Fatal("missing first cursor")
	}
	for _, changed := range []ArchiveFile{{Name: "2.db", Data: data}, {Name: "1.db", Data: append(append([]byte(nil), data[:len(data)-1]...), data[len(data)-1]^1)}} {
		if _, err := ReadSQLitePage(context.Background(), changed, scratch, since, until, 1, first.Next); !errors.Is(err, ErrSQLite) {
			t.Fatal("cursor crossed file boundary")
		}
	}
	if _, err := ReadSQLitePage(context.Background(), file, scratch, since.Add(-time.Minute), until, 1, first.Next); !errors.Is(err, ErrSQLite) {
		t.Fatal("cursor crossed period")
	}
	invalid := *first.Next
	invalid.TimestampMS = until.UnixMilli()
	if _, err := ReadSQLitePage(context.Background(), file, scratch, since, until, 1, &invalid); !errors.Is(err, ErrSQLite) {
		t.Fatal("out-of-period anchor accepted")
	}
}

func TestSQLiteRejectsShadowedOrInexactPagingKeys(t *testing.T) {
	since := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	shadowed := sqliteFixture(t, backupSQLiteSchema+`;ALTER TABLE ChatContent ADD COLUMN rowid INTEGER`, nil)
	inexact := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "1", "2", "3", "text", float64(since.UnixMilli())+0.5, 0, 0, 1, nil); err != nil {
			t.Fatal(err)
		}
	})
	for _, data := range [][]byte{shadowed, inexact} {
		// Act / Assert: no fabricated paging anchor or partial batch.
		batch, err := ReadSQLitePage(context.Background(), ArchiveFile{Name: "1.db", Data: data}, t.TempDir(), since, until, 1, nil)
		if !errors.Is(err, ErrSQLite) || batch.Rows != nil || batch.Next != nil {
			t.Fatal("invalid paging key accepted")
		}
	}
}

func TestSQLiteBatchRetainedByteBudget(t *testing.T) {
	// Arrange: individually acceptable rows exceed the total response budget.
	since := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	until := since.Add(time.Hour)
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		text := string(bytes.Repeat([]byte("x"), 512<<10))
		for i := 0; i < 17; i++ {
			if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "1", "2", "3", text, since.UnixMilli()+int64(i), 0, 0, 1, nil); err != nil {
				t.Fatal(err)
			}
		}
	})
	// Act.
	batch, err := ReadSQLitePage(context.Background(), ArchiveFile{Name: "1.db", Data: data}, t.TempDir(), since, until, 17, nil)
	// Assert: no partial response/continuation; a smaller page remains supported.
	if !errors.Is(err, ErrSQLite) || batch.Rows != nil || batch.Next != nil {
		t.Fatal("total byte budget bypass")
	}
	batch, err = ReadSQLitePage(context.Background(), ArchiveFile{Name: "1.db", Data: data}, t.TempDir(), since, until, 1, nil)
	if err != nil || len(batch.Rows) != 1 || !batch.HasMore || batch.Next == nil {
		t.Fatal("bounded smaller page failed")
	}
	batch.Clear()
}

func TestSQLiteFractionalMillisecondWindow(t *testing.T) {
	// Arrange: exact millisecond source timestamps, fractional inclusive/exclusive bounds.
	base := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for i := int64(0); i < 3; i++ {
			if _, e := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", fmt.Sprint(i+1), fmt.Sprint(i+1), "synthetic", base.UnixMilli()+i, 0, 0, 1, []byte{0, 0, 0, 9, 0, 0, 0, 1, 'x'}); e != nil {
				t.Fatal(e)
			}
		}
	})
	// Act: [0.5 ms,1.5 ms) contains only the source row at 1 ms.
	page, e := ReadSQLitePage(context.Background(), ArchiveFile{Name: "902.db", Data: data}, t.TempDir(), base.Add(500*time.Microsecond), base.Add(1500*time.Microsecond), 50, nil)
	defer page.Clear()
	// Assert: neither include a pre-window row nor lose the eligible upper-millisecond row.
	if e != nil || len(page.Rows) != 1 || page.Rows[0].TimestampMS != base.UnixMilli()+1 {
		t.Fatal("fractional window shifted source rows", e)
	}
}

func TestSQLiteFailureDiagnosticsExcludePrivateFile(t *testing.T) {
	// Arrange: private malformed file data in an otherwise valid read selection.
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	data := make([]byte, 512)
	copy(data, []byte("private-message-marker"))
	from := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	// Act.
	_, err := ReadSQLiteRows(context.Background(), ArchiveFile{Name: "123456789.db", Data: data}, t.TempDir(), from, from.Add(time.Hour), 1)
	// Assert: only the fixed failing stage and header counts, no content or filename.
	if err == nil || !strings.Contains(output.String(), "SQLITE_HEADER") {
		t.Fatal("missing header diagnosis")
	}
	for _, private := range []string{"private-message-marker", "123456789.db"} {
		if strings.Contains(output.String(), private) {
			t.Fatal("private archive diagnostic leaked")
		}
	}
}

func TestSQLiteImmutableCheckpointedWALSnapshot(t *testing.T) {
	// Arrange: SQLite itself generates the 2/2 header; checkpoint before extracting
	// the main-file image. The reader sees only a private immutable copy.
	from := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	data := sqliteFixture(t, "PRAGMA journal_mode=WAL;"+backupSQLiteSchema, func(db *sql.DB) {
		if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "12", "13", "14", "synthetic snapshot", from.UnixMilli(), 0, 0, 1, nil); err != nil {
			t.Fatal(err)
		}
		var busy, logPages, checkpointed int
		if err := db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logPages, &checkpointed); err != nil || busy != 0 {
			t.Fatal("fixture checkpoint failed", err)
		}
	})
	if data[18] != 2 || data[19] != 2 {
		t.Fatal("fixture is not WAL mode")
	}
	original := append([]byte(nil), data...)
	scratch := t.TempDir()
	// Act.
	batch, err := ReadSQLiteRows(context.Background(), ArchiveFile{Name: "12.db", Data: data}, scratch, from, from.Add(time.Hour), 10)
	// Assert: available committed main-file rows, unchanged image, no sidecars/scratch.
	if err != nil || !batch.WALMode || len(batch.Rows) != 1 || batch.Rows[0].MessageID != "13" {
		t.Fatal("immutable WAL read failed", err)
	}
	batch.Clear()
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 || !bytes.Equal(data, original) {
		t.Fatal("snapshot mutated or scratch retained")
	}
	for _, version := range []byte{0, 3, 255} {
		invalid := append([]byte(nil), data...)
		invalid[18], invalid[19] = version, version
		if standaloneSQLite(invalid) {
			t.Fatal("unsupported format accepted")
		}
	}
}

func TestSQLiteCoverageDistinguishesEmptyWindowFromEmptyImage(t *testing.T) {
	// Arrange: selected snapshot has valid records outside the requested interval
	// and one malformed timestamp. Source coverage must not count only valid messages.
	from := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	until := from.Add(time.Hour)
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for _, ts := range []any{from.UnixMilli() - 1, until.UnixMilli(), "invalid-time"} {
			if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "12", "13", "14", "synthetic", ts, 0, 0, 1, nil); err != nil {
				t.Fatal(err)
			}
		}
	})
	// Act.
	batch, err := ReadSQLiteRows(context.Background(), ArchiveFile{Name: "12.db", Data: data}, t.TempDir(), from, until, 10)
	// Assert: no period matches, but nonempty image and exact valid timestamp bounds.
	c := batch.Coverage
	if err != nil || batch.Examined != 0 || c.SourceRows != 3 || c.PeriodRows != 0 || c.InvalidTimestamps != 1 || !c.HasRange || c.EarliestMS != from.UnixMilli()-1 || c.LatestMS != until.UnixMilli() {
		t.Fatal("incorrect source coverage", err)
	}
	empty := sqliteFixture(t, backupSQLiteSchema, nil)
	batch, err = ReadSQLiteRows(context.Background(), ArchiveFile{Name: "12.db", Data: empty}, t.TempDir(), from, until, 10)
	if err != nil || batch.Coverage.SourceRows != 0 || batch.Coverage.HasRange {
		t.Fatal("empty snapshot has invented range", err)
	}
}

func TestSQLiteImmutableMainImageCannotProveWALCompleteness(t *testing.T) {
	// Arrange: checkpoint one message, then commit another message and a control
	// only into WAL. Keep the producer connection open so close cannot checkpoint.
	path := filepath.Join(t.TempDir(), "producer.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0;" + backupSQLiteSchema); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	insert := func(id string, kind int) {
		t.Helper()
		if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "12", id, id, "synthetic", from.UnixMilli(), 0, kind, 1, nil); err != nil {
			t.Fatal(err)
		}
	}
	insert("13", 0)
	var busy, logPages, checkpointed int
	if err = db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logPages, &checkpointed); err != nil || busy != 0 {
		t.Fatal("checkpoint failed", err)
	}
	insert("14", 0)
	insert("15", 33)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(data)
	// Act: inspect only the copied main image, then compare with the producer view.
	batch, err := ReadSQLiteRows(context.Background(), ArchiveFile{Name: "12.db", Data: data}, t.TempDir(), from, from.Add(time.Hour), 10)
	defer batch.Clear()
	var producerRows, producerControls int
	queryErr := db.QueryRow("SELECT count(*), sum(CASE WHEN MsgType=33 THEN 1 ELSE 0 END) FROM ChatContent").Scan(&producerRows, &producerControls)
	// Assert: successful integrity/schema checks cannot detect omitted commits,
	// including a control. This is synthetic evidence, not a claim about Zalo export.
	if err != nil || queryErr != nil || !batch.WALMode || batch.Coverage.SourceRows != 1 || batch.SourceControls != 0 || len(batch.Rows) != 1 || batch.Rows[0].MessageID != "13" || producerRows != 3 || producerControls != 1 {
		t.Fatal("WAL/main-image evidence boundary lost", err, queryErr)
	}
}

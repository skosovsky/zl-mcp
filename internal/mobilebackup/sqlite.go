package mobilebackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var ErrSQLite = errors.New("invalid mobile backup SQLite file")
var ErrSnapshotControls = errors.New("archive snapshot controls unclassified")

type SQLiteRow struct {
	SenderID, MessageID, ClientID, Text string `json:"-"`
	TimestampMS, TTL, Type, Status      int64  `json:"-"`
	SourceRowID                         int64  `json:"-"`
	BinNet                              []byte `json:"-"`
}

func (SQLiteRow) String() string   { return "mobile backup SQLite row [redacted]" }
func (SQLiteRow) GoString() string { return "mobile backup SQLite row [redacted]" }

// SQLiteBatch is private candidate data; counts do not establish complete history.
type SQLiteCoverage struct {
	SourceRows, PeriodRows, InvalidTimestamps int64
	EarliestMS, LatestMS                      int64
	HasRange                                  bool
}

type SQLiteBatch struct {
	Coverage                 SQLiteCoverage `json:"-"`
	SourceControls           int
	SourceInformation        int
	SourceNativeExcluded     int
	SourceRecallRows         int
	recalls                  *snapshotRecallTargets `json:"-"`
	DeferredControls         map[string]int         `json:"-"`
	WALMode                  bool
	Rows                     []SQLiteRow    `json:"-"`
	RejectedMessageIDShapes  map[string]int `json:"-"`
	RejectedMessageIDContext map[string]int `json:"-"`
	RejectedReasons          map[string]int `json:"-"`
	Examined, Rejected       int
	HasMore                  bool
	Next                     *SQLiteCursor `json:"-"`
}

// SQLiteCursor is private keyset state bound to exact bytes, name and date filter.
type SQLiteCursor struct {
	Digest                               [32]byte `json:"-"`
	Name                                 string   `json:"-"`
	SinceMS, UntilMS, TimestampMS, RowID int64    `json:"-"`
	Descending                           bool     `json:"-"`
	Snapshot                             bool     `json:"-"`
}

func (SQLiteCursor) String() string   { return "mobile backup SQLite cursor [redacted]" }
func (SQLiteCursor) GoString() string { return "mobile backup SQLite cursor [redacted]" }

func (SQLiteBatch) String() string   { return "mobile backup SQLite batch [redacted]" }
func (SQLiteBatch) GoString() string { return "mobile backup SQLite batch [redacted]" }
func (b *SQLiteBatch) Clear() {
	if b == nil {
		return
	}
	for i := range b.Rows {
		clear(b.Rows[i].BinNet)
		b.Rows[i] = SQLiteRow{}
	}
	b.Rows = nil
	b.Next = nil
	b.recalls.clear()
	b.recalls = nil
	clear(b.RejectedMessageIDShapes)
	b.RejectedMessageIDShapes = nil
	clear(b.RejectedMessageIDContext)
	b.RejectedMessageIDContext = nil
	clear(b.RejectedReasons)
	b.RejectedReasons = nil
	clear(b.DeferredControls)
	b.DeferredControls = nil
}

func standaloneSQLite(data []byte) bool {
	if len(data) < 512 || uint64(len(data)) > MaxFileBytes || string(data[:16]) != "SQLite format 3\x00" {
		return false
	}
	page := int(binary.BigEndian.Uint16(data[16:18]))
	if page == 1 {
		page = 65536
	}
	if page < 512 || page > 65536 || page&(page-1) != 0 || len(data)%page != 0 || !(data[18] == 1 && data[19] == 1 || data[18] == 2 && data[19] == 2) || page-int(data[20]) < 480 || !bytes.Equal(data[21:24], []byte{64, 32, 32}) || !bytes.Equal(data[72:92], make([]byte, 20)) {
		return false
	}
	count := binary.BigEndian.Uint32(data[28:32])
	if count != 0 && bytes.Equal(data[24:28], data[92:96]) && uint64(count)*uint64(page) != uint64(len(data)) {
		return false
	}
	return true
}

func ReadSQLiteRows(parent context.Context, file ArchiveFile, scratch string, since, until time.Time, maxRows int) (SQLiteBatch, error) {
	return ReadSQLitePage(parent, file, scratch, since, until, maxRows, nil)
}

func ReadSQLitePage(parent context.Context, file ArchiveFile, scratch string, since, until time.Time, maxRows int, after *SQLiteCursor) (SQLiteBatch, error) {
	return readSQLitePage(parent, file, scratch, since, until, maxRows, after, false)
}

// Control scans project scalar identities only; source text and BinNet are not read.
func readSQLitePage(parent context.Context, file ArchiveFile, scratch string, since, until time.Time, maxRows int, after *SQLiteCursor, controlsOnly bool) (batch SQLiteBatch, err error) {
	return readSQLitePageMode(parent, file, scratch, since, until, maxRows, after, controlsOnly, false, false)
}

// ReadSnapshotSQLitePage is a private read layer: zero global IDs are not corpus IDs.
func ReadSnapshotSQLitePage(parent context.Context, file ArchiveFile, scratch string, since, until time.Time, maxRows int, after *SQLiteCursor, order string) (SQLiteBatch, error) {
	if maxRows < 1 || maxRows > 50 || (order != "asc" && order != "desc") {
		return SQLiteBatch{}, ErrSQLite
	}
	return readSQLitePageMode(parent, file, scratch, since, until, maxRows, after, false, true, order == "desc")
}

func readSQLitePageMode(parent context.Context, file ArchiveFile, scratch string, since, until time.Time, maxRows int, after *SQLiteCursor, controlsOnly, snapshot, descending bool) (batch SQLiteBatch, err error) {
	stage := "INPUT_BINDING"
	defer func() {
		if err != nil {
			batch.Clear()
			slog.Warn("mobile_archive_sqlite_failed", "stage", stage, "cancelled", parent != nil && parent.Err() != nil)
		}
	}()
	from, to, validWindow := millisecondWindow(since, until)
	if !validWindow || parent == nil || parent.Err() != nil || !validName(file.Name) || !filepath.IsAbs(scratch) || maxRows < 1 || maxRows > 5000 || since.IsZero() || !until.After(since) {
		return batch, ErrSQLite
	}
	stage = "SQLITE_HEADER"
	if !standaloneSQLite(file.Data) {
		modeRead, modeWrite := 0, 0
		if len(file.Data) >= 20 {
			modeRead = int(file.Data[19])
			modeWrite = int(file.Data[18])
		}
		slog.Warn("mobile_archive_sqlite_header", "file_bytes", len(file.Data), "read_version", modeRead, "write_version", modeWrite)
		return batch, ErrSQLite
	}
	batch.WALMode = file.Data[18] == 2
	stage = "CURSOR_BINDING"
	digest := sha256.Sum256(file.Data)
	if after != nil && (after.Digest != digest || after.Name != file.Name || after.SinceMS != from || after.UntilMS != to || after.TimestampMS < from || after.TimestampMS >= to || after.Descending != descending || after.Snapshot != snapshot) {
		return batch, ErrSQLite
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	stage = "SCRATCH_CREATE"
	dir, e := os.MkdirTemp(scratch, "mobile-read-")
	if e != nil {
		return batch, ErrSQLite
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "archive.sqlite")
	stage = "SCRATCH_WRITE"
	if os.WriteFile(path, file.Data, 0600) != nil {
		return batch, ErrSQLite
	}
	absolute, e := filepath.Abs(path)
	if e != nil {
		return batch, ErrSQLite
	}
	uri := url.URL{Scheme: "file", Path: absolute}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Set("immutable", "1")
	for _, pragma := range []string{"trusted_schema(OFF)", "query_only(ON)", "temp_store(MEMORY)", "cache_size(-2048)"} {
		query.Add("_pragma", pragma)
	}
	uri.RawQuery = query.Encode()
	stage = "SQLITE_OPEN"
	db, e := sql.Open("sqlite", uri.String())
	if e != nil {
		return batch, ErrSQLite
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	stage = "SQLITE_CONNECTION"
	conn, e := db.Conn(ctx)
	if e != nil {
		return batch, ErrSQLite
	}
	defer conn.Close()
	stage = "SQLITE_LIMITS"
	for _, limit := range [][2]int{{sqlite3.SQLITE_LIMIT_LENGTH, 1 << 20}, {sqlite3.SQLITE_LIMIT_SQL_LENGTH, 16 << 10}, {sqlite3.SQLITE_LIMIT_COLUMN, 64}, {sqlite3.SQLITE_LIMIT_EXPR_DEPTH, 20}, {sqlite3.SQLITE_LIMIT_ATTACHED, 0}} {
		if _, e = sqlite.Limit(conn, limit[0], limit[1]); e != nil {
			return batch, ErrSQLite
		}
	}
	stage = "INTEGRITY_CHECK"
	var check string
	if conn.QueryRowContext(ctx, "PRAGMA integrity_check(1)").Scan(&check) != nil || check != "ok" {
		return batch, ErrSQLite
	}
	stage = "CHAT_TABLE"
	var kind string
	var root int64
	if conn.QueryRowContext(ctx, "SELECT type,rootpage FROM sqlite_schema WHERE name='ChatContent'").Scan(&kind, &root) != nil || kind != "table" || root <= 0 {
		return batch, ErrSQLite
	}
	stage = "COLUMN_SCHEMA"
	columns, e := conn.QueryContext(ctx, "SELECT name,type,hidden FROM pragma_table_xinfo('ChatContent')")
	if e != nil {
		return batch, ErrSQLite
	}
	required := map[string]bool{"senderid": false, "glbmsgid": false, "climsgid": false, "msgcontent": false, "timestamp": false, "ttl": false, "msgtype": false, "msgstatus": false, "binnet": false}
	for columns.Next() {
		var name, affinity string
		var hidden int
		if columns.Scan(&name, &affinity, &hidden) != nil || hidden != 0 {
			columns.Close()
			return batch, ErrSQLite
		}
		name = strings.ToLower(name)
		if name == "rowid" || name == "_rowid_" || name == "oid" {
			columns.Close()
			return batch, ErrSQLite
		}
		if _, ok := required[name]; ok {
			required[name] = true
		}
		if name == "timestamp" && !strings.Contains(strings.ToUpper(affinity), "INT") {
			columns.Close()
			return batch, ErrSQLite
		}
	}
	e = columns.Err()
	columns.Close()
	if e != nil {
		return batch, ErrSQLite
	}
	for _, present := range required {
		if !present {
			return batch, ErrSQLite
		}
	}
	stage = "SOURCE_TIME_COVERAGE"
	var earliest, latest sql.NullInt64
	const validTimestamp = "typeof(TimeStamp)='integer' AND TimeStamp>=0 AND TimeStamp<=253402300799999"
	coverageSQL := "SELECT count(*), coalesce(sum(CASE WHEN " + validTimestamp + " AND TimeStamp>=? AND TimeStamp<? THEN 1 ELSE 0 END),0), coalesce(sum(CASE WHEN " + validTimestamp + " THEN 0 ELSE 1 END),0), min(CASE WHEN " + validTimestamp + " THEN TimeStamp END), max(CASE WHEN " + validTimestamp + " THEN TimeStamp END) FROM ChatContent"
	if conn.QueryRowContext(ctx, coverageSQL, from, to).Scan(&batch.Coverage.SourceRows, &batch.Coverage.PeriodRows, &batch.Coverage.InvalidTimestamps, &earliest, &latest) != nil {
		return batch, ErrSQLite
	}
	batch.Coverage.HasRange = earliest.Valid && latest.Valid
	batch.Coverage.EarliestMS, batch.Coverage.LatestMS = earliest.Int64, latest.Int64
	// Controls outside the requested interval can still invalidate imported content.
	stage = "SOURCE_CONTROLS"
	if conn.QueryRowContext(ctx, "SELECT count(*) FROM ChatContent WHERE MsgType IN (33,36)").Scan(&batch.SourceControls) != nil {
		return SQLiteBatch{}, ErrSQLite
	}
	controlRows, e := conn.QueryContext(ctx, "SELECT MsgType,count(*) FROM ChatContent WHERE MsgType IN (20,21,25,26,29,32,33,34,35,36,45,51,52) GROUP BY MsgType")
	if e != nil {
		return SQLiteBatch{}, ErrSQLite
	}
	batch.DeferredControls = map[string]int{}
	for controlRows.Next() {
		var kind int64
		var count int
		if controlRows.Scan(&kind, &count) != nil || !deferredMobileControl(kind) || count < 1 {
			controlRows.Close()
			return SQLiteBatch{}, ErrSQLite
		}
		batch.DeferredControls[strconv.FormatInt(kind, 10)] = count
	}
	controlErr := controlRows.Err()
	controlRows.Close()
	if controlErr != nil || ctx.Err() != nil {
		return SQLiteBatch{}, ErrSQLite
	}
	if snapshot {
		if conn.QueryRowContext(ctx, "SELECT count(*) FROM ChatContent WHERE MsgType IN (20,21,25,26,29,32,33,34,35,36,45,51,52)").Scan(&batch.SourceControls) != nil {
			return SQLiteBatch{}, ErrSQLite
		}
		if batch.SourceControls > 5000 {
			return SQLiteBatch{}, ErrSnapshotControls
		}
		information := batch.DeferredControls["20"]
		if information > 0 {
			classified, err := classifySnapshotInformation(ctx, conn, information)
			if err != nil {
				return SQLiteBatch{}, err
			}
			batch.SourceInformation = classified
			batch.SourceNativeExcluded = information
			batch.SourceControls -= information
		}
		recalls := batch.DeferredControls["36"]
		if recalls > 0 {
			targets, err := classifySnapshotRecalls(ctx, conn, recalls)
			if err != nil {
				return SQLiteBatch{}, err
			}
			batch.recalls = targets
			batch.SourceRecallRows = recalls
			batch.SourceControls -= recalls
		}
		if batch.SourceControls != 0 {
			batch.recalls.clear()
			return SQLiteBatch{}, ErrSnapshotControls
		}
	}
	statement := "SELECT rowid,SenderId,GlbMsgId,CliMsgId,MsgContent,TimeStamp,TTL,MsgType,MsgStatus,BinNet FROM ChatContent WHERE TimeStamp>=? AND TimeStamp<?"
	if controlsOnly {
		statement = "SELECT rowid,SenderId,GlbMsgId,CliMsgId,'',TimeStamp,TTL,MsgType,MsgStatus,NULL FROM ChatContent WHERE TimeStamp>=? AND TimeStamp<? AND MsgType IN (33,36)"
	}
	args := []any{from, to}
	if after != nil {
		operator := ">"
		if descending {
			operator = "<"
		}
		statement += " AND (TimeStamp" + operator + "? OR (TimeStamp=? AND rowid" + operator + "?))"
		args = append(args, after.TimestampMS, after.TimestampMS, after.RowID)
	}
	if descending {
		statement += " ORDER BY TimeStamp DESC,rowid DESC LIMIT ?"
	} else {
		statement += " ORDER BY TimeStamp,rowid LIMIT ?"
	}
	args = append(args, maxRows+1)
	stage = "MESSAGE_QUERY"
	rows, e := conn.QueryContext(ctx, statement, args...)
	if e != nil {
		return batch, ErrSQLite
	}
	defer rows.Close()
	// Discard all previously accumulated private rows if query execution fails.
	defer func() {
		if err != nil {
			batch.Clear()
		}
	}()
	stage = "ROW_VALIDATION"
	var last SQLiteCursor
	retainedBytes := 0
	for rows.Next() {
		if batch.Examined == maxRows {
			batch.HasMore = true
			batch.Next = &last
			break
		}
		var values [9]any
		var sourceRowID int64
		if rows.Scan(&sourceRowID, &values[0], &values[1], &values[2], &values[3], &values[4], &values[5], &values[6], &values[7], &values[8]) != nil {
			return batch, ErrSQLite
		}
		timestamp, ok := values[4].(int64)
		if !ok || timestamp < from || timestamp >= to {
			return batch, ErrSQLite
		}
		last = SQLiteCursor{Digest: digest, Name: file.Name, SinceMS: from, UntilMS: to, TimestampMS: timestamp, RowID: sourceRowID, Descending: descending, Snapshot: snapshot}
		batch.Examined++
		row, reason := sqliteRowMode(values, from, to, snapshot)
		if snapshot {
			row.SourceRowID = sourceRowID
		}
		if reason != "" {
			batch.Rejected++
			if batch.RejectedReasons == nil {
				batch.RejectedReasons = map[string]int{}
			}
			batch.RejectedReasons[reason]++
			if reason == "message_id" {
				recordRejectedMessageID(&batch, values)
			}
			continue
		}
		retainedBytes += len(row.Text) + len(row.BinNet)
		if retainedBytes > 8<<20 {
			clear(row.BinNet)
			return batch, ErrSQLite
		}
		batch.Rows = append(batch.Rows, row)
	}
	if rows.Err() != nil || ctx.Err() != nil {
		return batch, ErrSQLite
	}
	return batch, nil
}

func sqliteID(value any) (string, bool) {
	var id string
	switch v := value.(type) {
	case string:
		id = v
	case int64:
		if v <= 0 {
			return "", false
		}
		id = strconv.FormatInt(v, 10)
	default:
		return "", false
	}
	return id, canonicalIdentity(id)
}

// sqliteRow keeps the original internal bool contract; diagnostics use only fixed reason keys.
func sqliteRow(values [9]any, since, until int64) (SQLiteRow, bool) {
	row, reason := sqliteRowChecked(values, since, until)
	return row, reason == ""
}

func sqliteRowChecked(values [9]any, since, until int64) (SQLiteRow, string) {
	return sqliteRowMode(values, since, until, false)
}

func sqliteRowMode(values [9]any, since, until int64, snapshot bool) (SQLiteRow, string) {
	var row SQLiteRow
	var ok bool
	row.SenderID, ok = sqliteID(values[0])
	if !ok {
		return row, "sender_id"
	}
	row.MessageID, ok = sqliteID(values[1])
	if snapshot {
		switch value := values[1].(type) {
		case int64:
			if value == 0 {
				row.MessageID = ""
				ok = true
			}
		case string:
			if value == "0" {
				row.MessageID = ""
				ok = true
			}
		}
	}
	if !ok {
		return row, "message_id"
	}
	row.ClientID, ok = sqliteID(values[2])
	if !ok {
		return row, "client_id"
	}
	row.Text, ok = values[3].(string)
	if !ok || len(row.Text) > 1<<20 || !utf8.ValidString(row.Text) {
		return row, "text"
	}
	row.TimestampMS, ok = values[4].(int64)
	if !ok || row.TimestampMS < since || row.TimestampMS >= until {
		return row, "timestamp"
	}
	row.TTL, ok = values[5].(int64)
	if !ok || row.TTL < 0 {
		return row, "ttl"
	}
	row.Type, ok = values[6].(int64)
	if !ok || row.Type < 0 {
		return row, "message_type"
	}
	row.Status, ok = values[7].(int64)
	if !ok || row.Status <= 0 {
		return row, "message_status"
	}
	switch v := values[8].(type) {
	case nil:
	case []byte:
		if len(v) > 256<<10 {
			return row, "metadata"
		}
		row.BinNet = append([]byte(nil), v...)
	case string:
		if len(v) > 256<<10 {
			return row, "metadata"
		}
		row.BinNet = []byte(v)
	default:
		return row, "metadata"
	}
	return row, ""
}

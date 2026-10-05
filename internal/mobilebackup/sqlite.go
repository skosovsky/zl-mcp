package mobilebackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
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

type SQLiteRow struct {
	SenderID, MessageID, ClientID, Text string `json:"-"`
	TimestampMS, TTL, Type, Status      int64  `json:"-"`
	BinNet                              []byte `json:"-"`
}

func (SQLiteRow) String() string   { return "mobile backup SQLite row [redacted]" }
func (SQLiteRow) GoString() string { return "mobile backup SQLite row [redacted]" }

// SQLiteBatch is private candidate data; counts do not establish complete history.
type SQLiteBatch struct {
	SourceControls     int
	Rows               []SQLiteRow `json:"-"`
	Examined, Rejected int
	HasMore            bool
	Next               *SQLiteCursor `json:"-"`
}

// SQLiteCursor is private keyset state bound to exact bytes, name and date filter.
type SQLiteCursor struct {
	Digest                               [32]byte `json:"-"`
	Name                                 string   `json:"-"`
	SinceMS, UntilMS, TimestampMS, RowID int64    `json:"-"`
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
}

func standaloneSQLite(data []byte) bool {
	if len(data) < 512 || uint64(len(data)) > MaxFileBytes || string(data[:16]) != "SQLite format 3\x00" {
		return false
	}
	page := int(binary.BigEndian.Uint16(data[16:18]))
	if page == 1 {
		page = 65536
	}
	if page < 512 || page > 65536 || page&(page-1) != 0 || len(data)%page != 0 || data[18] != 1 || data[19] != 1 || page-int(data[20]) < 480 || !bytes.Equal(data[21:24], []byte{64, 32, 32}) || !bytes.Equal(data[72:92], make([]byte, 20)) {
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

func ReadSQLitePage(parent context.Context, file ArchiveFile, scratch string, since, until time.Time, maxRows int, after *SQLiteCursor) (batch SQLiteBatch, err error) {
	from, to, validWindow := millisecondWindow(since, until)
	if !validWindow || parent == nil || parent.Err() != nil || !validName(file.Name) || !standaloneSQLite(file.Data) || !filepath.IsAbs(scratch) || maxRows < 1 || maxRows > 5000 || since.IsZero() || !until.After(since) {
		return batch, ErrSQLite
	}
	digest := sha256.Sum256(file.Data)
	if after != nil && (after.Digest != digest || after.Name != file.Name || after.SinceMS != from || after.UntilMS != to || after.TimestampMS < from || after.TimestampMS >= to) {
		return batch, ErrSQLite
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	dir, e := os.MkdirTemp(scratch, "mobile-read-")
	if e != nil {
		return batch, ErrSQLite
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "archive.sqlite")
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
	db, e := sql.Open("sqlite", uri.String())
	if e != nil {
		return batch, ErrSQLite
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	conn, e := db.Conn(ctx)
	if e != nil {
		return batch, ErrSQLite
	}
	defer conn.Close()
	for _, limit := range [][2]int{{sqlite3.SQLITE_LIMIT_LENGTH, 1 << 20}, {sqlite3.SQLITE_LIMIT_SQL_LENGTH, 16 << 10}, {sqlite3.SQLITE_LIMIT_COLUMN, 64}, {sqlite3.SQLITE_LIMIT_EXPR_DEPTH, 20}, {sqlite3.SQLITE_LIMIT_ATTACHED, 0}} {
		if _, e = sqlite.Limit(conn, limit[0], limit[1]); e != nil {
			return batch, ErrSQLite
		}
	}
	var check string
	if conn.QueryRowContext(ctx, "PRAGMA integrity_check(1)").Scan(&check) != nil || check != "ok" {
		return batch, ErrSQLite
	}
	var kind string
	var root int64
	if conn.QueryRowContext(ctx, "SELECT type,rootpage FROM sqlite_schema WHERE name='ChatContent'").Scan(&kind, &root) != nil || kind != "table" || root <= 0 {
		return batch, ErrSQLite
	}
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
	// Controls outside the requested interval can still invalidate imported content.
	if conn.QueryRowContext(ctx, "SELECT count(*) FROM ChatContent WHERE MsgType IN (33,36)").Scan(&batch.SourceControls) != nil {
		return SQLiteBatch{}, ErrSQLite
	}
	statement := "SELECT rowid,SenderId,GlbMsgId,CliMsgId,MsgContent,TimeStamp,TTL,MsgType,MsgStatus,BinNet FROM ChatContent WHERE TimeStamp>=? AND TimeStamp<?"
	args := []any{from, to}
	if after != nil {
		statement += " AND (TimeStamp>? OR (TimeStamp=? AND rowid>?))"
		args = append(args, after.TimestampMS, after.TimestampMS, after.RowID)
	}
	statement += " ORDER BY TimeStamp,rowid LIMIT ?"
	args = append(args, maxRows+1)
	rows, e := conn.QueryContext(ctx, statement, args...)
	if e != nil {
		return batch, ErrSQLite
	}
	defer rows.Close()
	// Discard all previously accumulated private rows if query execution fails.
	defer func() {
		if err != nil {
			for i := range batch.Rows {
				clear(batch.Rows[i].BinNet)
			}
			batch = SQLiteBatch{}
		}
	}()
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
		last = SQLiteCursor{Digest: digest, Name: file.Name, SinceMS: from, UntilMS: to, TimestampMS: timestamp, RowID: sourceRowID}
		batch.Examined++
		row, ok := sqliteRow(values, from, to)
		if !ok {
			batch.Rejected++
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
func sqliteRow(values [9]any, since, until int64) (SQLiteRow, bool) {
	var row SQLiteRow
	var ok bool
	row.SenderID, ok = sqliteID(values[0])
	if !ok {
		return row, false
	}
	row.MessageID, ok = sqliteID(values[1])
	if !ok {
		return row, false
	}
	row.ClientID, ok = sqliteID(values[2])
	if !ok {
		return row, false
	}
	row.Text, ok = values[3].(string)
	if !ok || len(row.Text) > 1<<20 || !utf8.ValidString(row.Text) {
		return row, false
	}
	row.TimestampMS, ok = values[4].(int64)
	if !ok || row.TimestampMS < since || row.TimestampMS >= until {
		return row, false
	}
	row.TTL, ok = values[5].(int64)
	if !ok || row.TTL < 0 {
		return row, false
	}
	row.Type, ok = values[6].(int64)
	if !ok || row.Type < 0 {
		return row, false
	}
	row.Status, ok = values[7].(int64)
	if !ok || row.Status <= 0 {
		return row, false
	}
	switch v := values[8].(type) {
	case nil:
	case []byte:
		if len(v) > 256<<10 {
			return row, false
		}
		row.BinNet = append([]byte(nil), v...)
	case string:
		if len(v) > 256<<10 {
			return row, false
		}
		row.BinNet = []byte(v)
	default:
		return row, false
	}
	return row, true
}

package storage

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/skosovsky/zl-mcp/internal/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	DB                              *sql.DB
	allowed                         map[string]bool
	policy                          domain.CollectionPolicy
	retention                       int
	key                             []byte
	directIngestion, groupIngestion ingestionCounters
}

func Open(ctx context.Context, path string, groupIDs []string, retention int) (*Store, error) {
	policy := domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{}}
	for _, id := range groupIDs {
		policy.Selected[domain.ConversationRef{Type: domain.ConversationGroup, ID: id}] = true
	}
	return OpenWithPolicy(ctx, path, policy, retention)
}

func OpenWithPolicy(ctx context.Context, path string, policy domain.CollectionPolicy, retention int) (*Store, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	selected := map[domain.ConversationRef]bool{}
	for ref, enabled := range policy.Selected {
		if !ref.Valid() {
			db.Close()
			return nil, domain.Invalid("Invalid collection reference.")
		}
		selected[ref] = enabled
	}
	policy.Selected = selected
	s := &Store{DB: db, allowed: map[string]bool{}, policy: policy, retention: retention}
	for ref, enabled := range selected {
		if ref.Type == domain.ConversationGroup && enabled {
			s.allowed[ref.ID] = true
		}
	}
	if err = s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	var key string
	err = db.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='cursor_key'").Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		b := make([]byte, 32)
		if _, err = rand.Read(b); err != nil {
			db.Close()
			return nil, err
		}
		key = hex.EncodeToString(b)
		_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO metadata VALUES ('cursor_key',?)", key)
		if err == nil {
			err = db.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='cursor_key'").Scan(&key)
		}
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	s.key, err = hex.DecodeString(key)
	return s, err
}
func (s *Store) Close() error { return s.DB.Close() }
func (s *Store) Allowed(id string) bool {
	return s.AllowsConversation(domain.ConversationRef{Type: domain.ConversationGroup, ID: id})
}
func (s *Store) migrate(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `
 CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY);
 CREATE TABLE IF NOT EXISTS metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS groups(group_id TEXT PRIMARY KEY,name TEXT NOT NULL,description TEXT,member_count INTEGER,updated_at TEXT NOT NULL,membership TEXT NOT NULL DEFAULT 'member');
 CREATE TABLE IF NOT EXISTS messages(seq INTEGER PRIMARY KEY AUTOINCREMENT,account_key TEXT NOT NULL DEFAULT 'local',group_id TEXT NOT NULL,message_id TEXT NOT NULL,sender_id TEXT NOT NULL,sender_name TEXT,sent_at TEXT NOT NULL,received_at TEXT NOT NULL,text TEXT NOT NULL,reply_id TEXT,attachments TEXT NOT NULL,source TEXT NOT NULL,UNIQUE(account_key,group_id,message_id));
 CREATE INDEX IF NOT EXISTS message_order ON messages(group_id,sent_at DESC,message_id);
 CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(text,content='messages',content_rowid='seq',tokenize='unicode61 remove_diacritics 2');
 CREATE TRIGGER IF NOT EXISTS message_insert AFTER INSERT ON messages BEGIN INSERT INTO messages_fts(rowid,text) VALUES(new.seq,new.text); END;
 CREATE TRIGGER IF NOT EXISTS message_delete AFTER DELETE ON messages BEGIN INSERT INTO messages_fts(messages_fts,rowid,text) VALUES('delete',old.seq,old.text); END;
 CREATE TRIGGER IF NOT EXISTS message_update AFTER UPDATE OF text ON messages BEGIN INSERT INTO messages_fts(messages_fts,rowid,text) VALUES('delete',old.seq,old.text);INSERT INTO messages_fts(rowid,text) VALUES(new.seq,new.text); END;
 CREATE TABLE IF NOT EXISTS collection_gaps(id INTEGER PRIMARY KEY,group_id TEXT NOT NULL,started_at TEXT NOT NULL,ended_at TEXT,reason TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS collection_started(group_id TEXT PRIMARY KEY,started_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS read_budget(id INTEGER PRIMARY KEY CHECK(id=1),tokens REAL NOT NULL,last_ns INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS collector_state(id INTEGER PRIMARY KEY CHECK(id=1),payload TEXT NOT NULL,heartbeat TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS join_previews(id TEXT PRIMARY KEY,payload TEXT NOT NULL,expires_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS join_approvals(token_hash TEXT PRIMARY KEY,preview_id TEXT NOT NULL,expires_at TEXT NOT NULL,request_id TEXT);
 CREATE TABLE IF NOT EXISTS join_requests(request_id TEXT PRIMARY KEY,operation_id TEXT UNIQUE NOT NULL,token_hash TEXT NOT NULL,payload TEXT NOT NULL,created_at TEXT NOT NULL);
 INSERT OR IGNORE INTO schema_migrations VALUES(1);`)
	if err != nil {
		return err
	}
	if err = s.migrateEvents(ctx); err != nil {
		return err
	}
	if err = s.migrateConversations(ctx); err != nil {
		return err
	}
	if err = s.migrateSending(ctx); err != nil {
		return err
	}
	if err = s.migrateIncoming(ctx); err != nil {
		return err
	}
	if err = s.migrateContacts(ctx); err != nil {
		return err
	}
	if err = s.migrateHistoryOperations(ctx); err != nil {
		return err
	}
	if err = s.migrateMobileBackup(ctx); err != nil {
		return err
	}
	if err = s.migrateHistoryExpiry(ctx); err != nil {
		return err
	}
	if err = s.migrateTombstones(ctx); err != nil {
		return err
	}
	return s.migrateEventJournal(ctx)
}
func (s *Store) BindAccount(ctx context.Context, account string) error {
	h := sha256.Sum256([]byte(account))
	value := hex.EncodeToString(h[:])
	if _, err := s.DB.ExecContext(ctx, "INSERT OR IGNORE INTO metadata VALUES ('account',?)", value); err != nil {
		return err
	}
	var existing string
	if err := s.DB.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='account'").Scan(&existing); err != nil {
		return err
	}
	if existing != value {
		return fmt.Errorf("state directory belongs to another Zalo account; use a separate state directory")
	}
	return nil
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func (s *Store) UpsertGroup(ctx context.Context, g domain.Group, description *string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := now()
	if _, err = tx.ExecContext(ctx, `INSERT INTO groups(group_id,name,description,member_count,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(group_id) DO UPDATE SET name=excluded.name,description=excluded.description,member_count=excluded.member_count,updated_at=excluded.updated_at,membership='member'`, g.ID, g.Name, description, g.MemberCount, at); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversations(conversation_type,conversation_id,name,metadata_source,availability,first_discovered_at,updated_at) VALUES('group',?,?,'group_catalog','member',?,?) ON CONFLICT(conversation_type,conversation_id) DO UPDATE SET name=excluded.name,metadata_source='group_catalog',availability='member',updated_at=excluded.updated_at`, g.ID, g.Name, at, at); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Put(ctx context.Context, m domain.Message) (err error) {
	ref := m.Ref()
	if !ref.Valid() {
		return domain.Invalid("Typed conversation identity is required.")
	}
	counters := &s.groupIngestion
	if ref.Type == domain.ConversationDirect {
		counters = &s.directIngestion
	}
	counters.received.Add(1)
	defer func() {
		if err != nil {
			counters.errors.Add(1)
		}
	}()
	if !s.AllowsConversation(ref) {
		counters.excluded.Add(1)
		return nil
	}
	if m.ID == "" || m.SenderID == "" || m.SentAt.IsZero() {
		return domain.Invalid("Message ID, sender ID and timestamp are required.")
	}
	if len(m.Text) > 1<<20 {
		return fmt.Errorf("message exceeds 1 MiB text limit")
	}
	if m.Source != "live" && m.Source != "replay" {
		return domain.Invalid("Message source must be live or replay.")
	}
	if m.AttachmentTypes == nil {
		m.AttachmentTypes = []string{}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	inserted, err := s.putMessageTx(ctx, tx, m, false)
	if err != nil {
		return err
	}
	err = tx.Commit()
	if err == nil {
		if inserted {
			counters.inserted.Add(1)
		} else {
			counters.duplicates.Add(1)
		}
	}
	return err
}

// putMessageTx is shared by ordinary collection and silent historical pages.
// The caller validates the complete request and owns the transaction/checkpoint.
func (s *Store) putMessageTx(ctx context.Context, tx *sql.Tx, m domain.Message, historical bool) (bool, error) {
	ref := m.Ref()
	var deleted int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM message_tombstones WHERE conversation_type=? AND conversation_id=? AND message_id=?", ref.Type, ref.ID, m.ID).Scan(&deleted); err != nil {
		return false, err
	}
	if deleted > 0 {
		return false, nil
	}
	var expired int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM history_message_expiry WHERE conversation_type=? AND conversation_id=? AND message_id=? AND (expired=1 OR expires_ms<=?)`, ref.Type, ref.ID, m.ID, time.Now().UnixMilli()).Scan(&expired); err != nil {
		return false, err
	}
	if expired > 0 {
		return false, nil
	}
	a, err := json.Marshal(m.AttachmentTypes)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO messages(group_id,conversation_type,message_id,sender_id,sender_name,sent_at,received_at,text,reply_id,attachments,source) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, ref.ID, ref.Type, m.ID, m.SenderID, m.SenderName, m.SentAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), now(), m.Text, m.ReplyTo, string(a), m.Source)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if inserted != 0 {
		seq, err := result.LastInsertId()
		if err != nil {
			return false, err
		}
		if historical {
			err = appendHistoryIdentity(ctx, tx, seq, m)
		} else {
			err = appendMessageEvent(ctx, tx, seq, m)
		}
		if err != nil {
			return false, err
		}
	}
	// A matching replay may restore missing protocol quote identifiers without
	// changing retained text, permanent identity or event eligibility.
	if m.QuoteMetadata != nil && ref.Type == domain.ConversationDirect {
		q := m.QuoteMetadata
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO message_quote_metadata(message_seq,client_message_id,message_type,timestamp,ttl) SELECT seq,?,?,?,? FROM messages WHERE conversation_type='direct' AND group_id=? AND message_id=? AND sender_id=? AND text=? AND sent_at=?`, q.ClientMessageID, q.MessageType, q.Timestamp, q.TTL, ref.ID, m.ID, m.SenderID, m.Text, m.SentAt.UTC().Format("2006-01-02T15:04:05.000000000Z")); err != nil {
			return false, err
		}
	}
	if !historical {
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO collection_started(group_id,conversation_type,started_at) VALUES(?,?,?)`, ref.ID, ref.Type, now())
		if err != nil {
			return false, err
		}
	}
	var peerName *string
	if ref.Type == domain.ConversationDirect && m.SenderID == ref.ID {
		peerName = m.SenderName
	}
	metadataSource := m.Source
	if historical {
		metadataSource = "stored_message"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversations(conversation_type,conversation_id,name,metadata_source,availability,first_discovered_at,updated_at) VALUES(?,?,?,?,'observed',?,?) ON CONFLICT(conversation_type,conversation_id) DO UPDATE SET name=COALESCE(excluded.name,conversations.name),availability=CASE WHEN excluded.conversation_type='direct' THEN 'observed' ELSE conversations.availability END,updated_at=excluded.updated_at`, ref.Type, ref.ID, peerName, metadataSource, now(), now()); err != nil {
		return false, err
	}
	return inserted != 0, nil
}
func (s *Store) Delete(ctx context.Context, groupID, messageID string) error {
	return s.DeleteConversation(ctx, domain.ConversationRef{Type: domain.ConversationGroup, ID: groupID}, messageID)
}
func (s *Store) Retain(ctx context.Context) error {
	if err := s.ExpireHistoryAt(ctx, time.Now()); err != nil {
		return err
	}
	if s.retention == 0 {
		return nil
	}
	cut := time.Now().AddDate(0, 0, -s.retention).UTC().Format("2006-01-02T15:04:05.000000000Z")
	return s.removeMessages(ctx, "sent_at < ?", "retention", cut)
}

// predicate is selected only by Delete/Retain, never supplied by a client.
func (s *Store) removeMessages(ctx context.Context, predicate, reason string, args ...any) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.removeMessagesTx(ctx, tx, predicate, reason, args...); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) removeMessagesTx(ctx context.Context, tx *sql.Tx, predicate, reason string, args ...any) error {
	selection := "SELECT seq FROM messages WHERE " + predicate
	values := append([]any{now(), reason}, args...)
	values = append(values, args...)
	_, err := tx.ExecContext(ctx, `UPDATE event_deliveries SET payload=X'',state=CASE WHEN state IN ('pending','sending') THEN 'cancelled' ELSE state END,completed_at=COALESCE(completed_at,?),lease_until=NULL,last_reason=? WHERE message_seq IN (`+selection+`) OR event_id IN (SELECT event_id FROM message_events WHERE seq IN (`+selection+`))`, values...)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM message_events WHERE seq IN ("+selection+")", args...); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM messages WHERE "+predicate, args...); err != nil {
		return err
	}
	return nil
}

const messageColumns = "group_id,message_id,sender_id,sender_name,sent_at,text,reply_id,attachments,source"

func scanMessage(row interface{ Scan(...any) error }) (domain.Message, error) {
	var m domain.Message
	var ts, a string
	err := row.Scan(&m.GroupID, &m.ID, &m.SenderID, &m.SenderName, &ts, &m.Text, &m.ReplyTo, &a, &m.Source)
	if err != nil {
		return m, err
	}
	m.SentAt, err = time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return m, err
	}
	err = json.Unmarshal([]byte(a), &m.AttachmentTypes)
	return m, err
}
func (s *Store) Message(ctx context.Context, g, id string) (domain.Message, error) {
	if !s.Allowed(g) {
		return domain.Message{}, &domain.Error{Code: "PERMISSION_DENIED", Message: "Group is outside the configured collection allowlist.", NextAction: domain.NextAction{Instruction: "Update the local collection allowlist if access is intended."}, Details: map[string]any{}}
	}
	return scanMessage(s.DB.QueryRowContext(ctx, "SELECT "+messageColumns+" FROM visible_messages WHERE conversation_type='group' AND group_id=? AND message_id=?", g, id))
}

type Search struct {
	General          bool   `json:"general,omitempty"`
	ConversationType string `json:"conversation_type,omitempty"`
	ConversationID   string `json:"conversation_id,omitempty"`
	Query            string `json:"query"`
	GroupID          string `json:"group_id,omitempty"`
	SenderID         string `json:"sender_id,omitempty"`
	Since            string `json:"since,omitempty"`
	Until            string `json:"until,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	Cursor           string `json:"cursor,omitempty"`
}
type cursor struct {
	Kind        string `json:"k,omitempty"`
	Fingerprint string `json:"f"`
	Snapshot    int64  `json:"s"`
	At          string `json:"a"`
	Group       string `json:"g"`
	ID          string `json:"i"`
	SnapshotAt  string `json:"t"`
}

func (s *Store) EncodeCursor(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := hmac.New(sha256.New, s.key)
	h.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}
func (s *Store) DecodeCursor(token string, v any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return domain.Invalid("Invalid continuation cursor.")
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return domain.Invalid("Invalid continuation cursor.")
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return domain.Invalid("Invalid continuation cursor.")
	}
	h := hmac.New(sha256.New, s.key)
	h.Write(b)
	if !hmac.Equal(sig, h.Sum(nil)) {
		return domain.Invalid("Invalid continuation cursor.")
	}
	if json.Unmarshal(b, v) != nil {
		return domain.Invalid("Invalid continuation cursor.")
	}
	return nil
}
func fingerprint(q Search) string {
	q.Cursor = ""
	q.Limit = 0
	b, _ := json.Marshal(q)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func searchText(q string) string {
	words := strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsMark(r) })
	for i, w := range words {
		words[i] = "\"" + strings.ReplaceAll(w, "\"", "\"\"") + "\""
	}
	return strings.Join(words, " AND ")
}
func (s *Store) searchPage(ctx context.Context, q Search) ([]domain.SearchHit, *string, string, cursor, error) {
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.Limit < 1 || q.Limit > 50 {
		return nil, nil, "", cursor{}, domain.Invalid("limit must be an integer between 1 and 50.")
	}
	query := searchText(q.Query)
	if query == "" {
		return nil, nil, "", cursor{}, domain.Invalid("query must contain at least one word.")
	}
	if q.GroupID != "" && !s.Allowed(q.GroupID) {
		return nil, nil, "", cursor{}, domain.Invalid("Selected group is outside the collection allowlist.")
	}
	var since, until time.Time
	var err error
	if q.Since != "" {
		since, err = time.Parse(time.RFC3339, q.Since)
		if err != nil {
			return nil, nil, "", cursor{}, domain.Invalid("since must use RFC3339, for example 2026-10-01T00:00:00Z.")
		}
	}
	if q.Until != "" {
		until, err = time.Parse(time.RFC3339, q.Until)
		if err != nil {
			return nil, nil, "", cursor{}, domain.Invalid("until must use RFC3339, for example 2026-10-01T00:00:00Z.")
		}
	}
	if !since.IsZero() && !until.IsZero() && !since.Before(until) {
		return nil, nil, "", cursor{}, domain.Invalid("since must precede until.")
	}
	c := cursor{Fingerprint: fingerprint(q), SnapshotAt: now()}
	if q.Cursor != "" {
		if err = s.DecodeCursor(q.Cursor, &c); err != nil {
			return nil, nil, "", cursor{}, err
		}
		if c.Fingerprint != fingerprint(q) {
			return nil, nil, "", cursor{}, domain.Invalid("Cursor filters differ from the original query.")
		}
	} else {
		if err = s.DB.QueryRowContext(ctx, "SELECT coalesce(max(seq),0) FROM messages").Scan(&c.Snapshot); err != nil {
			return nil, nil, "", cursor{}, err
		}
	}
	clauses := []string{"messages_fts MATCH ?", "m.seq<=?"}
	if !q.General {
		clauses = append(clauses, "m.conversation_type='group'")
	}
	if q.General && ((q.ConversationType != "" && q.ConversationType != "group" && q.ConversationType != "direct") || (q.ConversationID != "" && q.ConversationType == "")) {
		return nil, nil, "", cursor{}, domain.Invalid("A concrete conversation requires its type.")
	}
	if q.General && q.ConversationID != "" && !s.AllowsConversation(domain.ConversationRef{Type: q.ConversationType, ID: q.ConversationID}) {
		return nil, nil, "", cursor{}, subscriptionPermission("Conversation is outside the collection policy.")
	}

	args := []any{query, c.Snapshot}
	if !s.policy.All {
		permitted := []string{}
		for ref, enabled := range s.policy.Selected {
			if enabled {
				permitted = append(permitted, "(m.conversation_type=? AND m.conversation_id=?)")
				args = append(args, ref.Type, ref.ID)
			}
		}
		if len(permitted) == 0 {
			return []domain.SearchHit{}, nil, c.SnapshotAt, c, nil
		}
		clauses = append(clauses, "("+strings.Join(permitted, " OR ")+")")
	}
	if q.General && q.ConversationType != "" {
		clauses = append(clauses, "m.conversation_type=?")
		args = append(args, q.ConversationType)
	}
	if q.General && q.ConversationID != "" {
		clauses = append(clauses, "m.conversation_id=?")
		args = append(args, q.ConversationID)
	}
	if q.GroupID != "" {
		clauses = append(clauses, "m.group_id=?")
		args = append(args, q.GroupID)
	}
	if q.SenderID != "" {
		clauses = append(clauses, "m.sender_id=?")
		args = append(args, q.SenderID)
	}
	if !since.IsZero() {
		clauses = append(clauses, "m.sent_at>=?")
		args = append(args, since.UTC().Format("2006-01-02T15:04:05.000000000Z"))
	}
	if !until.IsZero() {
		clauses = append(clauses, "m.sent_at<?")
		args = append(args, until.UTC().Format("2006-01-02T15:04:05.000000000Z"))
	}
	if c.At != "" {
		clauses = append(clauses, "(m.sent_at<? OR (m.sent_at=? AND (m.conversation_type>? OR (m.conversation_type=? AND (m.group_id>? OR (m.group_id=? AND m.message_id>?))))))")
		kind := c.Kind
		if kind == "" {
			kind = domain.ConversationGroup
		}
		args = append(args, c.At, c.At, kind, kind, c.Group, c.Group, c.ID)
	}
	args = append(args, q.Limit+1)
	rows, err := s.DB.QueryContext(ctx, `SELECT m.conversation_type,m.group_id,m.message_id,m.sender_id,g.name,m.sender_name,m.sent_at,m.text FROM visible_messages m JOIN messages_fts ON messages_fts.rowid=m.seq LEFT JOIN conversations g ON g.conversation_type=m.conversation_type AND g.conversation_id=m.group_id WHERE `+strings.Join(clauses, " AND ")+` ORDER BY m.sent_at DESC,m.conversation_type,m.group_id,m.message_id LIMIT ?`, args...)
	if err != nil {
		return nil, nil, "", cursor{}, err
	}
	hits := []domain.SearchHit{}
	times := []string{}
	for rows.Next() {
		var h domain.SearchHit
		var ts, txt string
		if err = rows.Scan(&h.Conversation.Type, &h.GroupID, &h.ID, &h.SenderID, &h.GroupName, &h.SenderName, &ts, &txt); err != nil {
			rows.Close()
			return nil, nil, "", cursor{}, err
		}
		h.Conversation.ID = h.GroupID
		h.SentAt, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			rows.Close()
			return nil, nil, "", cursor{}, err
		}
		r := []rune(txt)
		h.TextTruncated = len(r) > 150
		if h.TextTruncated {
			r = r[:150]
		}
		h.Excerpt = string(r)
		hits = append(hits, h)
		times = append(times, ts)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, "", cursor{}, err
	}
	var next *string
	if len(hits) > q.Limit {
		hits = hits[:q.Limit]
		last := hits[len(hits)-1]
		c.At = times[q.Limit-1]
		c.Group = last.Conversation.ID
		c.Kind = last.Conversation.Type
		c.ID = last.ID
		v, e := s.EncodeCursor(c)
		if e != nil {
			return nil, nil, "", cursor{}, e
		}
		next = &v
	}
	return hits, next, c.SnapshotAt, c, nil
}

// SearchPage keeps the original snapshot when a transport must shorten a page.
type SearchPage struct {
	Hits         []domain.SearchHit
	NextCursor   *string
	SnapshotAt   string
	continuation cursor
}

func (s *Store) Page(ctx context.Context, q Search) (*SearchPage, error) {
	hits, next, at, continuation, err := s.searchPage(ctx, q)
	if err != nil {
		return nil, err
	}
	return &SearchPage{Hits: hits, NextCursor: next, SnapshotAt: at, continuation: continuation}, nil
}
func (s *Store) Search(ctx context.Context, q Search) ([]domain.SearchHit, *string, string, error) {
	p, err := s.Page(ctx, q)
	if err != nil {
		return nil, nil, "", err
	}
	return p.Hits, p.NextCursor, p.SnapshotAt, nil
}
func (s *Store) ShortenPage(p *SearchPage, n int) error {
	if n < 1 || n >= len(p.Hits) {
		return domain.Invalid("Invalid page boundary.")
	}
	last := p.Hits[n-1]
	c := p.continuation
	c.At = last.SentAt.UTC().Format("2006-01-02T15:04:05.000000000Z")
	c.Group = last.Conversation.ID
	c.Kind = last.Conversation.Type
	c.ID = last.ID
	token, err := s.EncodeCursor(c)
	if err != nil {
		return err
	}
	p.Hits = p.Hits[:n]
	p.NextCursor = &token
	p.continuation = c
	return nil
}

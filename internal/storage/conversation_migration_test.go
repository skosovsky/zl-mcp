package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func legacyConversationFixture(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "legacy.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`
 CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY);
 INSERT INTO schema_migrations VALUES(1),(2),(3);
 CREATE TABLE groups(group_id TEXT PRIMARY KEY,name TEXT NOT NULL,membership TEXT,updated_at TEXT);
 INSERT INTO groups VALUES('same','Synthetic group','member','2026-10-01T00:00:00Z');
 CREATE TABLE messages(seq INTEGER PRIMARY KEY AUTOINCREMENT,account_key TEXT NOT NULL DEFAULT 'local',group_id TEXT NOT NULL,message_id TEXT NOT NULL,sender_id TEXT NOT NULL,sender_name TEXT,sent_at TEXT NOT NULL,received_at TEXT NOT NULL,text TEXT NOT NULL,reply_id TEXT,attachments TEXT NOT NULL,source TEXT NOT NULL,UNIQUE(account_key,group_id,message_id));
 INSERT INTO messages VALUES(42,'local','same','m','sender',NULL,'2026-10-01T00:00:00Z','2026-10-01T00:00:01Z','synthetic',NULL,'[]','live');
 UPDATE sqlite_sequence SET seq=75 WHERE name='messages';
 CREATE TABLE message_identities(group_id TEXT,message_id TEXT,first_seq INTEGER,PRIMARY KEY(group_id,message_id));
 INSERT INTO message_identities VALUES('same','m',42);
 CREATE TABLE message_events(seq INTEGER PRIMARY KEY,event_id TEXT,group_id TEXT,message_json TEXT,created_at TEXT);
 INSERT INTO message_events VALUES(42,'evt_synthetic','same','{}','2026-10-01T00:00:01Z');
 CREATE TABLE event_subscriptions(id TEXT PRIMARY KEY,group_id TEXT,start_seq INTEGER,generation TEXT);
 INSERT INTO event_subscriptions VALUES('sub','same',75,'gen');
 CREATE TABLE event_deliveries(id INTEGER PRIMARY KEY,payload BLOB,state TEXT);
 INSERT INTO event_deliveries VALUES(1,X'010203','pending');
 CREATE TABLE event_fanout(id INTEGER PRIMARY KEY,seq INTEGER);
 INSERT INTO event_fanout VALUES(1,42);
 CREATE TABLE collection_started(group_id TEXT PRIMARY KEY,started_at TEXT);
 INSERT INTO collection_started VALUES('same','2026-10-01T00:00:00Z');
 CREATE TABLE collection_gaps(id INTEGER PRIMARY KEY,group_id TEXT,started_at TEXT,ended_at TEXT,reason TEXT);
 INSERT INTO collection_gaps VALUES(1,'same','2026-10-01T00:00:00Z',NULL,'restart');
 `)
	if err != nil {
		t.Fatal(err)
	}
	return &Store{DB: db}
}

func TestConversationMigrationPreservesBoundariesPayloadAndFTS(t *testing.T) {
	// Arrange
	s := legacyConversationFixture(t)
	ctx := context.Background()
	// Act
	if err := s.migrateConversations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateConversations(ctx); err != nil {
		t.Fatal(err)
	}
	// Assert
	var seq, boundary, watermark, fts int
	var kind, id, profile, generation, state string
	var payload []byte
	if err := s.DB.QueryRow("SELECT seq,conversation_type,conversation_id FROM messages").Scan(&seq, &kind, &id); err != nil {
		t.Fatal(err)
	}
	if seq != 42 || kind != "group" || id != "same" {
		t.Fatal("legacy message changed identity")
	}
	if err := s.DB.QueryRow("SELECT start_seq,profile,generation FROM event_subscriptions").Scan(&boundary, &profile, &generation); err != nil {
		t.Fatal(err)
	}
	if boundary != 75 || profile != "zalo.message.created" || generation != "gen" {
		t.Fatal("subscription changed")
	}
	if err := s.DB.QueryRow("SELECT seq FROM event_fanout WHERE id=1").Scan(&watermark); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow("SELECT payload,state FROM event_deliveries WHERE id=1").Scan(&payload, &state); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow("SELECT count(*) FROM messages_fts WHERE messages_fts MATCH 'synthetic'").Scan(&fts); err != nil {
		t.Fatal(err)
	}
	if watermark != 42 || string(payload) != "\x01\x02\x03" || state != "pending" || fts != 1 {
		t.Fatal("queue, watermark or FTS changed")
	}
	_, err := s.DB.Exec("INSERT INTO messages(group_id,conversation_type,message_id,sender_id,sent_at,received_at,text,attachments,source) VALUES('same','direct','m','sender','2026-10-03T00:00:00Z','2026-10-03T00:00:00Z','direct synthetic','[]','live')")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow("SELECT seq FROM messages WHERE conversation_type='direct'").Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if seq != 76 {
		t.Fatalf("deleted message sequence boundary lost: %d", seq)
	}
}

func TestConversationMigrationRollsBackPartialDDL(t *testing.T) {
	// Arrange: inject a collision late in migration to exercise atomic rollback.
	s := legacyConversationFixture(t)
	if _, err := s.DB.Exec("CREATE TABLE conversations(dummy TEXT)"); err != nil {
		t.Fatal(err)
	}
	// Act
	err := s.migrateConversations(context.Background())
	// Assert
	if err == nil {
		t.Fatal("injected failure did not occur")
	}
	var count int
	if err := s.DB.QueryRow("SELECT count(*) FROM schema_migrations WHERE version=4").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration marked complete")
	}
	if _, err = s.DB.Exec("SELECT conversation_type FROM messages"); err == nil {
		t.Fatal("partial table rebuild survived rollback")
	}
	if err := s.DB.QueryRow("SELECT count(*) FROM messages WHERE seq=42").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("legacy message lost during rollback")
	}
}

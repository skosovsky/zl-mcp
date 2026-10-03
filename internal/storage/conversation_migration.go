package storage

import "context"

// migrateConversations preserves legacy physical group_id columns as ID aliases.
// The generated conversation_id is the canonical read column; namespaces and
// uniqueness include conversation_type. Existing SQL writers default to group.
func (s *Store) migrateConversations(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=4").Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return tx.Commit()
	}
	// Deleted rows still contribute to the subscription activation boundary.
	// Rebuilding the table must not reset its AUTOINCREMENT high-water mark.
	var highWater int64
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name='messages'),0)").Scan(&highWater); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
DROP TRIGGER IF EXISTS message_insert;
DROP TRIGGER IF EXISTS message_delete;
DROP TRIGGER IF EXISTS message_update;
DROP TABLE IF EXISTS messages_fts;
CREATE TABLE messages_v4(
 seq INTEGER PRIMARY KEY AUTOINCREMENT, account_key TEXT NOT NULL DEFAULT 'local',
 group_id TEXT NOT NULL, conversation_type TEXT NOT NULL DEFAULT 'group' CHECK(conversation_type IN ('direct','group')),
 conversation_id TEXT GENERATED ALWAYS AS (group_id) VIRTUAL,
 message_id TEXT NOT NULL,sender_id TEXT NOT NULL,sender_name TEXT,sent_at TEXT NOT NULL,received_at TEXT NOT NULL,text TEXT NOT NULL,reply_id TEXT,attachments TEXT NOT NULL,source TEXT NOT NULL,
 UNIQUE(account_key,conversation_type,conversation_id,message_id));
INSERT INTO messages_v4(seq,account_key,group_id,message_id,sender_id,sender_name,sent_at,received_at,text,reply_id,attachments,source)
 SELECT seq,account_key,group_id,message_id,sender_id,sender_name,sent_at,received_at,text,reply_id,attachments,source FROM messages;
DROP TABLE messages;
ALTER TABLE messages_v4 RENAME TO messages;
CREATE INDEX message_order ON messages(conversation_type,conversation_id,sent_at DESC,message_id);
CREATE VIRTUAL TABLE messages_fts USING fts5(text,content='messages',content_rowid='seq',tokenize='unicode61 remove_diacritics 2');
INSERT INTO messages_fts(messages_fts) VALUES('rebuild');
CREATE TRIGGER message_insert AFTER INSERT ON messages BEGIN INSERT INTO messages_fts(rowid,text) VALUES(new.seq,new.text); END;
CREATE TRIGGER message_delete AFTER DELETE ON messages BEGIN INSERT INTO messages_fts(messages_fts,rowid,text) VALUES('delete',old.seq,old.text); END;
CREATE TRIGGER message_update AFTER UPDATE OF text ON messages BEGIN INSERT INTO messages_fts(messages_fts,rowid,text) VALUES('delete',old.seq,old.text);INSERT INTO messages_fts(rowid,text) VALUES(new.seq,new.text); END;
CREATE TABLE message_identities_v4(group_id TEXT NOT NULL,message_id TEXT NOT NULL,first_seq INTEGER NOT NULL,conversation_type TEXT NOT NULL DEFAULT 'group' CHECK(conversation_type IN ('direct','group')),conversation_id TEXT GENERATED ALWAYS AS (group_id) VIRTUAL,PRIMARY KEY(conversation_type,group_id,message_id));
INSERT INTO message_identities_v4(group_id,message_id,first_seq) SELECT group_id,message_id,first_seq FROM message_identities;
DROP TABLE message_identities;
ALTER TABLE message_identities_v4 RENAME TO message_identities;
ALTER TABLE message_events ADD COLUMN conversation_type TEXT NOT NULL DEFAULT 'group' CHECK(conversation_type IN ('direct','group'));
ALTER TABLE message_events ADD COLUMN conversation_id TEXT GENERATED ALWAYS AS (group_id) VIRTUAL;
CREATE INDEX message_events_conversation ON message_events(conversation_type,conversation_id,seq);
ALTER TABLE event_subscriptions ADD COLUMN profile TEXT NOT NULL DEFAULT 'zalo.message.created';
ALTER TABLE event_subscriptions ADD COLUMN scope TEXT NOT NULL DEFAULT 'conversation';
ALTER TABLE event_subscriptions ADD COLUMN conversation_type TEXT NOT NULL DEFAULT 'group';
ALTER TABLE event_subscriptions ADD COLUMN conversation_id TEXT GENERATED ALWAYS AS (group_id) VIRTUAL;
CREATE TABLE collection_started_v4(group_id TEXT NOT NULL,started_at TEXT NOT NULL,conversation_type TEXT NOT NULL DEFAULT 'group' CHECK(conversation_type IN ('direct','group')),conversation_id TEXT GENERATED ALWAYS AS (group_id) VIRTUAL,PRIMARY KEY(conversation_type,group_id));
INSERT INTO collection_started_v4(group_id,started_at) SELECT group_id,started_at FROM collection_started;
DROP TABLE collection_started;
ALTER TABLE collection_started_v4 RENAME TO collection_started;
ALTER TABLE collection_gaps ADD COLUMN conversation_type TEXT NOT NULL DEFAULT 'group';
ALTER TABLE collection_gaps ADD COLUMN conversation_id TEXT GENERATED ALWAYS AS (group_id) VIRTUAL;
CREATE INDEX collection_gap_conversation ON collection_gaps(conversation_type,conversation_id,started_at);
CREATE TABLE conversations(conversation_type TEXT NOT NULL CHECK(conversation_type IN ('direct','group')),conversation_id TEXT NOT NULL,name TEXT,metadata_source TEXT NOT NULL,availability TEXT NOT NULL,first_discovered_at TEXT NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(conversation_type,conversation_id));
INSERT INTO conversations SELECT 'group',group_id,name,'group_catalog',membership,updated_at,updated_at FROM groups;
INSERT OR IGNORE INTO conversations SELECT 'group',group_id,NULL,'stored_message','unknown',min(received_at),max(received_at) FROM messages GROUP BY group_id;
INSERT INTO schema_migrations VALUES(4);
`)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE sqlite_sequence SET seq=MAX(seq,?) WHERE name='messages'", highWater); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO sqlite_sequence(name,seq) SELECT 'messages',? WHERE NOT EXISTS(SELECT 1 FROM sqlite_sequence WHERE name='messages')", highWater); err != nil {
		return err
	}
	return tx.Commit()
}

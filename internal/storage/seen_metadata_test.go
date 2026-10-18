package storage

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func seenTestMessage() domain.Message {
	m := clientAliasMessage("direct", "peer", "123", "456")
	m.Direction = "incoming"
	m.SenderID = "peer"
	m.SeenMetadata = &domain.SeenMetadata{
		ClientMessageID: "456",
		RecipientID:     "owner",
		MessageType:     "webchat",
		Timestamp:       "1791028800000",
	}
	m.SentAt = time.UnixMilli(1791028800000).UTC()
	return m
}

func TestSeenProvenanceRequiresExactRetainedIdentityAndDoesNotBackfill(t *testing.T) {
	// Arrange: preexisting record has no counter provenance.
	s := seenTargetStore(t)
	m := seenTestMessage()
	native := m.SeenMetadata
	m.SeenMetadata = nil
	if err := s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	// Act / Assert: migration never fabricates values for existing messages.
	if err := s.migrateSeenMetadata(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SeenTarget(t.Context(), m.Ref(), m.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("old record gained write authority", err)
	}
	// A replay with another sender cannot bind private fields to this record.
	wrong := m
	wrong.SenderID = "other"
	wrong.SeenMetadata = native
	if err := s.Put(t.Context(), wrong); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SeenTarget(t.Context(), m.Ref(), m.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("foreign sender bound provenance")
	}
	m.SeenMetadata = native
	if err := s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	got, err := s.SeenTarget(t.Context(), m.Ref(), m.ID)
	if err != nil || got.SenderID != "peer" || got.Metadata != *native {
		t.Fatal("matching replay did not restore provenance", err)
	}
	// Immutable fields cannot be replaced by later replay.
	changed := *native
	changed.CMD = 501
	m.SeenMetadata = &changed
	if err = s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	got, err = s.SeenTarget(t.Context(), m.Ref(), m.ID)
	if err != nil || got.Metadata.CMD != 0 {
		t.Fatal("replay overwrote provenance")
	}
	var events int
	if err = s.DB.QueryRow(`SELECT count(*) FROM message_events`).Scan(&events); err != nil ||
		events != 1 {
		t.Fatal("provenance fill emitted an event")
	}
	if _, err = s.SeenTarget(t.Context(), domain.ConversationRef{Type: "group", ID: "peer"}, m.ID); !errors.Is(
		err,
		sql.ErrNoRows,
	) {
		t.Fatal("typed namespace crossed")
	}
	// Act / Assert: normal recall cascades private metadata and blocks resurrection.
	if err = s.DeleteConversation(t.Context(), m.Ref(), m.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.DB.QueryRow(`SELECT count(*) FROM message_seen_metadata`).Scan(&n); err != nil ||
		n != 0 {
		t.Fatal("recalled provenance survived")
	}
}

func TestSeenMetadataDirection(t *testing.T) {
	for _, tc := range []struct{ kind, direction string }{
		{"direct", "incoming"}, {"direct", "outgoing"},
		{"group", "incoming"}, {"group", "outgoing"},
	} {
		t.Run(tc.kind+tc.direction, func(t *testing.T) {
			// Arrange.
			s := seenTargetStore(t)
			m := seenTestMessage()
			m.Conversation.Type = tc.kind
			m.Direction = tc.direction
			if tc.kind == "group" {
				m.SeenMetadata.RecipientID = "peer"
			}
			// Act.
			err := s.Put(t.Context(), m)
			_, lookup := s.SeenTarget(t.Context(), m.Ref(), m.ID)
			// Assert: never guess incoming from identity alone.
			if err != nil {
				t.Fatal(err)
			}
			if tc.direction == "incoming" && lookup != nil {
				t.Fatal(lookup)
			}
			if tc.direction == "outgoing" && !errors.Is(lookup, sql.ErrNoRows) {
				t.Fatal("outgoing receipt permitted")
			}
		})
	}
}

func TestSeenMetadataInvalidProvenance(t *testing.T) {
	for _, mutate := range []func(*domain.Message){
		func(m *domain.Message) { m.SeenMetadata.Timestamp = "1" },
		func(m *domain.Message) { m.SeenMetadata.ClientMessageID = "999" },
		func(m *domain.Message) { m.SeenMetadata.MessageType = "" },
		func(m *domain.Message) { m.Direction = "unknown" },
	} {
		// Arrange.
		s := seenTargetStore(t)
		m := seenTestMessage()
		mutate(&m)
		// Act.
		err := s.Put(t.Context(), m)
		// Assert: invalid evidence rolls back the message and event atomically.
		var n int
		if err == nil {
			t.Fatal("invalid provenance accepted")
		}
		if err = s.DB.QueryRow(`SELECT count(*) FROM messages`).Scan(&n); err != nil || n != 0 {
			t.Fatal("partial write survived")
		}
	}
}

func TestSeenMetadataMigrationIsAtomic(t *testing.T) {
	// Arrange.
	s := seenTargetStore(t)
	if _, err := s.DB.Exec(`DROP TABLE message_seen_metadata;DELETE FROM schema_migrations WHERE version=18;
 CREATE TRIGGER reject_seen_migration BEFORE INSERT ON schema_migrations WHEN NEW.version=18 BEGIN SELECT RAISE(ABORT,'synthetic');END;`); err != nil {
		t.Fatal(err)
	}
	// Act.
	err := s.migrateSeenMetadata(t.Context())
	// Assert.
	var n int
	if err == nil {
		t.Fatal("failed marker ignored")
	}
	if err = s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='message_seen_metadata'`).
		Scan(&n); err != nil ||
		n != 0 {
		t.Fatal("half migration survived")
	}
	if _, err = s.DB.Exec(`DROP TRIGGER reject_seen_migration`); err != nil {
		t.Fatal(err)
	}
	if err = s.migrateSeenMetadata(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSeenMetadataFollowsExpiryAndRetention(t *testing.T) {
	for _, removal := range []string{"expiry", "retention"} {
		t.Run(removal, func(t *testing.T) {
			assertSeenMetadataRemoval(t, removal)
		})
	}
}

func assertSeenMetadataRemoval(t *testing.T, removal string) {
	t.Helper()
	// Arrange: a synthetic retained record with private receipt provenance.
	s := seenTargetStore(t)
	m := seenTestMessage()
	if err := s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	// Act: invoke the production removal path.
	var err error
	if removal == "expiry" {
		_, err = s.DB.Exec(`INSERT INTO history_message_expiry VALUES('direct','peer','123',1,0)`)
		if err == nil {
			err = s.ExpireHistoryAt(t.Context(), time.Now())
		}
	} else {
		// Retention applies the configured age to the retained message.
		_, err = s.DB.Exec(`UPDATE messages SET sent_at='2000-01-01T00:00:00.000000000Z'`)
		if err == nil {
			err = s.Retain(t.Context())
		}
	}
	// Assert: no orphan native metadata remains; reads cannot target removed data.
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err = s.DB.QueryRow(`SELECT count(*) FROM message_seen_metadata`).Scan(&n); err != nil ||
		n != 0 {
		t.Fatal("private provenance survived removal")
	}
	if _, err = s.SeenTarget(t.Context(), m.Ref(), m.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("removed target remained actionable")
	}
}

func seenTargetStore(t *testing.T) *Store {
	t.Helper()
	s := reactionTargetStore(t)
	if err := s.BindAccount(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSeenTargetRejectsForeignAccountAndSender(t *testing.T) {
	for _, kind := range []string{"direct", "group"} {
		// Arrange: inconsistent direction must not authorize a receipt to self or another account.
		for _, who := range []string{"owner", "foreign"} {
			s := seenTargetStore(t)
			m := seenTestMessage()
			m.Conversation.Type = kind
			if kind == "group" {
				m.SeenMetadata.RecipientID = "peer"
				m.SenderID = "owner"
			} else {
				m.SeenMetadata.RecipientID = who
				if who == "owner" {
					m.SenderID = "another-peer"
				}
			}
			if err := s.Put(t.Context(), m); err != nil {
				t.Fatal(err)
			}
			// Act.
			_, err := s.SeenTarget(t.Context(), m.Ref(), m.ID)
			// Assert.
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("foreign provenance authorized", err)
			}
		}
	}
}

package storage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func clientAliasMessage(kind, peer, id, client string) domain.Message {
	return domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: peer}, GroupID: peer,
		ID: id, SenderID: "author", SentAt: time.Now().UTC(), Text: "synthetic", Source: "live",
		ProtocolClientMessageID: client}
}

func countClientAliases(t *testing.T, s *Store) int {
	t.Helper()
	var count int
	if err := s.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM message_client_ids`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestMessageClientAliasMigrationIsAtomic(t *testing.T) {
	// Arrange: simulate the prior schema and reject the new version marker.
	s := reactionTargetStore(t)
	putReactionTarget(t, s, "direct", "peer", "old", "123")
	_, err := s.DB.ExecContext(
		t.Context(),
		`DROP TABLE message_client_ids;DELETE FROM schema_migrations WHERE version=17;
CREATE TRIGGER reject_alias_migration BEFORE INSERT ON schema_migrations WHEN NEW.version=17 BEGIN SELECT RAISE(ABORT,'synthetic'); END;`,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	err = s.migrateMessageClientIDs(t.Context())
	// Assert: failed migration leaves old messages/quote data and no half-table.
	if err == nil {
		t.Fatal("migration marker failure ignored")
	}
	var tables, versions, messages, quotes int
	if err := s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='message_client_ids'`).
		Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=17`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM messages`).Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM message_quote_metadata`).Scan(&quotes); err != nil {
		t.Fatal(err)
	}
	if tables != 0 || versions != 0 || messages != 1 || quotes != 1 {
		t.Fatal("migration escaped atomic rollback")
	}
	// Act / Assert: retry succeeds without promoting quote data or history.
	if _, err := s.DB.Exec(`DROP TRIGGER reject_alias_migration`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateMessageClientIDs(t.Context()); err != nil || countClientAliases(t, s) != 0 {
		t.Fatal("migration retry failed", err)
	}
}

func TestMessageClientAliasRequiresMatchingIdentityAndIsImmutable(t *testing.T) {
	// Arrange: a stored message without a client alias.
	s := reactionTargetStore(t)
	m := clientAliasMessage("direct", "peer", "target", "")
	if err := s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	// Act / Assert: replay with a different author/time cannot bind an alias.
	wrong := m
	wrong.ProtocolClientMessageID = "123"
	wrong.SenderID = "other"
	if err := s.Put(t.Context(), wrong); err != nil {
		t.Fatal(err)
	}
	wrong = m
	wrong.ProtocolClientMessageID = "123"
	wrong.SentAt = wrong.SentAt.Add(time.Second)
	if err := s.Put(t.Context(), wrong); err != nil || countClientAliases(t, s) != 0 {
		t.Fatal("identity mismatch bound alias", err)
	}
	// Act: exact replay fills the alias; a conflicting repeat keeps the first ID.
	m.ProtocolClientMessageID = "123"
	if err := s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	m.ProtocolClientMessageID = "456"
	if err := s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	// Assert: no new message event, no ID replacement; conflict count has no IDs.
	var alias, conflicts string
	var events int
	if err := s.DB.QueryRow(`SELECT client_message_id FROM message_client_ids`).Scan(&alias); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT value FROM metadata WHERE key='message_client_id_conflicts'`).
		Scan(&conflicts); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM message_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if alias != "123" || conflicts != "1" || events != 1 {
		t.Fatal("alias was rewritten or replay emitted a message")
	}
}

func TestMessageClientAliasAndLegacyQuoteAmbiguity(t *testing.T) {
	for _, name := range []string{"same target", "contradictory metadata", "two targets"} {
		t.Run(name, func(t *testing.T) {
			// Arrange: lookup must consider all sources rather than prefer an alias.
			s := reactionTargetStore(t)
			m := clientAliasMessage("direct", "peer", "target", "123")
			quote := "123"
			if name == "contradictory metadata" {
				quote = "456"
			}
			m.QuoteMetadata = &domain.QuoteMetadata{ClientMessageID: quote, MessageType: "webchat", Timestamp: "1"}
			if err := s.Put(t.Context(), m); err != nil {
				t.Fatal(err)
			}
			if name == "two targets" {
				putReactionTarget(t, s, "direct", "peer", "other", "123")
			}
			// Act.
			if err := s.PutActivity(t.Context(), clientReaction()); err != nil {
				t.Fatal(err)
			}
			got, _ := storedReaction(t, s)
			// Assert.
			want := 0
			if name == "same target" {
				want = 1
			}
			if len(got.Reaction.TargetMessageIDs) != want {
				t.Fatal("ambiguous metadata was chosen")
			}
		})
	}
}

//nolint:gocognit // Keep silent-import and all removal paths in one AAA preservation scenario.
func TestMessageClientAliasSilentImportAndRemoval(t *testing.T) {
	for _, removal := range []string{"recall", "expiry", "retention"} {
		t.Run(removal, func(t *testing.T) {
			// Arrange: native metadata on a silent historical media row.
			s := reactionTargetStore(t)
			m := clientAliasMessage("group", "peer", "target", "123")
			m.Source = ""
			if removal == "retention" {
				m.SentAt = m.SentAt.Add(-100 * 24 * time.Hour)
			}
			_, err := s.PutExpiringHistoryPage(t.Context(), m.Ref(), []domain.ExpiringHistoryRecord{{Message: m}})
			if err != nil {
				t.Fatal(err)
			}
			var events, quotes int
			if err = s.DB.QueryRow(`SELECT count(*) FROM message_events`).Scan(&events); err != nil {
				t.Fatal(err)
			}
			if err = s.DB.QueryRow(`SELECT count(*) FROM message_quote_metadata`).Scan(&quotes); err != nil {
				t.Fatal(err)
			}
			if countClientAliases(t, s) != 1 || events != 0 || quotes != 0 {
				t.Fatal("alias granted quote authority or emitted history")
			}
			// Act: normal service removal must also erase the private alias.
			switch removal {
			case "recall":
				err = s.DeleteConversation(t.Context(), m.Ref(), m.ID)
			case "expiry":
				_, err = s.DB.Exec(`INSERT INTO history_message_expiry VALUES('group','peer','target',1,0)`)
				if err == nil {
					err = s.ExpireHistoryAt(t.Context(), time.Now())
				}
			case "retention":
				err = s.Retain(t.Context())
			}
			// Assert: FK cascade removes alias; replay cannot restore recalled/expired data.
			if err != nil || countClientAliases(t, s) != 0 {
				t.Fatal("removed message retained alias", err)
			}
			if removal != "retention" {
				if _, err := s.PutExpiringHistoryPage(
					t.Context(),
					m.Ref(),
					[]domain.ExpiringHistoryRecord{{Message: m}},
				); err != nil ||
					countClientAliases(t, s) != 0 {
					t.Fatal("removed alias resurrected", err)
				}
			}
		})
	}
}

func TestMessageClientAliasInvalidInputRollsBackWholeWrite(t *testing.T) {
	// Arrange: bounded exact identities cannot be truncated to a matching prefix.
	s := reactionTargetStore(t)
	m := clientAliasMessage("direct", "peer", "target", strings.Repeat("x", 65))
	// Act.
	err := s.Put(t.Context(), m)
	// Assert: no partial message, alias, event or incoming fact survives.
	var messages, events, facts int
	if queryErr := s.DB.QueryRow(`SELECT count(*) FROM messages`).Scan(&messages); queryErr != nil {
		t.Fatal(queryErr)
	}
	if queryErr := s.DB.QueryRow(`SELECT count(*) FROM message_events`).Scan(&events); queryErr != nil {
		t.Fatal(queryErr)
	}
	if queryErr := s.DB.QueryRow(`SELECT count(*) FROM peer_first_incoming`).Scan(&facts); queryErr != nil {
		t.Fatal(queryErr)
	}
	encoded, _ := json.Marshal(m)
	if err == nil || messages != 0 || events != 0 || facts != 0 || countClientAliases(t, s) != 0 ||
		strings.Contains(string(encoded), m.ProtocolClientMessageID) {
		t.Fatal("invalid private ID escaped transaction or public projection")
	}
}

func TestMessageClientAliasTypedNamespaceCollisions(t *testing.T) {
	// Arrange: all three conversations independently use the same client ID.
	s := reactionTargetStore(t)
	for _, m := range []domain.Message{
		clientAliasMessage("direct", "peer", "target", "123"),
		clientAliasMessage("direct", "other", "foreign", "123"),
		clientAliasMessage("group", "peer", "group-target", "123"),
	} {
		if err := s.Put(t.Context(), m); err != nil {
			t.Fatal(err)
		}
	}
	// Act.
	if err := s.PutActivity(t.Context(), clientReaction()); err != nil {
		t.Fatal(err)
	}
	got, _ := storedReaction(t, s)
	// Assert: lookup uses the exact typed conversation, not just the client ID.
	if len(got.Reaction.TargetMessageIDs) != 1 || got.Reaction.TargetMessageIDs[0] != "target" {
		t.Fatal("client alias crossed a conversation boundary")
	}
}

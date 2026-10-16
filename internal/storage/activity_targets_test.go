package storage

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func reactionTargetStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenWithPolicy(
		t.Context(),
		filepath.Join(t.TempDir(), "targets.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func putReactionTarget(t *testing.T, s *Store, kind, peer, global, client string) {
	t.Helper()
	m := domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: peer}, GroupID: peer,
		ID: global, SenderID: "author", SentAt: time.Now().UTC(), Text: "synthetic", Source: "live",
		QuoteMetadata: &domain.QuoteMetadata{
			ClientMessageID: client,
			MessageType:     "webchat",
			Timestamp:       "1",
		}}
	if err := s.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	// Group quote metadata is deliberately injected to verify typed isolation;
	// production currently records it for supported direct text only.
	if kind == "group" {
		_, err := s.DB.ExecContext(
			t.Context(),
			`INSERT INTO message_quote_metadata SELECT seq,?,'webchat','1',0 FROM messages WHERE conversation_type=? AND group_id=? AND message_id=?`,
			client,
			kind,
			peer,
			global,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func clientReaction() domain.Activity {
	return domain.Activity{
		Family:           "reaction",
		SchemaVersion:    1,
		ConversationType: "direct",
		ConversationID:   "peer",
		ActorID:          "peer",
		OccurredAt:       time.Now().UTC(),
		Direction:        "incoming",
		SourceID:         "action",
		IdentityBasis:    "upstream_action_id",
		Source:           "live",
		Reaction: &domain.ReactionObservation{
			TargetMessageIDs: []string{},
			Action:           "unknown",
		},
		ReactionClientMessageIDs: []string{"123"},
	}
}

func storedReaction(t *testing.T, s *Store) (domain.Activity, string) {
	t.Helper()
	var body string
	if err := s.DB.QueryRowContext(t.Context(), `SELECT payload_json FROM activity_observations LIMIT 1`).
		Scan(&body); err != nil {
		t.Fatal(err)
	}
	var a domain.Activity
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		t.Fatal(err)
	}
	return a, body
}

//nolint:gocognit // Keep the security boundary cases and their AAA assertions together.
func TestReactionClientTargetsRequireUniqueVisibleTypedMatch(t *testing.T) {
	for _, name := range []string{"unique", "other peer", "other type", "ambiguous", "missing", "recalled", "expired", "global preserved"} {
		t.Run(name, func(t *testing.T) {
			// Arrange: a client ID is meaningful only inside its exact conversation.
			s := reactionTargetStore(t)
			a := clientReaction()
			want := []string{}
			switch name {
			case "unique":
				putReactionTarget(t, s, "direct", "peer", "unrelated", "999")
				putReactionTarget(t, s, "direct", "peer", "resolved", "123")
				putReactionTarget(t, s, "direct", "other", "foreign", "123")
				putReactionTarget(t, s, "group", "peer", "group", "123")
				want = []string{"resolved"}
			case "other peer":
				putReactionTarget(t, s, "direct", "other", "foreign", "123")
			case "other type":
				putReactionTarget(t, s, "group", "peer", "group", "123")
			case "ambiguous":
				putReactionTarget(t, s, "direct", "peer", "one", "123")
				putReactionTarget(t, s, "direct", "peer", "two", "123")
			case "recalled", "expired":
				putReactionTarget(t, s, "direct", "peer", "hidden", "123")
				if name == "recalled" {
					if err := s.DeleteConversation(t.Context(), a.Ref(), "hidden"); err != nil {
						t.Fatal(err)
					}
				} else {
					_, err := s.DB.ExecContext(
						t.Context(),
						`INSERT INTO history_message_expiry VALUES('direct','peer','hidden',1,0)`,
					)
					if err != nil {
						t.Fatal(err)
					}
				}
			case "global preserved":
				putReactionTarget(t, s, "direct", "peer", "global", "123")
				a.Reaction.TargetMessageIDs = []string{"global"}
				want = []string{"global"}
			}
			// Act.
			if err := s.PutActivity(t.Context(), a); err != nil {
				t.Fatal(err)
			}
			got, body := storedReaction(t, s)
			// Assert: no inferred IDs, private hints or mutation of the caller's data.
			if !reflect.DeepEqual(got.Reaction.TargetMessageIDs, want) ||
				strings.Contains(body, "ReactionClientMessageIDs") ||
				len(a.ReactionClientMessageIDs) != 1 {
				t.Fatal("unsafe client reference resolution", got.Reaction.TargetMessageIDs)
			}
		})
	}
}

func TestReactionReplayDoesNotRewriteAfterClientMetadataAppears(t *testing.T) {
	// Arrange: unresolved source is retained before the target is available.
	s := reactionTargetStore(t)
	a := clientReaction()
	if err := s.PutActivity(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	_, before := storedReaction(t, s)
	putReactionTarget(t, s, "direct", "peer", "later", "123")
	// Act: replay the same native source after local enrichment.
	a.Source = "replay"
	if err := s.PutActivity(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	_, after := storedReaction(t, s)
	// Assert: payload and source identity remain immutable.
	var count int
	if err := s.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM activity_observations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if before != after || count != 1 {
		t.Fatal("replay rewrote the original source")
	}
}

func TestReactionPrivateClientHintsAreValidated(t *testing.T) {
	for _, hints := range [][]string{{""}, {"same", "same"}, {strings.Repeat("x", 65)}} {
		// Arrange: private json-excluded fields still cross a trust boundary.
		s := reactionTargetStore(t)
		a := clientReaction()
		a.ReactionClientMessageIDs = hints
		// Act.
		err := s.PutActivity(t.Context(), a)
		// Assert: invalid hints never enter the source ledger.
		var count int
		queryErr := s.DB.QueryRowContext(t.Context(), `SELECT count(*) FROM activity_observations`).Scan(&count)
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		if err == nil || count != 0 {
			t.Fatal("invalid private hint accepted")
		}
	}
}

func TestReactionUpgradeSuppressesEarlierCandidateFingerprint(t *testing.T) {
	for _, expired := range []bool{false, true} {
		// Arrange: a previous candidate stored the weaker ID without client hints.
		s := reactionTargetStore(t)
		old := clientReaction()
		old.SourceID, old.IdentityBasis = "old-fingerprint", "observed_fingerprint"
		old.ReactionClientMessageIDs = nil
		if err := s.PutActivity(t.Context(), old); err != nil {
			t.Fatal(err)
		}
		if expired {
			if _, err := s.DB.ExecContext(
				t.Context(),
				`UPDATE activity_observations SET expired=1,payload_json=''`,
			); err != nil {
				t.Fatal(err)
			}
		}
		var before string
		if err := s.DB.QueryRowContext(t.Context(), `SELECT payload_json FROM activity_observations`).
			Scan(&before); err != nil {
			t.Fatal(err)
		}
		putReactionTarget(t, s, "direct", "peer", "resolved", "123")
		updated := clientReaction()
		updated.SourceID, updated.IdentityBasis = "stronger-fingerprint", "observed_fingerprint"
		updated.LegacyReactionSourceID = old.SourceID
		// Act: replay the same native source through the updated candidate.
		if err := s.PutActivity(t.Context(), updated); err != nil {
			t.Fatal(err)
		}
		// Assert: no second source or changed payload, including an expired tombstone.
		var after string
		var count int
		if err := s.DB.QueryRowContext(t.Context(), `SELECT count(*),MIN(payload_json) FROM activity_observations`).
			Scan(&count, &after); err != nil {
			t.Fatal(err)
		}
		if count != 1 || before != after {
			t.Fatal("upgrade replay duplicated or rewrote old identity")
		}
	}
}

package zalo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

//nolint:gocognit // Keep this AAA sequence and local error checks adjacent for end-to-end assertions.
func TestReactionNormalizationKeepsNamespaceSelfAndReplayIdentity(t *testing.T) {
	// Arrange: same native action in both namespaces/directions, unknown target allowed.
	for _, kind := range []model.ThreadType{model.ThreadTypeUser, model.ThreadTypeGroup} {
		for _, self := range []bool{false, true} {
			from, to := "peer", "0"
			if kind == model.ThreadTypeGroup {
				to = "group"
			}
			if self {
				from = "0"
				if kind == model.ThreadTypeUser {
					to = "peer"
				}
			}
			raw := model.NewReaction(
				"owner",
				model.TReaction{
					MsgID:   "reaction-id",
					UIDFrom: from,
					IDTo:    to,
					MsgType: "chat.reaction",
					TS:      "1790812800123",
					Content: model.ReactionContent{
						ReactionData: model.ReactionData{RIcon: model.ReactionLike},
					},
				},
				kind,
			)
			// Act.
			live, err := normalizeReaction(raw, "live")
			if err != nil {
				t.Fatal(err)
			}
			replay, err := normalizeReaction(raw, "replay")
			if err != nil {
				t.Fatal(err)
			}
			// Assert: no guessed target/action; replay has a stable source fingerprint.
			if live.SourceID != replay.SourceID || live.IdentityBasis != "observed_fingerprint" ||
				live.Reaction.Action != "unknown" ||
				len(live.Reaction.TargetMessageIDs) != 0 {
				t.Fatal("reaction identity or meaning invented")
			}
			want := "incoming"
			if self {
				want = "outgoing"
			}
			if live.Direction != want || self && live.ActorID != "owner" ||
				live.ConversationID != to && kind == model.ThreadTypeGroup {
				t.Fatal("reaction direction or peer wrong")
			}
			body, err := json.Marshal(live)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			json.Unmarshal(body, &value)
			schema, err := contracts.Compile("reaction_v1", "payload")
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(value); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestReactionUnsupportedMetadataDoesNotStopNextEvent(t *testing.T) {
	// Arrange / Act: rejected native shape is diagnostic, never raw data in an error.
	count := 0
	bad := model.NewReaction(
		"owner",
		model.TReaction{TS: "not-time", UIDFrom: "peer", IDTo: "0"},
		model.ThreadTypeUser,
	)
	if err := deliverReaction(bad, "live", func(domain.Activity) error { count++; return nil }); err != nil {
		t.Fatal(err)
	}
	// A malformed observation cannot be passed to persistence.
	_, err := normalizeReaction(bad, "live")
	// Assert.
	if err == nil || strings.Contains(err.Error(), "not-time") || count != 0 {
		t.Fatal("invalid native reaction was not bounded")
	}
}

func TestReactionOutOfRangeTimestampCannotBecomeStorageFailure(t *testing.T) {
	// Arrange: a syntactically valid int64 that cannot be encoded as RFC3339.
	raw := model.NewReaction(
		"owner",
		model.TReaction{MsgID: "id", UIDFrom: "peer", IDTo: "0", TS: "9223372036854775807"},
		model.ThreadTypeUser,
	)
	calls := 0
	// Act.
	err := deliverReaction(raw, "live", func(domain.Activity) error { calls++; return nil })
	// Assert: reject only this malformed native observation, never storage/session.
	if err != nil || calls != 0 {
		t.Fatal("out of range reaction timestamp reached persistence", calls, err)
	}
}

func TestReactionClientOnlyReferencesPreserveIdentityWithoutGuessing(t *testing.T) {
	// Arrange: native global IDs are authoritative; client-only refs stay private.
	raw := model.NewReaction(
		"owner",
		model.TReaction{UIDFrom: "peer", IDTo: "0", TS: "1790812800123",
			Content: model.ReactionContent{RMsg: []model.ReactionMessageRef{
				{GMsgID: 999, CMsgID: 777}, {CMsgID: 123}, {CMsgID: 123},
			}}},
		model.ThreadTypeUser,
	)
	// Act.
	got, err := normalizeReaction(raw, "live")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := normalizeReaction(raw, "replay")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: no paired client lookup, no public client ID, stable live/replay ID.
	if len(got.Reaction.TargetMessageIDs) != 1 || got.Reaction.TargetMessageIDs[0] != "999" ||
		len(got.ReactionClientMessageIDs) != 1 || got.ReactionClientMessageIDs[0] != "123" ||
		got.SourceID != replay.SourceID || strings.Contains(string(encoded), `"123"`) || strings.Contains(string(encoded), `"777"`) {
		t.Fatal("native references or source identity lost")
	}
	// Act / Assert: distinct client-only target changes fallback identity.
	raw.Data.Content.RMsg = []model.ReactionMessageRef{{GMsgID: 999, CMsgID: 777}, {CMsgID: 456}}
	other, err := normalizeReaction(raw, "live")
	if err != nil || other.SourceID == got.SourceID {
		t.Fatal("client-only references omitted from identity", err)
	}
	// Act: the earlier candidate fingerprinted this record without client refs.
	raw.Data.Content.RMsg = []model.ReactionMessageRef{{GMsgID: 999, CMsgID: 777}}
	old, err := normalizeReaction(raw, "live")
	// Assert: the compatibility identity matches that exact old algorithm.
	if err != nil || got.LegacyReactionSourceID != old.SourceID ||
		other.LegacyReactionSourceID != old.SourceID {
		t.Fatal("previous candidate identity not preserved", err)
	}
}

func TestReactionClientReferenceOverflowIsRejected(t *testing.T) {
	// Arrange: do not silently truncate a native reference batch.
	refs := make([]model.ReactionMessageRef, 17)
	for i := range refs {
		refs[i].CMsgID = i + 1
	}
	raw := model.NewReaction(
		"owner",
		model.TReaction{UIDFrom: "peer", IDTo: "0", TS: "1790812800123",
			Content: model.ReactionContent{RMsg: refs}},
		model.ThreadTypeUser,
	)
	// Act.
	_, err := normalizeReaction(raw, "live")
	// Assert.
	if err == nil {
		t.Fatal("client reference overflow silently accepted")
	}
}

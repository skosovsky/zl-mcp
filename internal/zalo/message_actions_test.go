package zalo

import (
	"context"
	"errors"
	"testing"

	"github.com/amrakk/zcago"
	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type messageActionAPI struct {
	zcago.API

	own   string
	seen  func([]model.OutboundMessage, model.ThreadType) (*api.SendSeenEventResponse, error)
	react func(api.AddReactionDestination, model.ReactionData) (*api.AddReactionResponse, error)
}

func (m messageActionAPI) GetOwnID() string { return m.own }

func (m messageActionAPI) SendSeenEvent(
	_ context.Context,
	targets []model.OutboundMessage,
	kind model.ThreadType,
) (*api.SendSeenEventResponse, error) {
	return m.seen(targets, kind)
}

func (m messageActionAPI) AddReaction(
	_ context.Context,
	target api.AddReactionDestination,
	reaction model.ReactionData,
) (*api.AddReactionResponse, error) {
	return m.react(target, reaction)
}

func nativeActionTestTarget(kind string) domain.MessageActionTarget {
	recipient := "owner"
	if kind == "group" {
		recipient = "peer"
	}
	return domain.MessageActionTarget{
		Conversation:    domain.ConversationRef{Type: kind, ID: "peer"},
		MessageID:       "123",
		ClientMessageID: "456",
		SenderID:        "peer",
		Seen: &domain.SeenMetadata{
			ClientMessageID: "456",
			RecipientID:     recipient,
			MessageType:     "chat.photo",
			Timestamp:       "1791028800000",
			ST:              3,
			AT:              0,
			CMD:             501,
		},
	}
}

func TestMarkSeenAdapterExactProvenanceAndNoForeignDispatch(t *testing.T) {
	for _, kind := range []string{"direct", "group"} {
		assertMarkSeenNative(t, kind)
	}
}

func TestReactionCatalogAndExplicitRemoval(t *testing.T) {
	// Arrange: catalogue names form a bijection with the adapter's allowed encoding.
	seen := map[string]bool{}
	for _, choice := range domain.ReactionCatalog() {
		icon, ok := nativeReaction(choice.Code)
		if !ok || choice.Code == "" || choice.Name == "" || seen[choice.Code] || icon.TypeCode() < 0 {
			t.Fatal("catalogue choice is invalid")
		}
		seen[choice.Code] = true
		target := nativeActionTestTarget("direct")
		calls := 0
		client := Client{
			api: messageActionAPI{
				react: func(dest api.AddReactionDestination, r model.ReactionData) (*api.AddReactionResponse, error) {
					calls++
					if dest.ThreadID != "peer" || dest.Data.MsgID != "123" || dest.Data.CliMsgID != "456" ||
						dest.Type != model.ThreadTypeUser {
						t.Fatal("reaction crossed target")
					}
					if r != model.NewReactionData(icon) {
						t.Fatal("selected reaction changed")
					}
					return &api.AddReactionResponse{MsgIDs: []int{1}}, nil
				},
			},
		}
		// Act / Assert: do not accept arbitrary emoji or a removal with an icon.
		if err := client.React(t.Context(), target, choice.Code, false); err != nil || calls != 1 {
			t.Fatal("catalogue selection failed", err)
		}
		if err := client.React(t.Context(), target, "arbitrary-untrusted-value", false); err == nil {
			t.Fatal("unknown reaction accepted")
		}
		if err := client.React(t.Context(), target, choice.Code, true); err == nil || calls != 1 {
			t.Fatal("removal accepted an icon")
		}
	}
}

func TestMessageActionFailuresStayAmbiguousUnlessConfirmed(t *testing.T) {
	for _, tc := range []struct {
		code     errs.ZaloErrorCode
		rejected bool
	}{{0, false}, {500, false}, {408, false}, {114, true}, {-10, true}} {
		// Arrange: native errors contain text which must never be returned to a caller.
		raw := errs.NewZaloAPIError("private-upstream-error", &tc.code)
		// Act.
		err := classifyMessageActionError(raw)
		// Assert.
		if errors.Is(err, domain.ErrMessageActionRejected) != tc.rejected || err.Error() == raw.Error() {
			t.Fatal("unsafe action error classification")
		}
	}
}

func assertMarkSeenNative(t *testing.T, kind string) {
	t.Helper()
	// Arrange.
	target := nativeActionTestTarget(kind)
	calls := 0
	client := Client{
		api: messageActionAPI{
			own: "owner",
			seen: func(messages []model.OutboundMessage, typ model.ThreadType) (*api.SendSeenEventResponse, error) {
				calls++
				if len(messages) != 1 {
					t.Fatal("changed target count")
				}
				got := messages[0]
				if got.MsgID != "123" || got.CliMsgID != "456" || got.UIDFrom != "peer" ||
					got.IDTo != target.Seen.RecipientID ||
					got.MsgType != "chat.photo" ||
					got.TS != 1791028800000 ||
					got.ST != 3 ||
					got.AT != 0 ||
					got.CMD != 501 {
					t.Fatal("native provenance changed")
				}
				if (kind == "group") != (typ == model.ThreadTypeGroup) {
					t.Fatal("changed route")
				}
				return &api.SendSeenEventResponse{Status: 1, HasStatus: true}, nil
			},
		},
	}
	// Act.
	receipt, err := client.MarkSeen(t.Context(), []domain.MessageActionTarget{target})
	// Assert.
	if !errors.Is(err, domain.ErrUnverifiedReadStatus) || receipt.Status == nil || *receipt.Status != 1 || calls != 1 {
		t.Fatal("native evidence was lost or overstated", err)
	}
	for _, mutate := range []func(*domain.MessageActionTarget){
		func(v *domain.MessageActionTarget) { v.Seen = nil },
		func(v *domain.MessageActionTarget) { v.Seen.RecipientID = "foreign" },
		func(v *domain.MessageActionTarget) { v.SenderID = "owner" },
		func(v *domain.MessageActionTarget) { v.Seen.ClientMessageID = "other" },
		func(v *domain.MessageActionTarget) { v.Seen.Timestamp = "invalid" },
	} {
		bad := nativeActionTestTarget(kind)
		mutate(&bad)
		if _, err = client.MarkSeen(t.Context(), []domain.MessageActionTarget{bad}); err == nil {
			t.Fatal("foreign/missing provenance accepted")
		}
	}
	if calls != 1 {
		t.Fatal("invalid metadata triggered receipt")
	}
}

func TestReactionExplicitRemoval(t *testing.T) {
	// Arrange / Act / Assert: removal is a distinct explicit instruction.
	target := nativeActionTestTarget("group")
	client := Client{
		api: messageActionAPI{
			react: func(dest api.AddReactionDestination, r model.ReactionData) (*api.AddReactionResponse, error) {
				if dest.Type != model.ThreadTypeGroup || r.RIcon != model.ReactionNone || r.RType != -1 ||
					r.Source != 6 {
					t.Fatal("removal sentinel changed")
				}
				return &api.AddReactionResponse{MsgIDs: []int{1}}, nil
			},
		},
	}
	if err := client.React(t.Context(), target, "", true); err != nil {
		t.Fatal(err)
	}
}

func TestReadAcceptanceRequiresObservedZeroStatus(t *testing.T) {
	for _, tc := range []struct {
		kind     string
		response *api.SendSeenEventResponse
		accepted bool
	}{
		{"direct", &api.SendSeenEventResponse{Status: 0, HasStatus: true}, true},
		{"group", &api.SendSeenEventResponse{Status: 0, HasStatus: true}, true},
		{"group", &api.SendSeenEventResponse{Status: 1, HasStatus: true}, false},
		{"group", &api.SendSeenEventResponse{Status: 0, HasStatus: false}, false},
		{"direct", &api.SendSeenEventResponse{Status: 1, HasStatus: true}, false},
		{"direct", &api.SendSeenEventResponse{Status: 0, HasStatus: false}, false},
	} {
		// Arrange: route and status presence are independent protocol evidence.
		client := Client{
			api: messageActionAPI{
				own: "owner",
				seen: func([]model.OutboundMessage, model.ThreadType) (*api.SendSeenEventResponse, error) {
					return tc.response, nil
				},
			},
		}
		// Act.
		result, err := client.MarkSeen(t.Context(), []domain.MessageActionTarget{nativeActionTestTarget(tc.kind)})
		// Assert: only the natively established zero status on either route is accepted.
		if (err == nil) != tc.accepted || (result.Status != nil) != tc.response.HasStatus {
			t.Fatal(tc.kind, tc.response, err)
		}
	}
}

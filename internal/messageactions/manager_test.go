package messageactions

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type actionSource struct {
	seen  func(context.Context, []domain.MessageActionTarget) error
	react func(context.Context, domain.MessageActionTarget, string, bool) error
}

func (s actionSource) MarkSeen(c context.Context, t []domain.MessageActionTarget) (domain.ReadReceiptResult, error) {
	return domain.ReadReceiptResult{}, s.seen(c, t)
}
func (s actionSource) React(c context.Context, t domain.MessageActionTarget, r string, remove bool) error {
	return s.react(c, t, r, remove)
}

func actionManager(t *testing.T) *Manager {
	t.Helper()
	store, err := storage.OpenWithPolicy(
		t.Context(),
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err = store.BindAccount(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	ref := domain.ConversationRef{Type: "direct", ID: "peer"}
	m := domain.Message{
		Conversation:            ref,
		ID:                      "123",
		SenderID:                "peer",
		Direction:               "incoming",
		Source:                  "live",
		SentAt:                  time.UnixMilli(1791028800000),
		Text:                    "synthetic",
		ProtocolClientMessageID: "456",
		SeenMetadata: &domain.SeenMetadata{
			ClientMessageID: "456",
			RecipientID:     "owner",
			MessageType:     "webchat",
			Timestamp:       "1791028800000",
		},
	}
	if err = store.Put(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	return &Manager{
		Store:         store,
		ReadChats:     map[domain.ConversationRef]bool{ref: true},
		ReactionChats: map[domain.ConversationRef]bool{ref: true},
	}
}
func actionIntent(plan string) domain.MessageActionIntent {
	kind := "reaction"
	reaction := "heart"
	if plan == "read" {
		kind = "read"
		reaction = ""
	}
	if plan == "clear" {
		reaction = ""
	}
	return domain.MessageActionIntent{
		RequestID:    "a0000000-0000-4000-8000-000000000001",
		Kind:         kind,
		Plan:         plan,
		Conversation: domain.ConversationRef{Type: "direct", ID: "peer"},
		MessageIDs:   []string{"123"},
		Reaction:     reaction,
	}
}

func TestExplicitActionsReplayNeverDuplicatesUpstream(t *testing.T) {
	for _, plan := range []string{"read", "add", "clear", "clear_add"} {
		t.Run(plan, func(t *testing.T) {
			// Arrange.
			m := actionManager(t)
			var calls atomic.Int32
			m.Source = actionSource{
				seen:  func(context.Context, []domain.MessageActionTarget) error { calls.Add(1); return nil },
				react: func(context.Context, domain.MessageActionTarget, string, bool) error { calls.Add(1); return nil },
			}
			intent := actionIntent(plan)
			// Act.
			first, err := m.Execute(t.Context(), intent)
			second, replayErr := m.Execute(t.Context(), intent)
			// Assert: confirmed step count matches the saved plan, replay returns the result.
			count := int32(1)
			if plan == "clear_add" {
				count = 2
			}
			if err != nil || replayErr != nil || first.Status != "accepted" || second.Status != "accepted" ||
				calls.Load() != count ||
				int32(first.CompletedSteps) != count {
				t.Fatal("action replayed or lost acceptance", err, replayErr)
			}
		})
	}
}

func TestActionUnknownAndPartialCannotResume(t *testing.T) {
	for _, reject := range []bool{false, true} {
		// Arrange: clear succeeded; add either failed ambiguously or was rejected.
		m := actionManager(t)
		calls := 0
		m.Source = actionSource{
			react: func(_ context.Context, _ domain.MessageActionTarget, _ string, remove bool) error {
				calls++
				if remove {
					return nil
				}
				if reject {
					return domain.ErrMessageActionRejected
				}
				return errors.New("private-upstream-marker")
			},
		}
		intent := actionIntent("clear_add")
		// Act.
		op, err := m.Execute(t.Context(), intent)
		again, replayErr := m.Execute(t.Context(), intent)
		// Assert.
		want := "unknown"
		if reject {
			want = "partial"
		}
		if err != nil || replayErr != nil || op.Status != want || again.Status != want || op.CompletedSteps != 1 ||
			calls != 2 {
			t.Fatal("failed replacement retried or hidden", err, replayErr)
		}
	}
}

func TestActionPermissionAndMetadataFailBeforeUpstream(t *testing.T) {
	for _, mode := range []string{"permission", "unknown_message", "foreign_chat", "invalid_native_id"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			m := actionManager(t)
			calls := 0
			m.Source = actionSource{
				seen: func(context.Context, []domain.MessageActionTarget) error { calls++; return nil },
			}
			intent := actionIntent("read")
			switch mode {
			case "permission":
				m.ReadChats = nil
			case "unknown_message":
				intent.MessageIDs = []string{"999"}
			case "foreign_chat":
				intent.Conversation.ID = "other"
			case "invalid_native_id":
				if _, err := m.Store.DB.Exec(
					`UPDATE message_seen_metadata SET client_message_id='invalid'`,
				); err != nil {
					t.Fatal(err)
				}
			}
			// Act.
			_, err := m.Execute(t.Context(), intent)
			// Assert.
			if err == nil || calls != 0 {
				t.Fatal("denied action reached upstream")
			}
		})
	}
}

func TestAcceptedActionSurvivesCallerCancellation(t *testing.T) {
	// Arrange.
	m := actionManager(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m.Source = actionSource{seen: func(context.Context, []domain.MessageActionTarget) error { cancel(); return nil }}
	// Act.
	op, err := m.Execute(ctx, actionIntent("read"))
	// Assert.
	if err != nil || op.Status != "accepted" || op.CompletedSteps != 1 {
		t.Fatal("accepted receipt lost after cancellation", err)
	}
}

func TestActionTargetsAreRevalidatedAfterClaimAndBetweenSteps(t *testing.T) {
	for _, when := range []string{"claim", "after_clear"} {
		t.Run(when, func(t *testing.T) {
			// Arrange: invalidate a previously resolved target at an irreversible boundary.
			m := actionManager(t)
			calls := 0
			if when == "claim" {
				if _, err := m.Store.DB.Exec(
					`CREATE TRIGGER remove_action_target AFTER UPDATE OF state ON message_action_operations WHEN NEW.state='dispatching' BEGIN DELETE FROM messages WHERE message_id='123';END;`,
				); err != nil {
					t.Fatal(err)
				}
			}
			m.Source = actionSource{
				react: func(_ context.Context, target domain.MessageActionTarget, _ string, remove bool) error {
					calls++
					if when == "after_clear" && remove {
						return m.Store.DeleteConversation(t.Context(), target.Conversation, target.MessageID)
					}
					return nil
				},
			}
			// Act.
			op, err := m.Execute(t.Context(), actionIntent("clear_add"))
			// Assert: an invalidated target never receives the next upstream step.
			want := "rejected"
			count := 0
			if when == "after_clear" {
				want = "partial"
				count = 1
			}
			if err != nil || op.Status != want || op.CompletedSteps != count || calls != count || op.Reason == nil ||
				*op.Reason != "metadata_unavailable" {
				t.Fatal("invalid target was dispatched", err)
			}
		})
	}
}

type unverifiedReadSource struct{ actionSource }

func (s unverifiedReadSource) MarkSeen(
	context.Context,
	[]domain.MessageActionTarget,
) (domain.ReadReceiptResult, error) {
	return domain.ReadReceiptResult{Status: new(7)}, domain.ErrUnverifiedReadStatus
}

func TestUnverifiedReadStatusIsDurableEvidenceNotAcceptance(t *testing.T) {
	// Arrange: an arbitrary native numeric status has no proven success semantics.
	m := actionManager(t)
	m.Source = unverifiedReadSource{}
	// Act.
	op, err := m.Execute(t.Context(), actionIntent("read"))
	saved, readErr := m.Store.MessageActionStatus(t.Context(), op.RequestID)
	// Assert.
	if err != nil || readErr != nil || op.Status != "unknown" || op.UpstreamStatus == nil || *op.UpstreamStatus != 7 ||
		saved.UpstreamStatus == nil ||
		*saved.UpstreamStatus != 7 ||
		saved.CompletedSteps != 0 ||
		saved.Reason == nil ||
		*saved.Reason != "upstream_unverified" {
		t.Fatal("unverified status became success or disappeared", err, readErr)
	}
}

func TestActionPersistenceFailuresNeverReplayOrLeaveRecoverableClaimBusy(t *testing.T) {
	for _, tc := range []struct {
		name, plan, trigger, want string
		calls                     int
	}{
		{"checkpoint before dispatch", "add", `CREATE TRIGGER fail_stage BEFORE UPDATE OF stage ON message_action_operations WHEN NEW.stage='add' BEGIN SELECT RAISE(ABORT,'synthetic');END;`, "rejected", 0},
		{"confirmation after dispatch", "add", `CREATE TRIGGER fail_confirm BEFORE UPDATE OF completed_steps ON message_action_operations WHEN NEW.completed_steps>OLD.completed_steps BEGIN SELECT RAISE(ABORT,'synthetic');END;`, "unknown", 1},
		{"read evidence after dispatch", "read", `CREATE TRIGGER fail_receipt BEFORE UPDATE OF upstream_status ON message_action_operations BEGIN SELECT RAISE(ABORT,'synthetic');END;`, "unknown", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: abort one persistence boundary without disabling final state writes.
			m := actionManager(t)
			calls := 0
			m.Source = actionSource{
				seen:  func(context.Context, []domain.MessageActionTarget) error { calls++; return nil },
				react: func(context.Context, domain.MessageActionTarget, string, bool) error { calls++; return nil },
			}
			if _, err := m.Store.DB.Exec(tc.trigger); err != nil {
				t.Fatal(err)
			}
			intent := actionIntent(tc.plan)
			// Act.
			result, err := m.Execute(t.Context(), intent)
			replay, replayErr := m.Execute(t.Context(), intent)
			// Assert: uncertainty survives UUID replay and conversation claim is released.
			if err != nil || replayErr != nil || result.Status != tc.want || replay.Status != tc.want ||
				calls != tc.calls ||
				result.Reason == nil ||
				*result.Reason != "storage_unavailable" {
				t.Fatal("persistence failure hid/replayed the external step", err, replayErr)
			}
			other := actionIntent("read")
			other.RequestID = "a0000000-0000-4000-8000-000000000002"
			if _, err = m.Store.PrepareMessageAction(
				t.Context(),
				other.RequestID,
				other.Kind,
				other.Plan,
				other.Conversation,
				other.Fingerprint(),
			); err != nil {
				t.Fatal(err)
			}
			if won, err := m.Store.ClaimMessageAction(t.Context(), other.RequestID); err != nil || !won {
				t.Fatal("recoverable failure kept the chat locked", err)
			}
		})
	}
}

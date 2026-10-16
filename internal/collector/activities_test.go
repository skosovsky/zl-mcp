package collector

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type activityScriptedListener struct {
	*scriptedListener

	activityScript func(context.Context, func(domain.Message) error, func() error, func(domain.Activity) error) error
}

func (s *activityScriptedListener) ListenConversationActivities(
	ctx context.Context,
	message func(domain.Message) error,
	_ func(domain.ConversationRef, string) error,
	ready func() error,
	activity func(domain.Activity) error,
) error {
	return s.activityScript(ctx, message, ready, activity)
}

func TestCollectorActivityGuardPreservesFirstMediaMessage(t *testing.T) {
	// Arrange: the real session guard must forward activity without spending novelty.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	c := config.Config{StateDir: t.TempDir()}
	c.Collection.Mode = "all"
	s, err := storage.OpenWithPolicy(ctx, filepath.Join(c.StateDir, "state.sqlite"), c.Policy(), 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	client := &activityScriptedListener{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}}
	client.activityScript = func(ctx context.Context, message func(domain.Message) error, ready func() error, activity func(domain.Activity) error) error {
		if callbackErr := ready(); callbackErr != nil {
			return callbackErr
		}
		a := domain.Activity{
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
			Reaction:         &domain.ReactionObservation{TargetMessageIDs: []string{}, Action: "unknown"},
		}
		if callbackErr := activity(a); callbackErr != nil {
			return callbackErr
		}
		a.Source = "replay"
		if callbackErr := activity(a); callbackErr != nil {
			return callbackErr
		}
		if callbackErr := message(
			domain.Message{
				Conversation: a.Ref(),
				ID:           "first-photo",
				SenderID:     "peer",
				Direction:    "incoming",
				SentAt:       a.OccurredAt,
				Source:       "live",
				Content: &domain.MessageContent{
					Kind:         "image",
					TextRole:     "none",
					Summary:      "Photo",
					Availability: "metadata_only",
					Reason:       new("metadata_limited"),
					Elements:     []domain.ContentElement{{Kind: "image", BytesAvailable: false}},
				},
			},
		); callbackErr != nil {
			return callbackErr
		}
		cancel()
		return ctx.Err()
	}
	// Act: shared listener -> session guard -> collector state -> durable storage.
	err = RunInternal(ctx, c, s, client, func(*JoinManager) error { return nil })
	// Assert: live/replay dedup and original message novelty remain independent.
	if err != nil {
		t.Fatal(err)
	}
	var activities, first int
	if err = s.DB.QueryRow("SELECT count(*) FROM activity_observations").Scan(&activities); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM message_events WHERE first_incoming=1").Scan(&first); err != nil {
		t.Fatal(err)
	}
	if activities != 1 || first != 1 {
		t.Fatalf("activities=%d first=%d", activities, first)
	}
	m, err := s.ConversationMessage(t.Context(), domain.ConversationRef{Type: "direct", ID: "peer"}, "first-photo")
	if err != nil || m.Content == nil || m.Content.Kind != "image" {
		t.Fatal("first media content lost", err)
	}
	state, err := s.State(t.Context())
	if err != nil || state["last_persisted_at"] == nil || state["collector_state"] != "stopped" {
		t.Fatal("activity lifecycle status lost", err)
	}
}

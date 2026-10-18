package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func publicActionPort(t *testing.T) *membershipPort {
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
	message := domain.Message{
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
	if err = store.Put(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	return &membershipPort{
		store:         store,
		actions:       &actionProbeSource{},
		readChats:     map[domain.ConversationRef]bool{ref: true},
		reactionChats: map[domain.ConversationRef]bool{ref: true},
	}
}

func TestPublicMessageActionsComposeExactPlanAndReadStatusAfterRevocation(t *testing.T) {
	// Arrange: a live synthetic source and a genuinely retained incoming message.
	port := publicActionPort(t)
	source := port.actions.(*actionProbeSource)
	args := map[string]any{
		"request_id":        "a0000000-0000-4000-8000-000000000001",
		"conversation_type": "direct",
		"conversation_id":   "peer",
		"message_id":        "123",
		"action":            "set",
		"reaction":          "heart",
	}
	// Act: a replacement is an explicitly selected clear/add plan; replay is a read.
	first, err := port.Call(t.Context(), "zalo_set_message_reaction", args)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := port.Call(t.Context(), "zalo_set_message_reaction", args)
	// Assert: exactly two native requests, with a stable accepted result.
	if err != nil || first["status"] != "accepted" || first["plan"] != "clear_add" ||
		first["completed_steps"] != float64(2) ||
		source.calls != 2 ||
		first["updated_at"] != replay["updated_at"] {
		t.Fatal(first, replay, err, source.calls)
	}
	schema, err := contracts.Compile("zalo_set_message_reaction", "output")
	if err != nil || schema.Validate(first) != nil {
		t.Fatal("result violates executable contract", err)
	}
	// Act: disable writes and detach the source. Status must remain available.
	port.reactionChats = nil
	port.readChats = nil
	port.actions = nil
	saved, err := port.Call(
		t.Context(),
		"zalo_get_message_action_status",
		map[string]any{"request_id": args["request_id"]},
	)
	// Assert: no native calls and no current grant/session dependency.
	if err != nil || saved["status"] != "accepted" || source.calls != 2 {
		t.Fatal(saved, err, source.calls)
	}
	// Act / Assert: another write is denied without changing the ledger result.
	_, err = port.Call(t.Context(), "zalo_set_message_reaction", args)
	if err == nil || source.calls != 2 {
		t.Fatal("write permission bypass", err)
	}
}

func TestPublicMessageActionRejectsProtocolFieldsAndPreservesConflict(t *testing.T) {
	// Arrange.
	port := publicActionPort(t)
	source := port.actions.(*actionProbeSource)
	base := map[string]any{
		"request_id":        "a0000000-0000-4000-8000-000000000001",
		"conversation_type": "direct",
		"conversation_id":   "peer",
		"message_ids":       []string{"123"},
	}
	// Act / Assert: protocol fields and wrong chat are rejected before dispatch.
	base["st"] = 0
	if _, err := port.Call(t.Context(), "zalo_mark_messages_read", base); err == nil || source.calls != 0 {
		t.Fatal("native field accepted")
	}
	delete(base, "st")
	if _, err := port.Call(t.Context(), "zalo_mark_messages_read", base); err != nil {
		t.Fatal(err)
	}
	base["message_ids"] = []string{"456"}
	_, err := port.Call(t.Context(), "zalo_mark_messages_read", base)
	var wire *domain.Error
	raw, _ := json.Marshal(err)
	_ = json.Unmarshal(raw, &wire)
	if wire == nil || wire.Code != "REQUEST_CONFLICT" || source.calls != 1 {
		t.Fatal("conflict lost", err)
	}
}

func TestPublicMessageActionFollowsActiveSessionCancellation(t *testing.T) {
	// Arrange: the production Call entry point, not the private diagnostic route.
	port := publicActionPort(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	port.actions = cancellingActionProbeSource{started: started}
	port.actionContext = ctx
	done := make(chan map[string]any, 1)
	go func() {
		result, err := port.Call(
			t.Context(),
			"zalo_mark_messages_read",
			map[string]any{
				"request_id":        "a0000000-0000-4000-8000-000000000001",
				"conversation_type": "direct",
				"conversation_id":   "peer",
				"message_ids":       []string{"123"},
			},
		)
		if err != nil {
			done <- nil
			return
		}
		done <- result
	}()
	// Act.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("no dispatch")
	}
	cancel()
	// Assert: cancellation retains uncertainty, no successful result or retry.
	select {
	case result := <-done:
		if result["status"] != "unknown" {
			t.Fatal(result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ignored session cancellation")
	}
}

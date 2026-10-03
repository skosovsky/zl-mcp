package collector

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"path/filepath"
	"testing"
	"time"
)

type typedScriptedListener struct {
	*scriptedListener
	typed func(context.Context, func(domain.Message) error, func(domain.ConversationRef, string) error, func() error) error
}

func (c *typedScriptedListener) ListenConversations(ctx context.Context, m func(domain.Message) error, d func(domain.ConversationRef, string) error, ready func() error) error {
	return c.typed(ctx, m, d, ready)
}

func TestTypedCollectorPersistsBothKindsAndDeletesOnlyTarget(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c config.Config
	c.StateDir = t.TempDir()
	c.Collection.Mode = "all"
	s, err := storage.OpenWithPolicy(ctx, filepath.Join(c.StateDir, "messages.sqlite"), c.Policy(), 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	client := &typedScriptedListener{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{name: "Synthetic group", joined: true}}}
	client.typed = func(ctx context.Context, message func(domain.Message) error, deletion func(domain.ConversationRef, string) error, ready func() error) error {
		if err := ready(); err != nil {
			return err
		}
		for _, kind := range []string{domain.ConversationDirect, domain.ConversationGroup} {
			if err := message(domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: "same"}, ID: "same-message", SenderID: "peer", SentAt: time.Now(), Text: kind + " synthetic", Source: "replay"}); err != nil {
				return err
			}
		}
		if err := deletion(domain.ConversationRef{Type: domain.ConversationGroup, ID: "same"}, "same-message"); err != nil {
			return err
		}
		if err := message(domain.Message{Conversation: domain.ConversationRef{Type: domain.ConversationDirect, ID: "new-peer"}, ID: "self", SenderID: "owner", SentAt: time.Now(), Text: "outgoing synthetic", Source: "live"}); err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	}
	// Act
	err = RunInternal(ctx, c, s, client, func(*JoinManager) error { return nil })
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	read := context.Background()
	if _, err = s.ConversationMessage(read, domain.ConversationRef{Type: domain.ConversationDirect, ID: "same"}, "same-message"); err != nil {
		t.Fatal("typed guard dropped direct replay")
	}
	if _, err = s.Message(read, "same", "same-message"); err == nil {
		t.Fatal("group deletion failed")
	}
	m, err := s.ConversationMessage(read, domain.ConversationRef{Type: domain.ConversationDirect, ID: "new-peer"}, "self")
	if err != nil || m.SenderID != "owner" {
		t.Fatal("new peer/self message lost")
	}
	state, err := s.State(read)
	if err != nil {
		t.Fatal(err)
	}
	if state["stored_message_count"] != 2 || state["last_persisted_at"] == nil {
		t.Fatal("direct persistence missing from status")
	}
}

package storage

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"path/filepath"
	"testing"
	"time"
)

func TestAllPolicySeparatesMessagesEventsAndLegacyReads(t *testing.T) {
	// Arrange
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	group := domain.ConversationRef{Type: domain.ConversationGroup, ID: "same"}
	direct := domain.ConversationRef{Type: domain.ConversationDirect, ID: "same"}
	put := func(ref domain.ConversationRef, text string) {
		t.Helper()
		err := s.Put(ctx, domain.Message{Conversation: ref, ID: "same-message", SenderID: "peer", SentAt: time.Now(), Text: text, Source: "replay"})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Act
	put(group, "group synthetic")
	put(direct, "direct synthetic")
	put(direct, "duplicate synthetic")
	// Assert
	gm, err := s.Message(ctx, "same", "same-message")
	if err != nil || gm.Text != "group synthetic" {
		t.Fatal("legacy read crossed namespace")
	}
	dm, err := s.ConversationMessage(ctx, direct, "same-message")
	if err != nil || dm.Text != "direct synthetic" || dm.GroupID != "" || dm.Ref() != direct {
		t.Fatal("direct read lost identity or duplicate replaced data")
	}
	var events, identities, catalog int
	if err = s.DB.QueryRow("SELECT count(*) FROM message_events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM message_identities").Scan(&identities); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM conversations").Scan(&catalog); err != nil {
		t.Fatal(err)
	}
	if events != 2 || identities != 2 || catalog != 2 {
		t.Fatal("typed dedup or automatic discovery failed")
	}
	if err = s.Delete(ctx, "same", "same-message"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConversationMessage(ctx, direct, "same-message"); err != nil {
		t.Fatal("group deletion removed direct record")
	}
	if err = s.DeleteConversation(ctx, direct, "same-message"); err != nil {
		t.Fatal(err)
	}
	put(direct, "retained identity synthetic")
	if err = s.DB.QueryRow("SELECT count(*) FROM message_events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatal("replayed deleted text generated another event")
	}
}

func TestSelectedDirectPolicyDoesNotAdmitSameGroup(t *testing.T) {
	// Arrange
	ctx := context.Background()
	ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: "same"}
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{ref: true}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := domain.Message{Conversation: ref, ID: "m", SenderID: "peer", SentAt: time.Now(), Text: "synthetic", Source: "live"}
	// Act
	if err = s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	m.Conversation.Type = domain.ConversationGroup
	if err = s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	// Assert
	if s.Allowed("same") {
		t.Fatal("selected direct widened to group")
	}
	var count int
	if err = s.DB.QueryRow("SELECT count(*) FROM messages").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("policy failed at ingestion")
	}
}

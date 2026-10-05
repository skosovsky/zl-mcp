package zalo

import (
	"errors"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestPreloadHistorySelectsOnlyTypedDialogueWithinLimit(t *testing.T) {
	// Arrange
	ref := domain.ConversationRef{Type: "direct", ID: "same"}
	stamp := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	snapshot := domain.PreloadSnapshot{DirectMessagesAvailable: true, GroupMessagesAvailable: true, Messages: []domain.Message{
		{Conversation: ref, ID: "older", SentAt: stamp.Add(-time.Hour)},
		{Conversation: domain.ConversationRef{Type: "group", ID: "same"}, ID: "group", SentAt: stamp},
		{Conversation: ref, ID: "newest-b", SentAt: stamp},
		{Conversation: ref, ID: "newest-a", SentAt: stamp},
	}}
	// Act
	page, err := selectPreloadHistory(snapshot, ref, 2)
	// Assert
	if err != nil || len(page.Messages) != 2 || page.Messages[0].ID != "newest-a" || page.Messages[1].ID != "newest-b" || !page.LimitedSnapshot || page.HasMore != nil || page.Cursor != nil {
		t.Fatal("snapshot selection fabricated paging or mixed identity")
	}
	if snapshot.Messages[0].ID != "older" {
		t.Fatal("selection mutated source snapshot")
	}
	// Empty available source remains a partial snapshot, not absence/exhaustion.
	page, err = selectPreloadHistory(domain.PreloadSnapshot{DirectMessagesAvailable: true}, ref, 2)
	if err != nil || len(page.Messages) != 0 || !page.LimitedSnapshot || page.HasMore != nil {
		t.Fatal("empty source fabricated exhaustion")
	}
	_, err = selectPreloadHistory(domain.PreloadSnapshot{}, ref, 2)
	if !errors.Is(err, domain.ErrHistoryUnsupported) {
		t.Fatal("absent category not unsupported")
	}
}

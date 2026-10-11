package conversations

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"testing"
)

type collisionReads struct {
	ReadPort
	calls int
}

func (r *collisionReads) GroupContext(context.Context, ContextRequest) (GroupContext, error) {
	r.calls++
	return GroupContext{}, nil
}
func (r *collisionReads) Message(context.Context, MessageRequest) (domain.Message, error) {
	r.calls++
	return domain.Message{}, nil
}
func TestLegacyGroupCapabilityCannotReadDirectNamespace(t *testing.T) {
	// Arrange: a direct and a group can share the same opaque ID.
	reads := &collisionReads{}
	handler := Handler{Reads: reads}
	ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: "9007199254740993"}
	// Act
	_, contextErr := handler.GroupContext(context.Background(), ContextRequest{Conversation: ref, MessageID: "9007199254740995"})
	_, messageErr := handler.Message(context.Background(), MessageRequest{Conversation: ref, MessageID: "9007199254740995", LegacyGroup: true})
	// Assert: reject the namespace before dispatching a read.
	if contextErr == nil || messageErr == nil || reads.calls != 0 {
		t.Fatal(contextErr, messageErr, reads.calls)
	}
}

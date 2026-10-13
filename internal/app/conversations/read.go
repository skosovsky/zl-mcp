// Package conversations exposes typed reads without transport or storage ownership.
package conversations

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type ContextRequest struct {
	Conversation  domain.ConversationRef
	MessageID     string
	Before, After int
}
type MessageRequest struct {
	Conversation domain.ConversationRef
	MessageID    string
	LegacyGroup  bool
}

type GroupContext struct {
	Anchor           domain.Message     `json:"anchor"`
	Before           []domain.Message   `json:"before"`
	After            []domain.Message   `json:"after"`
	ReplyTo          *domain.Message    `json:"reply_to"`
	Coverage         domain.Coverage    `json:"coverage"`
	ContextTruncated bool               `json:"context_truncated"`
	OmittedBeforeID  *string            `json:"omitted_before_id"`
	OmittedAfterID   *string            `json:"omitted_after_id"`
	NextAction       *domain.NextAction `json:"next_action"`
}

type ConversationContext struct {
	Anchor           shared.ConversationMessage   `json:"anchor"`
	Before           []shared.ConversationMessage `json:"before"`
	After            []shared.ConversationMessage `json:"after"`
	ReplyTo          *shared.ConversationMessage  `json:"reply_to"`
	Coverage         shared.ConversationCoverage  `json:"coverage"`
	ContextTruncated bool                         `json:"context_truncated"`
	OmittedBeforeID  *string                      `json:"omitted_before_id"`
	OmittedAfterID   *string                      `json:"omitted_after_id"`
	NextAction       *domain.NextAction           `json:"next_action"`
}

// ReadPort is owned by this consumer; every method rechecks current read access.
type ReadPort interface {
	GroupContext(context.Context, ContextRequest) (GroupContext, error)
	ConversationContext(context.Context, ContextRequest) (ConversationContext, error)
	Message(context.Context, MessageRequest) (domain.Message, error)
}

type Handler struct {
	Reads          ReadPort
	Catalog        CatalogPort
	LiveGroups     LiveGroupPort
	DirectoryReads DirectoryPort
}

func (h *Handler) GroupContext(ctx context.Context, r ContextRequest) (GroupContext, error) {
	if r.Conversation.Type != domain.ConversationGroup {
		return GroupContext{}, domain.Invalid("Group context requires a group identity.")
	}
	return h.Reads.GroupContext(ctx, r)
}
func (h *Handler) ConversationContext(ctx context.Context, r ContextRequest) (ConversationContext, error) {
	return h.Reads.ConversationContext(ctx, r)
}
func (h *Handler) Message(ctx context.Context, r MessageRequest) (domain.Message, error) {
	if r.LegacyGroup && r.Conversation.Type != domain.ConversationGroup {
		return domain.Message{}, domain.Invalid("Legacy group reads require a group identity.")
	}
	return h.Reads.Message(ctx, r)
}

// AccessPort checks current context access before a handler selects its anchor.
type AccessPort interface {
	CheckGroupContext(context.Context, ContextRequest) error
	CheckConversationContext(context.Context, ContextRequest) error
}

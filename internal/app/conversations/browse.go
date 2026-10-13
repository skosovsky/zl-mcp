package conversations

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type DirectoryRequest struct {
	Type, Query, Cursor string
	Limit               int
}
type Directory struct {
	Conversations   []shared.ConversationInfo `json:"conversations"`
	HasMore         bool                      `json:"has_more"`
	NextCursor      *string                   `json:"next_cursor"`
	CatalogComplete bool                      `json:"catalog_complete"`
	CatalogSources  []string                  `json:"catalog_sources"`
	EmptyReason     *string                   `json:"empty_reason"`
}
type Detail struct {
	Conversation    shared.ConversationInfo     `json:"conversation"`
	Coverage        shared.ConversationCoverage `json:"coverage"`
	CatalogComplete bool                        `json:"catalog_complete"`
}
type BrowseRequest struct {
	Conversation                domain.ConversationRef
	Since, Until, Order, Cursor string
	Limit                       int
}

// BrowseHit keeps the native stored timestamp spelling; search/context use
// their existing domain time values and different projections.
type BrowseHit struct {
	ConversationType string  `json:"conversation_type"`
	ConversationID   string  `json:"conversation_id"`
	ConversationName *string `json:"conversation_name"`
	MessageID        string  `json:"message_id"`
	SenderID         string  `json:"sender_id"`
	SenderName       *string `json:"sender_name"`
	SentAt           string  `json:"sent_at"`
	Excerpt          string  `json:"excerpt"`
	TextTruncated    bool    `json:"text_truncated"`
	TextResourceURI  *string `json:"text_resource_uri"`
	Direction        string  `json:"direction"`
}
type Browse struct {
	Messages           []BrowseHit                 `json:"messages"`
	HasMore            bool                        `json:"has_more"`
	NextCursor         *string                     `json:"next_cursor"`
	EmptyReason        *string                     `json:"empty_reason"`
	Coverage           shared.ConversationCoverage `json:"coverage"`
	SnapshotAt         string                      `json:"snapshot_at"`
	CoverageObservedAt string                      `json:"coverage_observed_at"`
	RequestedSince     *string                     `json:"requested_since"`
	RequestedUntil     *string                     `json:"requested_until"`
}
type DirectoryPort interface {
	Conversations(context.Context, DirectoryRequest) (Directory, error)
	Conversation(context.Context, domain.ConversationRef) (Detail, error)
	Browse(context.Context, BrowseRequest) (Browse, error)
}
type DirectoryExecution struct {
	Result  Directory
	request DirectoryRequest
	port    DirectoryPort
}

func (e *DirectoryExecution) Shorten(ctx context.Context) error {
	if len(e.Result.Conversations) < 2 {
		return domain.ResponseTooLarge(
			"One conversation record exceeds the response budget.",
			"Use a more specific filter or report the metadata size limitation.",
		)
	}
	e.request.Limit = len(e.Result.Conversations) - 1
	value, err := e.port.Conversations(ctx, e.request)
	if err == nil {
		e.Result = value
	}
	return err
}
func (h *Handler) Directory(ctx context.Context, r DirectoryRequest) (*DirectoryExecution, error) {
	result, err := h.DirectoryReads.Conversations(ctx, r)
	if err != nil {
		return nil, err
	}
	return &DirectoryExecution{Result: result, request: r, port: h.DirectoryReads}, nil
}
func (h *Handler) Conversation(ctx context.Context, ref domain.ConversationRef) (Detail, error) {
	return h.DirectoryReads.Conversation(ctx, ref)
}
func (h *Handler) Browse(ctx context.Context, r BrowseRequest) (Browse, error) {
	return h.DirectoryReads.Browse(ctx, r)
}

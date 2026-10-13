package history

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// StartRequest preserves optional bounds until the native executable contract and
// owner apply their defaults; explicit zero remains invalid at the schema gate.
type StartRequest struct {
	Source           string `json:"source,omitempty"`
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
	RequestID        string `json:"request_id"`
	Since            string `json:"since"`
	Until            string `json:"until"`
	PageSize         *int   `json:"page_size,omitempty"`
	MaxPages         *int   `json:"max_pages,omitempty"`
	MaxMessages      *int   `json:"max_messages,omitempty"`
}
type CommandPort interface {
	StartImport(context.Context, StartRequest) (domain.HistoryImportStatus, error)
	CancelImport(context.Context, string) (domain.HistoryImportStatus, error)
}

func (h *Handler) Start(ctx context.Context, request StartRequest) (domain.HistoryImportStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, handlerStartTimeout)
	defer cancel()
	return h.Commands.StartImport(ctx, request)
}
func (h *Handler) Cancel(ctx context.Context, id string) (domain.HistoryImportStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, handlerCancelTimeout)
	defer cancel()
	return h.Commands.CancelImport(ctx, id)
}

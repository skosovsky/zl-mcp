package conversations

import (
	"context"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type CollectorFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type CoverageSummary = shared.GroupCoverageSummary

type CollectorStatus struct {
	Authenticated      bool              `json:"authenticated"`
	CollectorState     string            `json:"collector_state"`
	LastConnectedAt    *string           `json:"last_connected_at"`
	LastEventAt        *string           `json:"last_event_at"`
	LastPersistedAt    *string           `json:"last_persisted_at"`
	EnabledGroupCount  int               `json:"enabled_group_count"`
	StoredMessageCount int               `json:"stored_message_count"`
	CoverageSummary    CoverageSummary   `json:"coverage_summary"`
	LastError          *CollectorFailure `json:"last_error"`
}
type Counts struct {
	Direct int `json:"direct"`
	Group  int `json:"group"`
}
type Collection struct {
	Mode               string `json:"mode"`
	ConversationCounts Counts `json:"conversation_counts"`
	MessageCounts      Counts `json:"message_counts"`
	CatalogComplete    bool   `json:"catalog_complete"`
	HistoryComplete    bool   `json:"history_complete"`
}
type ProfileDiagnostics struct {
	Status         string  `json:"status"`
	LastAttemptAt  *string `json:"last_attempt_at"`
	LastSuccessAt  *string `json:"last_success_at"`
	RequestedCount int     `json:"requested_count"`
	ReturnedCount  int     `json:"returned_count"`
	MissingCount   int     `json:"missing_count"`
	StopReason     *string `json:"stop_reason"`
}
type PreloadDiagnostics struct {
	Status           string  `json:"status"`
	LastAttemptAt    *string `json:"last_attempt_at"`
	LastSuccessAt    *string `json:"last_success_at"`
	ObservedCount    int     `json:"observed_count"`
	PermittedCount   int     `json:"permitted_count"`
	CatalogComplete  bool    `json:"catalog_complete"`
	MessagesImported bool    `json:"messages_imported"`
	StopReason       *string `json:"stop_reason"`
}
type CatalogDiagnostics struct {
	Status         string              `json:"status"`
	LastAttemptAt  *string             `json:"last_attempt_at"`
	LastSuccessAt  *string             `json:"last_success_at"`
	ObservedCount  int                 `json:"observed_count"`
	PermittedCount int                 `json:"permitted_count"`
	StopReason     *string             `json:"stop_reason"`
	Profiles       *ProfileDiagnostics `json:"profiles,omitempty"`
	Preload        *PreloadDiagnostics `json:"preload,omitempty"`
}
type GroupDetail struct {
	Group       domain.Group `json:"group"`
	Description *string      `json:"description"`
	Membership  string       `json:"membership"`
	UpdatedAt   string       `json:"updated_at"`
	Stale       bool         `json:"stale"`
}
type GroupRequest struct {
	Query  string
	Limit  int
	Cursor string
}
type GroupList struct {
	Groups           []domain.Group `json:"groups"`
	HasMore          bool           `json:"has_more"`
	NextCursor       *string        `json:"next_cursor"`
	EmptyReason      *string        `json:"empty_reason"`
	CatalogUpdatedAt *string        `json:"catalog_updated_at"`
	Stale            bool           `json:"stale"`
}
type CatalogPort interface {
	CollectorStatus(context.Context) (CollectorStatus, error)
	CollectionStatus(context.Context) (Collection, error)
	ContactDiagnostics(context.Context) (CatalogDiagnostics, error)
	ProfileDiagnostics(context.Context) (ProfileDiagnostics, error)
	PreloadDiagnostics(context.Context) (PreloadDiagnostics, error)
	Groups(context.Context, GroupRequest) (GroupList, error)
	Group(context.Context, string) (GroupDetail, error)
}
type LiveGroupPort interface {
	Group(context.Context, string) (GroupDetail, error)
}
type GroupExecution struct {
	Result  GroupList
	request GroupRequest
	port    CatalogPort
}

func (e *GroupExecution) Shorten(ctx context.Context) error {
	if len(e.Result.Groups) < 2 {
		return domain.ResponseTooLarge("One group record exceeds the response budget.", "Select another group or report that this record cannot fit within the response limit.")
	}
	e.request.Limit = len(e.Result.Groups) - 1
	v, err := e.port.Groups(ctx, e.request)
	if err == nil {
		e.Result = v
	}
	return err
}
func (h *Handler) Status(ctx context.Context) (CollectorStatus, error) {
	return h.Catalog.CollectorStatus(ctx)
}
func (h *Handler) Groups(ctx context.Context, request GroupRequest) (*GroupExecution, error) {
	result, err := h.Catalog.Groups(ctx, request)
	if err != nil {
		return nil, err
	}
	return &GroupExecution{Result: result, request: request, port: h.Catalog}, nil
}
func (h *Handler) Group(ctx context.Context, id string) (GroupDetail, error) {
	refresh, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.LiveGroups.Group(refresh, id)
	cancel()
	if err != nil {
		result, err = h.Catalog.Group(ctx, id)
		if err == nil {
			result.Stale = true
		}
	}
	return result, err
}

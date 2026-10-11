// Package archives exposes reads of retained sources; native owners retain all
// account, window, policy, TTL, tombstone and resource-token decisions.
package archives

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type Source struct {
	SourceID              string `json:"source_id"`
	CapturedAt            string `json:"captured_at"`
	CaptureCacheExpiresAt string `json:"capture_cache_expires_at"`
	PreservedAt           string `json:"preserved_at"`
	Retention             string `json:"retention"`
	Digest                string `json:"digest"`
	FileCount             int64  `json:"file_count"`
	DirectFiles           int64  `json:"direct_files"`
	GroupFiles            int64  `json:"group_files"`
	HistoryComplete       bool   `json:"history_complete"`
	ReadScope             string `json:"read_scope"`
}
type Sources struct {
	Sources         []Source `json:"sources"`
	HistoryComplete bool     `json:"history_complete"`
}
type DirectoryRequest struct {
	SourceID string `json:"source_id"`
	Type     string `json:"conversation_type,omitempty"`
	Query    string `json:"query,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
	Limit    int    `json:"limit"`
}
type Directory struct {
	Conversations   []shared.ConversationInfo `json:"conversations"`
	HasMore         bool                      `json:"has_more"`
	NextCursor      *string                   `json:"next_cursor"`
	CatalogComplete bool                      `json:"catalog_complete"`
	CatalogSources  []string                  `json:"catalog_sources"`
	EmptyReason     *string                   `json:"empty_reason"`
	Source          Source                    `json:"source"`
}
type BrowseRequest struct {
	SourceID         string `json:"source_id"`
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
	Since            string `json:"since"`
	Until            string `json:"until"`
	Order            string `json:"order,omitempty"`
	Cursor           string `json:"cursor,omitempty"`
	Limit            int    `json:"limit"`
}
type Record struct {
	ArchiveRowID        string  `json:"archive_row_id"`
	ZaloMessageID       *string `json:"zalo_message_id"`
	ConversationType    string  `json:"conversation_type"`
	ConversationID      string  `json:"conversation_id"`
	SenderID            *string `json:"sender_id"`
	SenderMapping       string  `json:"sender_mapping"`
	Direction           string  `json:"direction"`
	Timestamp           string  `json:"timestamp"`
	Text                string  `json:"text"`
	TextKind            string  `json:"text_kind"`
	QuoteAnchorEligible bool    `json:"quote_anchor_eligible"`
}
type Hit struct {
	Record
	TextTruncated bool    `json:"text_truncated"`
	ResourceURI   *string `json:"resource_uri"`
}
type Coverage struct {
	ConversationType          string           `json:"conversation_type"`
	ConversationID            string           `json:"conversation_id"`
	SourceRows                int64            `json:"source_rows"`
	PeriodRows                int64            `json:"period_rows"`
	InvalidTimestampRows      int64            `json:"invalid_timestamp_rows"`
	SourceEarliestAt          *string          `json:"source_earliest_at"`
	SourceLatestAt            *string          `json:"source_latest_at"`
	WALMode                   bool             `json:"wal_mode"`
	HistoryComplete           bool             `json:"history_complete"`
	Examined                  int64            `json:"examined"`
	Rejected                  int64            `json:"rejected"`
	Expired                   int64            `json:"expired"`
	SuppressedLive            int64            `json:"suppressed_live"`
	UnresolvedSenders         int64            `json:"unresolved_senders"`
	UnsupportedMetadataFields int64            `json:"unsupported_metadata_fields"`
	UnresolvedQuotes          int64            `json:"unresolved_quotes"`
	UnresolvedMentions        int64            `json:"unresolved_mentions"`
	UnsupportedContent        map[string]int64 `json:"unsupported_content"`
	CoverageScope             string           `json:"coverage_scope"`
	LiveVisibility            string           `json:"live_visibility"`
	SourceInformationRows     *int64           `json:"source_information_rows,omitempty"`
	SourceNativeExcludedRows  *int64           `json:"source_native_excluded_rows,omitempty"`
	SourceRecallRows          *int64           `json:"source_recall_rows,omitempty"`
	SuppressedSource          *int64           `json:"suppressed_source,omitempty"`
}
type Browse struct {
	Messages           []Hit    `json:"messages"`
	HasMore            bool     `json:"has_more"`
	NextCursor         *string  `json:"next_cursor"`
	EmptyReason        *string  `json:"empty_reason"`
	Coverage           Coverage `json:"coverage"`
	SnapshotAt         string   `json:"snapshot_at"`
	CoverageObservedAt string   `json:"coverage_observed_at"`
	RequestedSince     *string  `json:"requested_since"`
	RequestedUntil     *string  `json:"requested_until"`
	Source             Source   `json:"source"`
}
type ReadPort interface {
	Sources(context.Context) (Sources, error)
	Conversations(context.Context, DirectoryRequest) (Directory, error)
	Browse(context.Context, BrowseRequest) (Browse, error)
	Record(context.Context, string) (Record, error)
}
type Handler struct{ Reads ReadPort }
type DirectoryExecution struct {
	Result  Directory
	request DirectoryRequest
	port    ReadPort
}
type BrowseExecution struct {
	Result  Browse
	request BrowseRequest
	port    ReadPort
}

func (h *Handler) Sources(ctx context.Context) (Sources, error) { return h.Reads.Sources(ctx) }
func (h *Handler) Record(ctx context.Context, token string) (Record, error) {
	return h.Reads.Record(ctx, token)
}
func (h *Handler) Directory(ctx context.Context, q DirectoryRequest) (*DirectoryExecution, error) {
	result, err := h.Reads.Conversations(ctx, q)
	if err != nil {
		return nil, err
	}
	return &DirectoryExecution{Result: result, request: q, port: h.Reads}, nil
}
func (h *Handler) Browse(ctx context.Context, q BrowseRequest) (*BrowseExecution, error) {
	result, err := h.Reads.Browse(ctx, q)
	if err != nil {
		return nil, err
	}
	return &BrowseExecution{Result: result, request: q, port: h.Reads}, nil
}
func (e *DirectoryExecution) Shorten(ctx context.Context) error {
	if len(e.Result.Conversations) < 2 {
		return domain.ResponseTooLarge("One conversation record exceeds the response budget.", "Use a more specific filter or report the metadata size limitation.")
	}
	e.request.Limit = len(e.Result.Conversations) - 1
	result, err := e.port.Conversations(ctx, e.request)
	if err == nil {
		e.Result = result
	}
	return err
}
func (e *BrowseExecution) Shorten(ctx context.Context) error {
	if e.request.Limit < 2 {
		return domain.ResponseTooLarge("One message page exceeds the response budget.", "Use a smaller page or report the coverage size limitation.")
	}
	e.request.Limit /= 2
	result, err := e.port.Browse(ctx, e.request)
	if err == nil {
		e.Result = result
	}
	return err
}

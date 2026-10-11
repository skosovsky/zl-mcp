// Package search owns typed search/coverage assembly and invocation-local pages.
package search

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type Query struct {
	Text, SenderID, Since, Until, Cursor string
	Limit                                int
}
type GroupRequest struct {
	Query
	GroupID string
}
type ConversationRequest struct {
	Query
	Conversation domain.ConversationRef
}

// Page exposes business values and a narrow continuation capability. The adapter
// retains the native cursor; no native storage type or FTS requery crosses the port.
type Page struct {
	Hits       []domain.SearchHit
	NextCursor *string
	SnapshotAt string
	shorten    func(*Page, int) error
}

func NewPage(hits []domain.SearchHit, next *string, snapshot string, shorten func(*Page, int) error) *Page {
	return &Page{Hits: hits, NextCursor: next, SnapshotAt: snapshot, shorten: shorten}
}
func (p *Page) Shorten(n int) error {
	if p.shorten == nil {
		return domain.Invalid("Page continuation is unavailable.")
	}
	return p.shorten(p, n)
}

type GroupSummary struct {
	GroupCount          int  `json:"group_count"`
	HistoryComplete     bool `json:"history_complete"`
	GroupsWithKnownGaps int  `json:"groups_with_known_gaps"`
}
type ConversationSummary struct {
	ConversationCount          int  `json:"conversation_count"`
	HistoryComplete            bool `json:"history_complete"`
	ConversationsWithKnownGaps int  `json:"conversations_with_known_gaps"`
}

type GroupResult struct {
	Messages        []domain.SearchHit `json:"messages"`
	HasMore         bool               `json:"has_more"`
	NextCursor      *string            `json:"next_cursor"`
	EmptyReason     *string            `json:"empty_reason"`
	Coverage        []domain.Coverage  `json:"coverage"`
	CoverageSummary GroupSummary       `json:"coverage_summary"`
	SnapshotAt      string             `json:"snapshot_at"`
}
type ConversationResult struct {
	Messages        []shared.ConversationHit      `json:"messages"`
	HasMore         bool                          `json:"has_more"`
	NextCursor      *string                       `json:"next_cursor"`
	EmptyReason     *string                       `json:"empty_reason"`
	Coverage        []shared.ConversationCoverage `json:"coverage"`
	CoverageSummary ConversationSummary           `json:"coverage_summary"`
	SnapshotAt      string                        `json:"snapshot_at"`
}

type ReadPort interface {
	GroupPage(context.Context, GroupRequest) (*Page, error)
	ConversationPage(context.Context, ConversationRequest) (*Page, error)
	GroupCoverage(context.Context, string) (domain.Coverage, error)
	ConversationCoverage(context.Context, domain.ConversationRef) (domain.Coverage, error)
	GroupSummary(context.Context) (GroupSummary, error)
	ConversationSummary(context.Context) (ConversationSummary, error)
	StoredMessageCount(context.Context) (int, error)
}

type Handler struct{ Reads ReadPort }
type GroupExecution struct {
	Result  GroupResult
	request GroupRequest
	page    *Page
	reads   ReadPort
}
type ConversationExecution struct {
	Result  ConversationResult
	request ConversationRequest
	page    *Page
	reads   ReadPort
}

func (h *Handler) Group(ctx context.Context, r GroupRequest) (*GroupExecution, error) {
	p, err := h.Reads.GroupPage(ctx, r)
	if err != nil {
		return nil, err
	}
	result, err := groupResult(ctx, h.Reads, r, p)
	if err != nil {
		return nil, err
	}
	return &GroupExecution{Result: result, request: r, page: p, reads: h.Reads}, nil
}
func (h *Handler) Conversation(ctx context.Context, r ConversationRequest) (*ConversationExecution, error) {
	p, err := h.Reads.ConversationPage(ctx, r)
	if err != nil {
		return nil, err
	}
	result, err := conversationResult(ctx, h.Reads, r, p)
	if err != nil {
		return nil, err
	}
	return &ConversationExecution{Result: result, request: r, page: p, reads: h.Reads}, nil
}
func (e *GroupExecution) Shorten(ctx context.Context) error {
	if len(e.page.Hits) < 2 {
		return domain.ResponseTooLarge("One search record or its coverage exceeds the response budget.", "Select a different group with less metadata. If a selected group still exceeds the limit, report the limitation rather than repeating the same query.")
	}
	if err := e.page.Shorten(len(e.page.Hits) - 1); err != nil {
		return err
	}
	result, err := groupResult(ctx, e.reads, e.request, e.page)
	if err == nil {
		e.Result = result
	}
	return err
}
func (e *ConversationExecution) Shorten(ctx context.Context) error {
	if len(e.page.Hits) < 2 {
		return domain.ResponseTooLarge("One search record or coverage exceeds the response budget.", "Select a conversation with less metadata or report the limitation.")
	}
	if err := e.page.Shorten(len(e.page.Hits) - 1); err != nil {
		return err
	}
	result, err := conversationResult(ctx, e.reads, e.request, e.page)
	if err == nil {
		e.Result = result
	}
	return err
}

func groupResult(ctx context.Context, reads ReadPort, r GroupRequest, p *Page) (GroupResult, error) {
	out := GroupResult{Messages: p.Hits, HasMore: p.NextCursor != nil, NextCursor: p.NextCursor, Coverage: []domain.Coverage{}, SnapshotAt: p.SnapshotAt}
	seen := map[string]bool{}
	for _, hit := range p.Hits {
		if seen[hit.GroupID] {
			continue
		}
		seen[hit.GroupID] = true
		coverage, err := reads.GroupCoverage(ctx, hit.GroupID)
		if err != nil {
			return out, err
		}
		out.Coverage = append(out.Coverage, coverage)
	}
	if len(p.Hits) == 0 && r.GroupID != "" {
		c, err := reads.GroupCoverage(ctx, r.GroupID)
		if err != nil {
			return out, err
		}
		out.Coverage = append(out.Coverage, c)
	}
	if len(p.Hits) == 0 {
		reason := "no_matches"
		count, err := reads.StoredMessageCount(ctx)
		if err != nil {
			return out, err
		}
		if count == 0 || (r.GroupID != "" && len(out.Coverage) == 1 && out.Coverage[0].EarliestStoredAt == nil) {
			reason = "no_collected_data"
		}
		out.EmptyReason = &reason
	}
	summary, err := reads.GroupSummary(ctx)
	out.CoverageSummary = summary
	return out, err
}
func conversationResult(ctx context.Context, reads ReadPort, r ConversationRequest, p *Page) (ConversationResult, error) {
	out := ConversationResult{Messages: []shared.ConversationHit{}, HasMore: p.NextCursor != nil, NextCursor: p.NextCursor, Coverage: []shared.ConversationCoverage{}, SnapshotAt: p.SnapshotAt}
	seen := map[domain.ConversationRef]bool{}
	for _, hit := range p.Hits {
		out.Messages = append(out.Messages, shared.Hit(hit))
		if seen[hit.Conversation] {
			continue
		}
		seen[hit.Conversation] = true
		c, err := reads.ConversationCoverage(ctx, hit.Conversation)
		if err != nil {
			return out, err
		}
		out.Coverage = append(out.Coverage, shared.Coverage(hit.Conversation, c))
	}
	if len(p.Hits) == 0 {
		reason := "no_matches"
		if r.Conversation.ID != "" {
			c, err := reads.ConversationCoverage(ctx, r.Conversation)
			if err != nil {
				return out, err
			}
			out.Coverage = append(out.Coverage, shared.Coverage(r.Conversation, c))
			if c.EarliestStoredAt == nil {
				reason = "no_collected_data"
			}
		} else {
			count, err := reads.StoredMessageCount(ctx)
			if err != nil {
				return out, err
			}
			if count == 0 {
				reason = "no_collected_data"
			}
		}
		out.EmptyReason = &reason
	}
	summary, err := reads.ConversationSummary(ctx)
	out.CoverageSummary = summary
	return out, err
}

// AccessPort preserves native preflight/cursor error ordering before dispatch.
type AccessPort interface {
	CheckGroupSearch(context.Context, GroupRequest) error
	CheckConversationSearch(context.Context, ConversationRequest) error
}

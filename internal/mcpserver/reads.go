package mcpserver

import (
	"encoding/json"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// ReadPorts are supplied by composition; MCP never constructs a storage adapter.
type ReadPorts struct {
	Search        *search.Handler
	Conversations *conversations.Handler
}

func searchQuery(args map[string]any) search.Query {
	return search.Query{Text: str(args, "query"), SenderID: str(args, "sender_id"), Since: str(args, "since"), Until: str(args, "until"), Limit: integer(args, "limit", 20), Cursor: str(args, "cursor")}
}
func groupSearchRequest(args map[string]any) search.GroupRequest {
	return search.GroupRequest{Query: searchQuery(args), GroupID: str(args, "group_id")}
}
func conversationSearchRequest(args map[string]any) search.ConversationRequest {
	return search.ConversationRequest{Query: searchQuery(args), Conversation: conversationRef(args)}
}
func contextRequest(args map[string]any, legacy bool) conversations.ContextRequest {
	ref := conversationRef(args)
	if legacy {
		ref = domain.ConversationRef{Type: domain.ConversationGroup, ID: str(args, "group_id")}
	}
	return conversations.ContextRequest{Conversation: ref, MessageID: str(args, "message_id"), Before: integer(args, "before", 5), After: integer(args, "after", 5)}
}
func groupSearchResult(v search.GroupResult) map[string]any {
	summary := map[string]any{"group_count": v.CoverageSummary.GroupCount, "history_complete": v.CoverageSummary.HistoryComplete, "groups_with_known_gaps": v.CoverageSummary.GroupsWithKnownGaps}
	return map[string]any{"messages": v.Messages, "has_more": v.HasMore, "next_cursor": v.NextCursor, "empty_reason": v.EmptyReason, "coverage": v.Coverage, "coverage_summary": summary, "snapshot_at": v.SnapshotAt}
}
func groupContextResult(v conversations.GroupContext) map[string]any {
	return map[string]any{"anchor": v.Anchor, "before": v.Before, "after": v.After, "reply_to": v.ReplyTo, "coverage": v.Coverage, "context_truncated": v.ContextTruncated, "omitted_before_id": v.OmittedBeforeID, "omitted_after_id": v.OmittedAfterID, "next_action": v.NextAction}
}

// Existing wire maps remain confined to transport projection.
func conversationReadResult(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(raw, &out)
	return out, err
}

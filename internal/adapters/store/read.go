// Package store adapts the existing borrowed storage owner to consumer read ports.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type Reader struct{ owner *storage.Store }

func New(owner *storage.Store) *Reader { return &Reader{owner: owner} }

var _ search.ReadPort = (*Reader)(nil)
var _ conversations.ReadPort = (*Reader)(nil)

func readError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return &domain.Error{
			Code:       "NOT_FOUND",
			Message:    "No accessible local record was found.",
			NextAction: domain.NextAction{Instruction: "Use IDs returned by zalo_list_groups or zalo_search_messages."},
			Details:    map[string]any{},
		}
	}
	return err
}
func query(q search.Query) storage.Search {
	return storage.Search{
		Query:    q.Text,
		SenderID: q.SenderID,
		Since:    q.Since,
		Until:    q.Until,
		Limit:    q.Limit,
		Cursor:   q.Cursor,
	}
}
func (r *Reader) GroupPage(ctx context.Context, input search.GroupRequest) (*search.Page, error) {
	q := query(input.Query)
	q.GroupID = input.GroupID
	return r.page(ctx, q)
}
func (r *Reader) ConversationPage(ctx context.Context, input search.ConversationRequest) (*search.Page, error) {
	q := query(input.Query)
	q.General = true
	q.ConversationType = input.Conversation.Type
	q.ConversationID = input.Conversation.ID
	return r.page(ctx, q)
}
func (r *Reader) page(ctx context.Context, q storage.Search) (*search.Page, error) {
	native, err := r.owner.Page(ctx, q)
	if err != nil {
		return nil, readError(err)
	}
	return search.NewPage(native.Hits, native.NextCursor, native.SnapshotAt, func(page *search.Page, n int) error {
		if err := r.owner.ShortenPage(native, n); err != nil {
			return readError(err)
		}
		page.Hits = native.Hits
		page.NextCursor = native.NextCursor
		page.SnapshotAt = native.SnapshotAt
		return nil
	}), nil
}
func (r *Reader) GroupCoverage(ctx context.Context, id string) (domain.Coverage, error) {
	c, err := r.owner.Coverage(ctx, id)
	return c, readError(err)
}
func (r *Reader) ConversationCoverage(ctx context.Context, ref domain.ConversationRef) (domain.Coverage, error) {
	c, err := r.owner.ConversationCoverageValue(ctx, ref)
	return c, readError(err)
}
func (r *Reader) GroupSummary(ctx context.Context) (search.GroupSummary, error) {
	value, err := r.owner.Summary(ctx)
	if err != nil {
		return search.GroupSummary{}, readError(err)
	}
	groupCount, groupCountOK := value["group_count"].(int)
	historyComplete, historyCompleteOK := value["history_complete"].(bool)
	groupsWithKnownGaps, groupsWithKnownGapsOK := value["groups_with_known_gaps"].(int)
	if !groupCountOK || !historyCompleteOK || !groupsWithKnownGapsOK {
		return search.GroupSummary{}, errors.New("invalid storage GroupSummary projection")
	}

	return search.GroupSummary{
		GroupCount:          groupCount,
		HistoryComplete:     historyComplete,
		GroupsWithKnownGaps: groupsWithKnownGaps,
	}, nil
}
func (r *Reader) ConversationSummary(ctx context.Context) (search.ConversationSummary, error) {
	value, err := r.owner.ConversationSummary(ctx)
	if err != nil {
		return search.ConversationSummary{}, readError(err)
	}
	conversationCount, conversationCountOK := value["conversation_count"].(int)
	historyComplete, historyCompleteOK := value["history_complete"].(bool)
	conversationsWithKnownGaps, conversationsWithKnownGapsOK := value["conversations_with_known_gaps"].(int)
	if !conversationCountOK || !historyCompleteOK || !conversationsWithKnownGapsOK {
		return search.ConversationSummary{}, errors.New("invalid storage ConversationSummary projection")
	}

	return search.ConversationSummary{
		ConversationCount:          conversationCount,
		HistoryComplete:            historyComplete,
		ConversationsWithKnownGaps: conversationsWithKnownGaps,
	}, nil
}
func (r *Reader) StoredMessageCount(ctx context.Context) (int, error) {
	value, err := r.owner.State(ctx)
	if err != nil {
		return 0, readError(err)
	}
	count, ok := value["stored_message_count"].(int)
	if !ok {
		return 0, errors.New("invalid stored message count projection")
	}
	return count, nil
}
func optionalID(value any) *string {
	if id, ok := value.(string); ok {
		return &id
	}
	return nil
}

func (r *Reader) GroupContext(
	ctx context.Context,
	input conversations.ContextRequest,
) (conversations.GroupContext, error) {
	value, err := r.owner.Context(ctx, input.Conversation.ID, input.MessageID, input.Before, input.After)
	if err != nil {
		return conversations.GroupContext{}, readError(err)
	}
	anchor, anchorOK := value["anchor"].(domain.Message)
	before, beforeOK := value["before"].([]domain.Message)
	after, afterOK := value["after"].([]domain.Message)
	replyTo, replyToOK := value["reply_to"].(*domain.Message)
	coverage, coverageOK := value["coverage"].(domain.Coverage)
	contextTruncated, contextTruncatedOK := value["context_truncated"].(bool)
	if !anchorOK || !beforeOK || !afterOK || !replyToOK || !coverageOK || !contextTruncatedOK {
		return conversations.GroupContext{}, errors.New("invalid storage GroupContext projection")
	}
	out := conversations.GroupContext{
		Anchor:           anchor,
		Before:           before,
		After:            after,
		ReplyTo:          replyTo,
		Coverage:         coverage,
		ContextTruncated: contextTruncated,
		OmittedBeforeID:  optionalID(value["omitted_before_id"]),
		OmittedAfterID:   optionalID(value["omitted_after_id"]),
	}
	if action, ok := value["next_action"].(domain.NextAction); ok {
		out.NextAction = &action
	}
	return out, nil
}

func (r *Reader) ConversationContext(
	ctx context.Context,
	input conversations.ContextRequest,
) (conversations.ConversationContext, error) {
	value, err := r.owner.ConversationContext(ctx, input.Conversation, input.MessageID, input.Before, input.After)
	if err != nil {
		return conversations.ConversationContext{}, readError(err)
	}
	// The existing legacy conversation view already owns its clipping/projection.
	// Its map is converted here, never exposed as an application API.
	raw, err := json.Marshal(value)
	if err != nil {
		return conversations.ConversationContext{}, err
	}
	var out conversations.ConversationContext
	err = json.Unmarshal(raw, &out)
	return out, err
}
func (r *Reader) Message(ctx context.Context, input conversations.MessageRequest) (domain.Message, error) {
	var value domain.Message
	var err error
	if input.LegacyGroup {
		value, err = r.owner.Message(ctx, input.Conversation.ID, input.MessageID)
	} else {
		value, err = r.owner.ConversationMessage(ctx, input.Conversation, input.MessageID)
	}
	return value, readError(err)
}

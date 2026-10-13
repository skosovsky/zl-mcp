package evaluation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/evalfixture"
)

const requestID = "00000000-0000-4000-8000-000000000002"
const collisionID = "9007199254740993"

// ChildOptions contains only synthetic paths allocated by the parent.
type ChildOptions struct{ CaseID, StateDir, EffectsFile, Corpus, ArchiveVector string }

//nolint:cyclop,funlen,gocognit,gocyclo,mnd,nonamedreturns // The closed synthetic scenario switch keeps each ordered AAA/restart trace and its shared cleanup owner together; assertions grade the complete scenario. Deferred cleanup retains the returned result/error. Literal query limits are independent synthetic scenario inputs.
func RunCase(ctx context.Context, opts ChildOptions) (out Output, err error) {
	out.CaseID = opts.CaseID
	out.Facts = map[string]any{}
	//nolint:exhaustruct_v5 // Fixture flags and New fill the remaining defaults; preserve unset optional policy/effect options.
	options := evalfixture.Options{
		Offline:     true,
		StateDir:    opts.StateDir,
		KeepState:   true,
		EffectsFile: opts.EffectsFile,
		Fixtures:    opts.Corpus,
	}
	if opts.CaseID == "empty-corpus" {
		options.Empty = true
	}
	if opts.CaseID == "send-forbidden" {
		options.DenySend = true
	}
	if opts.CaseID == "send-unknown-restart" {
		options.SendTimeout = true
	}
	if opts.CaseID == fixtureArchiveDatesID || opts.CaseID == fixtureArchiveEmptyPageID {
		options.ArchiveVector = opts.ArchiveVector
	}
	f, err := evalfixture.New(ctx, options)
	if err != nil {
		return out, err
	}
	c, err := connect(ctx, f)
	if err != nil {
		//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
		f.Close()
		return out, err
	}
	defer func() {
		if c != nil {
			out.Trace = c.trace
			c.close()
		}
		var closeErr error
		if f != nil {
			closeErr = f.Close()
		}
		out.Cleanup = closeErr == nil
		if err == nil {
			err = closeErr
		}
		out.Complete = err == nil
	}()
	tool := func(name string, args any) map[string]any {
		if err != nil || c == nil {
			return nil
		}
		var r map[string]any
		r, err = c.tool(ctx, name, args)
		return r
	}
	restart := func(policy *domain.CollectionPolicy) {
		previous := c.trace
		c.close()
		if e := f.Close(); e != nil {
			err = e
			return
		}
		options.Policy = policy
		f, err = evalfixture.New(ctx, options)
		if err != nil {
			return
		}
		c, err = connect(ctx, f)
		if err == nil {
			c.appendTrace(previous)
		}
	}
	search := func() { tool(toolSearchMessages, map[string]any{fieldQuery: fixtureSearchText, fieldLimit: 10}) }
	send := func(text string, quote string) {
		args := map[string]any{"recipient_id": fixturePeerID, fieldText: text, fieldRequestID: requestID}
		if quote != "" {
			args["reply_to_message_id"] = quote
		}
		tool("zalo_send_direct_message", args)
	}
	switch opts.CaseID {
	case "normal-search", "gaps", "injection":
		search()
	case "same-name":
		tool("zalo_list_conversations", map[string]any{fieldQuery: "Команда"})
	case "typed-collision", "numeric-precision":
		if opts.CaseID == "numeric-precision" {
			for _, raw := range []string{`9007199254740993`, `"9007199254740993"`} {
				msg, e := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":` + raw + `,"method":"tools/list"}`))
				if e != nil {
					err = e
					break
				}
				request, ok := msg.(*jsonrpc.Request)
				if !ok {
					err = errors.New("SDK did not decode a request")
					break
				}
				out.Facts["sdk-envelope-"+raw] = string(marshaled(request.ID.Raw()))
			}
		}
		for _, kind := range []string{domain.ConversationDirect, domain.ConversationGroup} {
			tool(
				"zalo_list_conversation_messages",
				map[string]any{fieldConversationType: kind, fieldConversationID: collisionID},
			)
		}
	case "empty-corpus":
		search()
	case "large-resource":
		r := tool(
			toolGetMessageContext,
			map[string]any{"group_id": fixtureG1ID, "message_id": "m-long", "before": 0, "after": 0},
		)
		if err == nil {
			c.resource(ctx, stringAt(payload(r), "anchor.text_resource_uri"))
		}
	case "pagination":
		args := map[string]any{fieldQuery: fixtureSearchText, fieldLimit: 1}
		r := tool(toolSearchMessages, args)
		cursor := stringAt(payload(r), "next_cursor")
		args["cursor"] = cursor
		tool(toolSearchMessages, args)
		args[fieldQuery] = "релиз"
		tool(toolSearchMessages, args)
	case fixtureArchiveDatesID, fixtureArchiveEmptyPageID:
		r := tool("zalo_list_archive_sources", map[string]any{})
		source := stringAt(payload(r), "sources.0.source_id")
		if opts.CaseID == fixtureArchiveDatesID {
			tool(
				"zalo_list_conversation_messages",
				map[string]any{
					"source_id":           source,
					fieldConversationType: domain.ConversationDirect,
					fieldConversationID:   "12",
				},
			)
		}
		args := map[string]any{
			"source_id":           source,
			fieldConversationType: domain.ConversationDirect,
			fieldConversationID:   "12",
			fieldSince:            "2000-01-01T00:00:00Z",
			fieldUntil:            "2030-01-01T00:00:00Z",
			"order":               "asc",
			fieldLimit:            1,
		}
		if opts.CaseID == fixtureArchiveEmptyPageID {
			args["order"] = "desc"
		}
		emptyMore := false
		seen := map[string]bool{}
		fullResource := false
		for index := 0; index < 20 && err == nil; index++ {
			r = tool("zalo_list_conversation_messages", args)
			p := payload(r)
			items, _ := p["messages"].([]any)
			if len(items) == 0 && p["has_more"] == true {
				emptyMore = true
			}
			for _, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					err = errors.New("invalid archive row")
					break
				}
				id, ok := m["archive_row_id"].(string)
				if !ok {
					err = errors.New("invalid archive row ID")
					break
				}
				if seen[id] {
					err = errors.New("duplicate archive row")
				}
				seen[id] = true
				if uri, ok := m["text_resource_uri"].(string); ok && !fullResource {
					c.resource(ctx, uri)
					fullResource = true
				}
			}
			if p["has_more"] != true {
				break
			}
			args["cursor"] = p["next_cursor"]
		}
		out.Facts["empty_page_with_more"] = emptyMore
		out.Facts["archive_rows"] = len(seen)
	case "ttl":
		ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: "ttl-peer"}
		now := time.Now().UTC()
		_, err = f.Store.PutExpiringHistoryPage(
			ctx,
			ref,
			[]domain.ExpiringHistoryRecord{
				{
					//nolint:exhaustruct_v5 // Message preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
					Message: domain.Message{
						Conversation: ref,
						ID:           "ttl-message",
						SenderID:     fixturePeerID,
						Text:         "expiring",
						SentAt:       now.Add(-time.Hour),
					},
					ExpiresAtMS: now.Add(time.Hour).UnixMilli(),
				},
			},
		)
		tool(
			"zalo_list_conversation_messages",
			map[string]any{fieldConversationType: domain.ConversationDirect, fieldConversationID: ref.ID},
		)
		if err == nil {
			err = f.Store.ExpireHistoryAt(ctx, now.Add(2*time.Hour))
		}
		tool(
			"zalo_list_conversation_messages",
			map[string]any{fieldConversationType: domain.ConversationDirect, fieldConversationID: ref.ID},
		)
	case "tombstone":
		args := map[string]any{fieldConversationType: domain.ConversationDirect, fieldConversationID: collisionID}
		tool("zalo_list_conversation_messages", args)
		if err == nil {
			err = f.Store.DeleteConversation(
				ctx,
				domain.ConversationRef{Type: domain.ConversationDirect, ID: collisionID},
				"9007199254740995",
			)
		}
		restart(nil)
		tool("zalo_list_conversation_messages", args)
	case "policy-revoke":
		args := map[string]any{fieldConversationType: domain.ConversationDirect, fieldConversationID: collisionID}
		tool("zalo_list_conversation_messages", args)
		restart(
			//nolint:exhaustruct_v5 // CollectionPolicy preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
			&domain.CollectionPolicy{
				Selected: map[domain.ConversationRef]bool{{Type: domain.ConversationGroup, ID: fixtureG1ID}: true},
			},
		)
		tool("zalo_list_conversation_messages", args)
		if err == nil {
			c.resource(ctx, "zalo://conversations/direct/9007199254740993/messages/9007199254740995")
		}
	case "malformed-normalization":
		tool(toolSearchMessages, map[string]any{fieldQuery: fixtureSearchText, fieldLimit: "10"})
		tool(toolSearchMessages, map[string]any{fieldQuery: fixtureSearchText, fieldLimit: 0})
		tool(toolSearchMessages, map[string]any{fieldQuery: fixtureSearchText, fieldSince: "bad-date"})
		tool(
			"zalo_send_direct_message",
			map[string]any{"recipient_id": fixturePeerID, fieldText: "synthetic", fieldRequestID: "bad-uuid"},
		)
		tool("zalo_get_status", map[string]any{"extra": nil})
	case "send-allowed":
		send("authorized synthetic text", "")
	case "send-forbidden":
		send("authorized synthetic text", "")
	case "cross-quote":
		send("authorized synthetic quote", "foreign-quote")
	case "send-idempotency":
		send("authorized synthetic text", "")
		send("authorized synthetic text", "")
		send("different text", "")
	case "send-unknown-restart":
		send("authorized synthetic text", "")
		restart(nil)
		tool("zalo_get_send_status", map[string]any{fieldRequestID: requestID})
		send("authorized synthetic text", "")
	case "join-no-approval":
		tool(toolJoinGroup, map[string]any{fieldRequestID: requestID, "plan_token": "unapproved"})
	case "join-approved":
		var token string
		token, err = f.ApproveSyntheticJoin(ctx)
		r := tool(toolJoinGroup, map[string]any{fieldRequestID: requestID, "plan_token": token})
		operation := stringAt(payload(r), fieldOperationID)
		// Poll saved status only; never repeat the effect to obtain a terminal response.
		for i := 0; i < 10 && err == nil; i++ {
			r = tool("zalo_get_join_status", map[string]any{fieldOperationID: operation})
			if payload(r)["status"] == "joined" {
				break
			}
		}
	case "history-cancel-restart":
		before, e := f.EventCount(ctx)
		if e != nil {
			err = e
			break
		}
		r := tool(
			"zalo_import_conversation_history",
			map[string]any{
				fieldRequestID:        requestID,
				fieldConversationType: domain.ConversationGroup,
				fieldConversationID:   fixtureG1ID,
				fieldSince:            "2026-10-01T00:00:00Z",
				fieldUntil:            "2026-10-02T00:00:00Z",
			},
		)
		operation := stringAt(payload(r), fieldOperationID)
		started, release, e := f.StartBlockedHistory(ctx)
		if e != nil {
			err = e
			break
		}
		select {
		case <-started:
		case <-ctx.Done():
			err = ctx.Err()
		}
		if err != nil {
			release()
			break
		}
		tool("zalo_cancel_history_import", map[string]any{fieldOperationID: operation})
		release()
		restart(nil)
		tool("zalo_get_history_import_status", map[string]any{fieldOperationID: operation})
		tool(
			"zalo_import_conversation_history",
			map[string]any{
				fieldRequestID:        requestID,
				fieldConversationType: domain.ConversationGroup,
				fieldConversationID:   fixtureG1ID,
				fieldSince:            "2026-10-01T00:00:00Z",
				fieldUntil:            "2026-10-02T00:00:00Z",
			},
		)
		if err != nil {
			break
		}
		after, e := f.EventCount(ctx)
		if e != nil {
			err = e
		}
		out.Facts["history_event_delta"] = after - before
		m, e := f.Store.Message(ctx, fixtureG1ID, "late-history")
		out.Facts["late_history_visible"] = e == nil && m.ID != ""
	case "events-redelivery":
		if err = f.PrepareSyntheticEvents(ctx); err != nil {
			break
		}
		args := map[string]any{"subscription_id": "synthetic-sub", fieldLimit: 20}
		first := tool("zalo_read_subscription_events", args)
		tool("zalo_read_subscription_events", args)
		receipt := stringAt(payload(first), "events.0.receipt")
		ack := map[string]any{"subscription_id": "synthetic-sub", "receipt": receipt}
		tool("zalo_ack_subscription_events", ack)
		tool("zalo_ack_subscription_events", ack)
		restart(nil)
		tool("zalo_read_subscription_events", args)
	case "malformed-resource":
		for _, uri := range []string{"zalo://groups/g-1/messages/m-101?extra=1", "zalo://conversations/direct/9007199254740993/messages/9007199254740995#fragment", "zalo://archives/not-a-valid-token"} {
			c.resource(ctx, uri)
		}
	default:
		err = fmt.Errorf("unknown dataset case %q", opts.CaseID)
	}
	return out, err
}

// DefaultChildOptions selects synthetic fixture assets without accepting live state paths.
func DefaultChildOptions(root, state, id string) ChildOptions {
	return ChildOptions{
		CaseID:        id,
		StateDir:      filepath.Join(state, "state"),
		EffectsFile:   filepath.Join(state, "effects.jsonl"),
		Corpus:        filepath.Join(root, "docs/evals/fixtures.json"),
		ArchiveVector: filepath.Join(root, "internal/mobilebackup/testdata/format1-account-vector.json"),
	}
}

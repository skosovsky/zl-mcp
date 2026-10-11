package evaluation

import (
	"context"
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

func RunCase(ctx context.Context, opts ChildOptions) (out Output, err error) {
	out.CaseID = opts.CaseID
	out.Facts = map[string]any{}
	options := evalfixture.Options{Offline: true, StateDir: opts.StateDir, KeepState: true, EffectsFile: opts.EffectsFile, Fixtures: opts.Corpus}
	if opts.CaseID == "empty-corpus" {
		options.Empty = true
	}
	if opts.CaseID == "send-forbidden" {
		options.DenySend = true
	}
	if opts.CaseID == "send-unknown-restart" {
		options.SendTimeout = true
	}
	if opts.CaseID == "archive-dates" || opts.CaseID == "archive-empty-page" {
		options.ArchiveVector = opts.ArchiveVector
	}
	f, err := evalfixture.New(ctx, options)
	if err != nil {
		return out, err
	}
	c, err := connect(ctx, f)
	if err != nil {
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
	search := func() { tool("zalo_search_messages", map[string]any{"query": "ремонт", "limit": 10}) }
	send := func(text string, quote string) {
		args := map[string]any{"recipient_id": "peer", "text": text, "request_id": requestID}
		if quote != "" {
			args["reply_to_message_id"] = quote
		}
		tool("zalo_send_direct_message", args)
	}
	switch opts.CaseID {
	case "normal-search", "gaps", "injection":
		search()
	case "same-name":
		tool("zalo_list_conversations", map[string]any{"query": "Команда"})
	case "typed-collision", "numeric-precision":
		if opts.CaseID == "numeric-precision" {
			for _, raw := range []string{`9007199254740993`, `"9007199254740993"`} {
				msg, e := jsonrpc.DecodeMessage([]byte(`{"jsonrpc":"2.0","id":` + raw + `,"method":"tools/list"}`))
				if e != nil {
					err = e
					break
				}
				out.Facts["sdk-envelope-"+raw] = string(marshaled(msg.(*jsonrpc.Request).ID.Raw()))
			}
		}
		for _, kind := range []string{"direct", "group"} {
			tool("zalo_list_conversation_messages", map[string]any{"conversation_type": kind, "conversation_id": collisionID})
		}
	case "empty-corpus":
		search()
	case "large-resource":
		r := tool("zalo_get_message_context", map[string]any{"group_id": "g-1", "message_id": "m-long", "before": 0, "after": 0})
		if err == nil {
			c.resource(ctx, stringAt(payload(r), "anchor.text_resource_uri"))
		}
	case "pagination":
		args := map[string]any{"query": "ремонт", "limit": 1}
		r := tool("zalo_search_messages", args)
		cursor := stringAt(payload(r), "next_cursor")
		args["cursor"] = cursor
		tool("zalo_search_messages", args)
		args["query"] = "релиз"
		tool("zalo_search_messages", args)
	case "archive-dates", "archive-empty-page":
		r := tool("zalo_list_archive_sources", map[string]any{})
		source := stringAt(payload(r), "sources.0.source_id")
		if opts.CaseID == "archive-dates" {
			tool("zalo_list_conversation_messages", map[string]any{"source_id": source, "conversation_type": "direct", "conversation_id": "12"})
		}
		args := map[string]any{"source_id": source, "conversation_type": "direct", "conversation_id": "12", "since": "2000-01-01T00:00:00Z", "until": "2030-01-01T00:00:00Z", "order": "asc", "limit": 1}
		if opts.CaseID == "archive-empty-page" {
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
				m := item.(map[string]any)
				id := m["archive_row_id"].(string)
				if seen[id] {
					err = fmt.Errorf("duplicate archive row")
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
		ref := domain.ConversationRef{Type: "direct", ID: "ttl-peer"}
		now := time.Now().UTC()
		_, err = f.Store.PutExpiringHistoryPage(ctx, ref, []domain.ExpiringHistoryRecord{{Message: domain.Message{Conversation: ref, ID: "ttl-message", SenderID: "peer", Text: "expiring", SentAt: now.Add(-time.Hour)}, ExpiresAtMS: now.Add(time.Hour).UnixMilli()}})
		tool("zalo_list_conversation_messages", map[string]any{"conversation_type": "direct", "conversation_id": ref.ID})
		if err == nil {
			err = f.Store.ExpireHistoryAt(ctx, now.Add(2*time.Hour))
		}
		tool("zalo_list_conversation_messages", map[string]any{"conversation_type": "direct", "conversation_id": ref.ID})
	case "tombstone":
		args := map[string]any{"conversation_type": "direct", "conversation_id": collisionID}
		tool("zalo_list_conversation_messages", args)
		if err == nil {
			err = f.Store.DeleteConversation(ctx, domain.ConversationRef{Type: "direct", ID: collisionID}, "9007199254740995")
		}
		restart(nil)
		tool("zalo_list_conversation_messages", args)
	case "policy-revoke":
		args := map[string]any{"conversation_type": "direct", "conversation_id": collisionID}
		tool("zalo_list_conversation_messages", args)
		restart(&domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{{Type: "group", ID: "g-1"}: true}})
		tool("zalo_list_conversation_messages", args)
		if err == nil {
			c.resource(ctx, "zalo://conversations/direct/9007199254740993/messages/9007199254740995")
		}
	case "malformed-normalization":
		tool("zalo_search_messages", map[string]any{"query": "ремонт", "limit": "10"})
		tool("zalo_search_messages", map[string]any{"query": "ремонт", "limit": 0})
		tool("zalo_search_messages", map[string]any{"query": "ремонт", "since": "bad-date"})
		tool("zalo_send_direct_message", map[string]any{"recipient_id": "peer", "text": "synthetic", "request_id": "bad-uuid"})
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
		tool("zalo_get_send_status", map[string]any{"request_id": requestID})
		send("authorized synthetic text", "")
	case "join-no-approval":
		tool("zalo_join_group", map[string]any{"request_id": requestID, "plan_token": "unapproved"})
	case "join-approved":
		var token string
		token, err = f.ApproveSyntheticJoin(ctx)
		r := tool("zalo_join_group", map[string]any{"request_id": requestID, "plan_token": token})
		operation := stringAt(payload(r), "operation_id")
		// Poll saved status only; never repeat the effect to obtain a terminal response.
		for i := 0; i < 10 && err == nil; i++ {
			r = tool("zalo_get_join_status", map[string]any{"operation_id": operation})
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
		r := tool("zalo_import_conversation_history", map[string]any{"request_id": requestID, "conversation_type": "group", "conversation_id": "g-1", "since": "2026-10-01T00:00:00Z", "until": "2026-10-02T00:00:00Z"})
		operation := stringAt(payload(r), "operation_id")
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
		tool("zalo_cancel_history_import", map[string]any{"operation_id": operation})
		release()
		restart(nil)
		tool("zalo_get_history_import_status", map[string]any{"operation_id": operation})
		tool("zalo_import_conversation_history", map[string]any{"request_id": requestID, "conversation_type": "group", "conversation_id": "g-1", "since": "2026-10-01T00:00:00Z", "until": "2026-10-02T00:00:00Z"})
		if err != nil {
			break
		}
		after, e := f.EventCount(ctx)
		if e != nil {
			err = e
		}
		out.Facts["history_event_delta"] = after - before
		m, e := f.Store.Message(ctx, "g-1", "late-history")
		out.Facts["late_history_visible"] = e == nil && m.ID != ""
	case "events-redelivery":
		if err = f.PrepareSyntheticEvents(ctx); err != nil {
			break
		}
		args := map[string]any{"subscription_id": "synthetic-sub", "limit": 20}
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

// Paths are synthetic fixture assets; no live state path is accepted implicitly.
func DefaultChildOptions(root, state, id string) ChildOptions {
	return ChildOptions{CaseID: id, StateDir: filepath.Join(state, "state"), EffectsFile: filepath.Join(state, "effects.jsonl"), Corpus: filepath.Join(root, "docs/evals/fixtures.json"), ArchiveVector: filepath.Join(root, "internal/mobilebackup/testdata/format1-account-vector.json")}
}

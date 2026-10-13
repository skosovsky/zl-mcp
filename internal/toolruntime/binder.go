package toolruntime

import (
	"encoding/json"
	"strconv"

	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// BindArguments is the baseline compatibility binder. It does not broaden schema
// acceptance, coerce opaque IDs, fill explicit nulls or add unknown defaults.
func BindArguments(raw []byte) (map[string]any, error) {
	args := map[string]any{}
	if len(raw) > 0 && json.Unmarshal(raw, &args) != nil {
		return nil, domain.Invalid("Arguments must be a JSON object.")
	}
	// The legacy typed map treated top-level null as an empty object. Keep
	// that characterization while leaving null field values untouched.
	if args == nil {
		args = map[string]any{}
	}
	for _, field := range []string{"limit", "before", "after"} {
		if v, ok := args[field].(string); ok {
			if n, err := strconv.Atoi(v); err == nil {
				args[field] = float64(n)
			}
		}
	}
	return args, nil
}
func stringArg(args map[string]any, key string) string { v, _ := args[key].(string); return v }
func intArg(args map[string]any, key string, fallback int) int {
	v, ok := args[key].(float64)
	if !ok {
		return fallback
	}
	return int(v)
}
func query(args map[string]any) search.Query {
	return search.Query{
		Text:     stringArg(args, "query"),
		SenderID: stringArg(args, "sender_id"),
		Since:    stringArg(args, "since"),
		Until:    stringArg(args, "until"),
		Cursor:   stringArg(args, "cursor"),
		Limit:    intArg(args, "limit", defaultReadLimit),
	}
}
func groupRequest(args map[string]any) search.GroupRequest {
	return search.GroupRequest{Query: query(args), GroupID: stringArg(args, "group_id")}
}
func conversationRequest(args map[string]any) search.ConversationRequest {
	return search.ConversationRequest{Query: query(args), Conversation: ref(args)}
}
func ref(args map[string]any) domain.ConversationRef {
	return domain.ConversationRef{Type: stringArg(args, "conversation_type"), ID: stringArg(args, "conversation_id")}
}
func contextRequest(args map[string]any, group bool) conversations.ContextRequest {
	identity := ref(args)
	if group {
		identity = domain.ConversationRef{Type: domain.ConversationGroup, ID: stringArg(args, "group_id")}
	}
	return conversations.ContextRequest{
		Conversation: identity,
		MessageID:    stringArg(args, "message_id"),
		Before:       intArg(args, "before", defaultContextNeighbors),
		After:        intArg(args, "after", defaultContextNeighbors),
	}
}

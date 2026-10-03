package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"net/url"
	"strings"
)

func conversationRef(args map[string]any) domain.ConversationRef {
	return domain.ConversationRef{Type: str(args, "conversation_type"), ID: str(args, "conversation_id")}
}
func conversationSearch(args map[string]any) storage.Search {
	return storage.Search{General: true, ConversationType: str(args, "conversation_type"), ConversationID: str(args, "conversation_id"), Query: str(args, "query"), SenderID: str(args, "sender_id"), Since: str(args, "since"), Until: str(args, "until"), Limit: integer(args, "limit", 20), Cursor: str(args, "cursor")}
}
func (s *Service) addConversationResources(server *mcp.Server) error {
	schema, err := contracts.Compile("conversation_collection", "output")
	if err != nil {
		return err
	}
	server.AddResource(&mcp.Resource{URI: "zalo://collection", Name: "Conversation collection status", MIMEType: "application/json"}, func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		allowed, err := s.Store.AllowRead(ctx)
		if err != nil {
			return nil, fmt.Errorf("local read budget unavailable")
		}
		if !allowed {
			return nil, fmt.Errorf("RATE_LIMITED: retry resource read later")
		}
		status, err := s.Store.CollectionStatus(ctx)
		if err != nil {
			return nil, fmt.Errorf("collection status unavailable")
		}
		body, err := json.Marshal(status)
		if err != nil {
			return nil, err
		}
		var value any
		if err = json.Unmarshal(body, &value); err != nil {
			return nil, err
		}
		if err = schema.Validate(value); err != nil {
			return nil, fmt.Errorf("collection status contract mismatch")
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: r.Params.URI, MIMEType: "application/json", Text: string(body)}}}, nil
	})
	server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "zalo://conversations/{conversation_type}/{conversation_id}/messages/{message_id}", Name: "Local conversation message", MIMEType: "text/plain"}, func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		allowed, err := s.Store.AllowRead(ctx)
		if err != nil {
			return nil, fmt.Errorf("local read budget unavailable")
		}
		if !allowed {
			return nil, fmt.Errorf("RATE_LIMITED: retry resource read later")
		}
		u, err := url.Parse(r.Params.URI)
		if err != nil || u.Scheme != "zalo" || u.Host != "conversations" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return nil, fmt.Errorf("invalid conversation message URI")
		}
		parts := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
		if len(parts) != 4 || parts[2] != "messages" {
			return nil, fmt.Errorf("invalid conversation message URI")
		}
		id, err := url.PathUnescape(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid conversation ID")
		}
		mid, err := url.PathUnescape(parts[3])
		if err != nil {
			return nil, fmt.Errorf("invalid message ID")
		}
		m, err := s.Store.ConversationMessage(ctx, domain.ConversationRef{Type: parts[0], ID: id}, mid)
		if err != nil {
			return nil, fmt.Errorf("message unavailable or access denied")
		}
		if len(m.Text) > 1<<20 {
			return nil, fmt.Errorf("message exceeds supported resource size")
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: r.Params.URI, MIMEType: "text/plain", Text: m.Text}}}, nil
	})
	return nil
}

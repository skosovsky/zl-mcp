package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (s *Service) addConversationResources(server *mcp.Server) error {
	archiveSchema, err := contracts.Compile("archive_snapshot_record", "output")
	if err != nil {
		return err
	}
	server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			URITemplate: "zalo://archives/{token}",
			Name:        "Full local archive record",
			MIMEType:    "application/json",
		},
		func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return s.readArchiveRecord(ctx, r, archiveSchema)
		},
	)
	schema, err := contracts.Compile("conversation_collection", "output")
	if err != nil {
		return err
	}
	server.AddResource(
		&mcp.Resource{URI: "zalo://collection", Name: "Conversation collection status", MIMEType: "application/json"},
		func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return s.readCollectionStatus(ctx, r, schema)
		},
	)
	server.AddResourceTemplate(
		&mcp.ResourceTemplate{
			URITemplate: "zalo://conversations/{conversation_type}/{conversation_id}/messages/{message_id}",
			Name:        "Local conversation message",
			MIMEType:    "text/plain",
		},
		func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return s.readConversationMessage(ctx, r)
		},
	)
	return nil
}

type resourceSchema interface {
	Validate(any) error
}

func (s *Service) readArchiveRecord(ctx context.Context, r *mcp.ReadResourceRequest, schema resourceSchema) (*mcp.ReadResourceResult, error) {
	allowed, allowReadErr := s.admission.AllowRead(ctx)
	if allowReadErr != nil || !allowed {
		return nil, errors.New("archive read budget unavailable; retry later")
	}
	u, allowReadErr := url.Parse(r.Params.URI)
	if allowReadErr != nil || u.Scheme != "zalo" || u.Host != "archives" || u.RawQuery != "" || u.Fragment != "" ||
		u.User != nil {
		return nil, errors.New("invalid archive resource URI")
	}
	token := strings.TrimPrefix(u.EscapedPath(), "/")
	if token == "" || len(token) > 5600 || strings.ContainsAny(token, "/%") {
		return nil, errors.New("invalid archive resource token")
	}
	value, allowReadErr := s.archives.Record(ctx, token)
	if allowReadErr != nil {
		return nil, errors.New("archive record unavailable or access denied")
	}
	body, allowReadErr := json.Marshal(value)
	if allowReadErr != nil {
		return nil, errors.New("archive record encoding failed")
	}
	var wire any
	if json.Unmarshal(body, &wire) != nil || schema.Validate(wire) != nil {
		return nil, errors.New("archive record contract mismatch")
	}
	if len(value.Text) > 1<<20 {
		return nil, errors.New("archive record exceeds supported text size")
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{URI: r.Params.URI, MIMEType: "application/json", Text: string(body)},
		},
	}, nil
}

func (s *Service) readCollectionStatus(ctx context.Context, r *mcp.ReadResourceRequest, schema resourceSchema) (*mcp.ReadResourceResult, error) {
	allowed, err := s.admission.AllowRead(ctx)
	if err != nil {
		return nil, errors.New("local read budget unavailable")
	}
	if !allowed {
		return nil, errors.New("RATE_LIMITED: retry resource read later")
	}
	status, err := s.conversations.Catalog.CollectionStatus(ctx)
	if err != nil {
		return nil, errors.New("collection status unavailable")
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
		return nil, errors.New("collection status contract mismatch")
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{URI: r.Params.URI, MIMEType: "application/json", Text: string(body)},
		},
	}, nil
}

func (s *Service) readConversationMessage(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	allowed, err := s.admission.AllowRead(ctx)
	if err != nil {
		return nil, errors.New("local read budget unavailable")
	}
	if !allowed {
		return nil, errors.New("RATE_LIMITED: retry resource read later")
	}
	u, err := url.Parse(r.Params.URI)
	if err != nil || u.Scheme != "zalo" || u.Host != "conversations" || u.RawQuery != "" || u.Fragment != "" ||
		u.User != nil {
		return nil, errors.New("invalid conversation message URI")
	}
	parts := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(parts) != 4 || parts[2] != "messages" {
		return nil, errors.New("invalid conversation message URI")
	}
	id, err := url.PathUnescape(parts[1])
	if err != nil {
		return nil, errors.New("invalid conversation ID")
	}
	mid, err := url.PathUnescape(parts[3])
	if err != nil {
		return nil, errors.New("invalid message ID")
	}
	m, err := s.conversations.Message(
		ctx,
		conversations.MessageRequest{
			Conversation: domain.ConversationRef{Type: parts[0], ID: id},
			MessageID:    mid,
		},
	)
	if err != nil {
		return nil, errors.New("message unavailable or access denied")
	}
	if len(m.Text) > 1<<20 {
		return nil, errors.New("message exceeds supported resource size")
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{URI: r.Params.URI, MIMEType: "text/plain", Text: m.Text}},
	}, nil
}

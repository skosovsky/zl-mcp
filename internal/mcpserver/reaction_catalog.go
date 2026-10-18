package mcpserver

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

const reactionCatalogURI = "zalo://catalog/reactions"

func (s *Service) addReactionCatalog(server *mcp.Server) error {
	schema, err := contracts.Compile("reaction_catalog", "output")
	if err != nil {
		return err
	}
	wire := struct {
		Version     int                     `json:"catalog_version"`
		Description string                  `json:"description"`
		Reactions   []domain.ReactionOption `json:"reactions"`
	}{Version: 2, Description: "Finite upstream-defined reaction codes for explicit selection. Display is a confirmed emoji for heart/like and a descriptive name for other icons; do not guess an obscure icon from a Unicode emoji. Reading the catalog authorizes no write. Arbitrary emoji are not accepted. Use the available write tool's current schema and description for supported actions; catalog membership alone does not establish native replacement or removal semantics.", Reactions: domain.ReactionCatalog()}
	body, err := json.Marshal(wire)
	if err != nil {
		return err
	}
	var raw any
	if json.Unmarshal(body, &raw) != nil || schema.Validate(raw) != nil {
		return errors.New("reaction catalogue violates contract")
	}
	server.AddResource(
		//nolint:exhaustruct_v5 // Optional SDK metadata remains absent for a static JSON catalog.
		&mcp.Resource{
			URI:         reactionCatalogURI,
			Name:        "Explicit reaction choices",
			Description: "Read finite reaction codes and names without changing Zalo state.",
			MIMEType:    contentTypeJSON,
		},
		func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			allowed, err := s.admission.AllowRead(ctx)
			if err != nil {
				return nil, errors.New("local read budget unavailable")
			}
			if !allowed {
				return nil, errors.New("RATE_LIMITED: retry resource read later")
			}
			//nolint:exhaustruct_v5 // Populate only the selected text representation and omit optional SDK metadata.
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{URI: request.Params.URI, MIMEType: contentTypeJSON, Text: string(body)},
				},
			}, nil
		},
	)
	return nil
}

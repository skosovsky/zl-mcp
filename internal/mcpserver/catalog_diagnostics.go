package mcpserver

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func (s *Service) addCatalogDiagnostics(server *mcp.Server) error {
	schema, err := contracts.Compile("catalog_diagnostics", "output")
	if err != nil {
		return err
	}
	server.AddResource(
		//nolint:exhaustruct_v5 // Resource keeps unused optional SDK metadata/representations absent; preserve existing wire defaults.
		&mcp.Resource{
			URI:      "zalo://catalog/diagnostics",
			Name:     "Contact catalogue source diagnostics",
			MIMEType: contentTypeJSON,
		},
		func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			allowed, err := s.admission.AllowRead(ctx)
			if err != nil {
				return nil, errors.New("local read budget unavailable")
			}
			if !allowed {
				return nil, errors.New("RATE_LIMITED: retry resource read later")
			}
			status, err := s.conversations.Catalog.ContactDiagnostics(ctx)
			if err != nil {
				return nil, errors.New("catalogue diagnostics unavailable")
			}
			profiles, err := s.conversations.Catalog.ProfileDiagnostics(ctx)
			if err != nil {
				return nil, errors.New("profile diagnostics unavailable")
			}
			status.Profiles = &profiles
			preload, err := s.conversations.Catalog.PreloadDiagnostics(ctx)
			if err != nil {
				return nil, errors.New("preload diagnostics unavailable")
			}
			status.Preload = &preload
			body, err := json.Marshal(status)
			if err != nil || len(body) > 64<<10 {
				return nil, errors.New("catalogue diagnostics exceed response limit")
			}
			var wire any
			if json.Unmarshal(body, &wire) != nil || schema.Validate(wire) != nil {
				return nil, errors.New("catalogue diagnostics violate contract")
			}
			//nolint:exhaustruct_v5 // ReadResourceResult keeps unused optional SDK metadata/representations absent; preserve existing wire defaults.
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					//nolint:exhaustruct_v5 // Only the selected text/blob resource representation is populated; the other remains absent.
					{URI: r.Params.URI, MIMEType: contentTypeJSON, Text: string(body)},
				},
			}, nil
		},
	)
	return nil
}

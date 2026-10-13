package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func (s *Service) addDeliveryDiagnostics(server *mcp.Server) error {
	schema, err := contracts.Compile("events_diagnostics", "output")
	if err != nil {
		return err
	}
	server.AddResource(
		//nolint:exhaustruct_v5 // Resource keeps unused optional SDK metadata/representations absent; preserve existing wire defaults.
		&mcp.Resource{
			URI:      "zalo://events/diagnostics",
			Name:     "MCP Events delivery diagnostics",
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
			diagnostics, err := s.events.DeliveryDiagnostics(ctx, time.Now().UTC())
			if err != nil {
				return nil, errors.New("delivery diagnostics unavailable")
			}
			body, err := json.Marshal(diagnostics)
			if err != nil || len(body) > 64<<10 {
				return nil, errors.New("delivery diagnostics exceed response limit")
			}
			var wire any
			if json.Unmarshal(body, &wire) != nil || schema.Validate(wire) != nil {
				return nil, errors.New("delivery diagnostics violate contract")
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

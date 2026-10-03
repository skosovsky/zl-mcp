package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func (s *Service) addCatalogDiagnostics(server *mcp.Server) error {
	schema, err := contracts.Compile("catalog_diagnostics", "output")
	if err != nil {
		return err
	}
	server.AddResource(&mcp.Resource{URI: "zalo://catalog/diagnostics", Name: "Contact catalogue source diagnostics", MIMEType: "application/json"}, func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		allowed, err := s.Store.AllowRead(ctx)
		if err != nil {
			return nil, fmt.Errorf("local read budget unavailable")
		}
		if !allowed {
			return nil, fmt.Errorf("RATE_LIMITED: retry resource read later")
		}
		status, err := s.Store.ContactStatus(ctx)
		if err != nil {
			return nil, fmt.Errorf("catalogue diagnostics unavailable")
		}
		body, err := json.Marshal(status)
		if err != nil || len(body) > 64<<10 {
			return nil, fmt.Errorf("catalogue diagnostics exceed response limit")
		}
		var wire any
		if json.Unmarshal(body, &wire) != nil || schema.Validate(wire) != nil {
			return nil, fmt.Errorf("catalogue diagnostics violate contract")
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: r.Params.URI, MIMEType: "application/json", Text: string(body)}}}, nil
	})
	return nil
}

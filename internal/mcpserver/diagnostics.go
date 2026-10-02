package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func (s *Service) addDeliveryDiagnostics(server *mcp.Server) error {
	schema, err := contracts.Compile("events_diagnostics", "output")
	if err != nil {
		return err
	}
	server.AddResource(&mcp.Resource{URI: "zalo://events/diagnostics", Name: "MCP Events delivery diagnostics", MIMEType: "application/json"}, func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		allowed, err := s.Store.AllowRead(ctx)
		if err != nil {
			return nil, fmt.Errorf("local read budget unavailable")
		}
		if !allowed {
			return nil, fmt.Errorf("RATE_LIMITED: retry resource read later")
		}
		diagnostics, err := s.Store.DeliveryDiagnostics(ctx, time.Now().UTC())
		if err != nil {
			return nil, fmt.Errorf("delivery diagnostics unavailable")
		}
		body, err := json.Marshal(diagnostics)
		if err != nil || len(body) > 64<<10 {
			return nil, fmt.Errorf("delivery diagnostics exceed response limit")
		}
		var wire any
		if json.Unmarshal(body, &wire) != nil || schema.Validate(wire) != nil {
			return nil, fmt.Errorf("delivery diagnostics violate contract")
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: r.Params.URI, MIMEType: "application/json", Text: string(body)}}}, nil
	})
	return nil
}

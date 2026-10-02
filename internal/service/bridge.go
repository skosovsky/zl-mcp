package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/local"
)

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	request := r.Clone(r.Context())
	request.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(request)
}

// BridgeStdio forwards the existing tools/resources without opening the corpus
// or owning Zalo. Events use the service's HTTP endpoint directly.
func BridgeStdio(ctx context.Context, c config.Config) error {
	data, err := local.ReadPrivate(c.MCP.TokenFile)
	if err != nil {
		return fmt.Errorf("MCP service token unavailable; start service first: %w", err)
	}
	token, err := validateToken(data)
	if err != nil {
		return err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Timeout: 30 * time.Second, Transport: bearerTransport{token: token, base: transport}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	client := mcp.NewClient(&mcp.Implementation{Name: "zl-mcp-stdio-bridge", Version: "0.1.0-dev"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: "http://" + c.MCP.Listen + "/mcp", HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		return fmt.Errorf("cannot connect to zl-mcp service: %w", err)
	}
	defer session.Close()
	bridge := mcp.NewServer(&mcp.Implementation{Name: "zl-mcp", Version: "0.1.0-dev"}, nil)
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return err
		}
		name := tool.Name
		bridge.AddTool(tool, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: r.Params.Arguments})
		})
	}
	read := func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return session.ReadResource(ctx, &mcp.ReadResourceParams{URI: r.Params.URI})
	}
	for resource, err := range session.Resources(ctx, nil) {
		if err != nil {
			return err
		}
		bridge.AddResource(resource, read)
	}
	for template, err := range session.ResourceTemplates(ctx, nil) {
		if err != nil {
			return err
		}
		bridge.AddResourceTemplate(template, read)
	}
	return bridge.Run(ctx, &mcp.StdioTransport{})
}

func validateToken(data []byte) (string, error) {
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || len(token) > 4096 {
		return "", fmt.Errorf("MCP token length must be 32..4096 ASCII characters")
	}
	for _, r := range token {
		if r < 33 || r > 126 {
			return "", fmt.Errorf("MCP token must contain printable ASCII without spaces")
		}
	}
	return token, nil
}

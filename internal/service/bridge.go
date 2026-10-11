package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
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
	httpClient := &http.Client{Timeout: 40 * time.Second, Transport: bearerTransport{token: token, base: transport}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
	var stdio mcp.Transport = &mcp.StdioTransport{}
	if observer := shared.Telemetry(ctx); observer != nil && observer.Recording() {
		stdio = &observedStdioTransport{reader: os.Stdin, writer: stdioOutput{os.Stdout}, observer: observer}
	}
	return bridge.Run(ctx, stdio)
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

// STDIO observation is borrowed. It creates no SDK provider or execution span.
type stdioOutput struct{ io.Writer }

func (stdioOutput) Close() error { return nil }

type stdioCountingWriter struct {
	io.WriteCloser
	bytes int64
	batch bool
}

func (w *stdioCountingWriter) Write(data []byte) (int, error) {
	n, err := w.WriteCloser.Write(data)
	w.bytes += int64(n)
	w.batch = false
	for _, b := range data[:n] {
		if b != ' ' && b != '\n' && b != '\r' && b != '\t' {
			w.batch = b == '['
			break
		}
	}
	return n, err
}

type observedStdioTransport struct {
	reader   io.ReadCloser
	writer   io.WriteCloser
	observer shared.TelemetryPort
}

func (t *observedStdioTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	counted := &stdioCountingWriter{WriteCloser: t.writer}
	connection, err := (&mcp.IOTransport{Reader: t.reader, Writer: counted}).Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &observedStdioConnection{Connection: connection, writer: counted, observer: t.observer, requests: map[jsonrpc.ID]stdioRequest{}}, nil
}

type stdioRequest struct {
	tool      string
	transport domain.Transport
	expires   time.Time
}
type observedStdioConnection struct {
	mcp.Connection
	writer   *stdioCountingWriter
	observer shared.TelemetryPort
	writeMu  sync.Mutex
	mu       sync.Mutex
	requests map[jsonrpc.ID]stdioRequest
	closed   bool
}

func (c *observedStdioConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	message, err := c.Connection.Read(ctx)
	request, ok := message.(*jsonrpc.Request)
	if err != nil || !ok || !request.ID.IsValid() || (request.Method != "tools/call" && request.Method != "resources/read") {
		return message, err
	}
	if id, ok := request.ID.Raw().(string); ok && len(id) > 2048 {
		return message, err
	}
	var params struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(request.Params, &params)
	if len(params.Name) > 256 {
		params.Name = "other"
	}
	transport := domain.TransportSTDIO
	if request.Method == "resources/read" {
		transport = domain.TransportResource
		params.Name = "other"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return message, err
	}
	now := time.Now()
	for id, entry := range c.requests {
		if !now.Before(entry.expires) {
			delete(c.requests, id)
		}
	}
	if len(c.requests) >= 4096 {
		for id := range c.requests {
			delete(c.requests, id)
			break
		}
	}
	c.requests[request.ID] = stdioRequest{params.Name, transport, now.Add(24 * time.Hour)}
	return message, err
}
func (c *observedStdioConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	before := c.writer.bytes
	err := c.Connection.Write(ctx, message)
	response, ok := message.(*jsonrpc.Response)
	if !ok {
		return err
	}
	c.mu.Lock()
	request, found := c.requests[response.ID]
	delete(c.requests, response.ID)
	c.mu.Unlock()
	accepted := c.writer.bytes - before
	if found && accepted > 0 {
		if c.writer.batch {
			request.tool = "other"
		}
		c.observer.ResponseBytes(ctx, request.tool, request.transport, accepted)
	}
	return err
}

func (c *observedStdioConnection) Close() error {
	c.mu.Lock()
	c.closed = true
	clear(c.requests)
	c.mu.Unlock()
	return c.Connection.Close()
}

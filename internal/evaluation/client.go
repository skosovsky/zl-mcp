package evaluation

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/evalfixture"
)

type client struct {
	session *mcp.ClientSession
	server  *mcp.ServerSession
	trace   []Trace
}

func connect(ctx context.Context, f *evalfixture.Fixture) (*client, error) {
	ct, st := mcp.NewInMemoryTransports()
	ss, err := f.Server.Connect(ctx, st, nil)
	if err != nil {
		return nil, err
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "offline-eval", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		ss.Close()
		return nil, err
	}
	return &client{session: cs, server: ss}, nil
}
func (c *client) close() { c.session.Close(); c.server.Close() }
func (c *client) tool(ctx context.Context, name string, args any) (map[string]any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(110 * time.Millisecond):
	}
	r, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	t := Trace{Kind: "tool", Name: name, Input: marshaled(args)}
	if err != nil {
		t.Error = err.Error()
		c.trace = append(c.trace, t)
		return nil, err
	}
	t.Result = marshaled(r)
	c.trace = append(c.trace, t)
	v, err := decode(t.Result)
	if err != nil {
		return nil, err
	}
	result, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid result object")
	}
	return result, nil
}
func (c *client) resource(ctx context.Context, uri string) {
	r, err := c.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	t := Trace{Kind: "resource", Name: uri}
	if err != nil {
		t.Error = err.Error()
	} else {
		t.Result = marshaled(r)
	}
	c.trace = append(c.trace, t)
}
func payload(r map[string]any) map[string]any {
	v, _ := r["structuredContent"].(map[string]any)
	return v
}
func stringAt(r map[string]any, p string) string { v, _ := path(r, p); s, _ := v.(string); return s }
func (c *client) appendTrace(previous []Trace)   { c.trace = append(previous, c.trace...) }

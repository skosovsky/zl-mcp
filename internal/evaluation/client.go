package evaluation

import (
	"context"
	"errors"
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
	//nolint:exhaustruct_v5 // Optional implementation metadata is absent; preserve the advertised name/version contract.
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "offline-eval", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
		ss.Close()
		return nil, err
	}
	//nolint:exhaustruct_v5 // client fills only this synthetic run/result phase; remaining fields keep their empty/default values.
	return &client{session: cs, server: ss}, nil
}

//nolint:gosec // G104: synthetic fixture/session cleanup preserves the primary evaluation error.
func (c *client) close() { c.session.Close(); c.server.Close() }
func (c *client) tool(ctx context.Context, name string, args any) (map[string]any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(measurementPacingInterval):
	}
	//nolint:exhaustruct_v5 // Unused optional SDK metadata/progress fields remain absent for this invocation.
	r, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	//nolint:exhaustruct_v5 // Trace fills only this synthetic run/result phase; remaining fields keep their empty/default values.
	t := Trace{Kind: fieldTool, Name: name, Input: marshaled(args)}
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
		return nil, errors.New("invalid result object")
	}
	return result, nil
}
func (c *client) resource(ctx context.Context, uri string) {
	//nolint:exhaustruct_v5 // Unused optional SDK metadata remains absent for this resource read.
	r, err := c.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	//nolint:exhaustruct_v5 // Trace fills only this synthetic run/result phase; remaining fields keep their empty/default values.
	t := Trace{Kind: fieldResource, Name: uri}
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

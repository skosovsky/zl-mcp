package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"io"
	"net"
	"net/http"
	"path/filepath"
)

type Client struct{ http *http.Client }

func New(dir string) *Client {
	return &Client{http: &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "collector.sock"))
	}}}}
}
func (c *Client) Call(ctx context.Context, method string, args any) (map[string]any, error) {
	b, e := json.Marshal(map[string]any{"method": method, "arguments": args})
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, "http://collector/rpc", bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.http.Do(req)
	if e != nil {
		return nil, &domain.Error{Code: "COLLECTOR_UNAVAILABLE", Message: "Collector is not reachable.", Retryable: true, NextAction: domain.NextAction{Instruction: "Start zl-mcp service after login."}, Details: map[string]any{}}
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if e != nil {
		return nil, e
	}
	if resp.StatusCode != 200 {
		var de domain.Error
		if json.Unmarshal(data, &de) != nil {
			return nil, fmt.Errorf("invalid collector error response")
		}
		return nil, &de
	}
	var result map[string]any
	e = json.Unmarshal(data, &result)
	return result, e
}

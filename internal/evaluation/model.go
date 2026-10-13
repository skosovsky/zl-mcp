package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/evalfixture"
)

// modelCapture owns observed requests, never trusts a driver's reported tool count.
type modelCapture struct {
	mu       sync.Mutex
	calls    int
	violated bool
	trace    []Trace
	allowed  map[string]bool
}

func (c *modelCapture) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		var t Trace
		switch method {
		case "tools/call":
			params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
			if !ok {
				return nil, errors.New("invalid model call params")
			}
			t = Trace{Kind: "tool", Name: params.Name, Input: params.Arguments}
			c.mu.Lock()
			c.calls++
			denied := c.calls > 30 || !c.allowed[params.Name]
			c.violated = c.violated || denied
			c.mu.Unlock()
			if denied {
				return nil, errors.New("model tool budget or allowlist exceeded")
			}
		case "resources/read":
			params, ok := req.GetParams().(*mcp.ReadResourceParams)
			if !ok {
				return nil, errors.New("invalid model resource params")
			}
			t = Trace{Kind: "resource", Name: params.URI}
		default:
			return next(ctx, method, req)
		}
		result, err := next(ctx, method, req)
		if err != nil {
			t.Error = err.Error()
		} else {
			t.Result = marshaled(result)
		}
		c.mu.Lock()
		c.trace = append(c.trace, t)
		c.mu.Unlock()
		return result, err
	}
}

// RunModelCase is only selected by an explicitly supplied, authorized host driver.
// The outer case watchdog owns this process and all driver descendants.
func RunModelCase(ctx context.Context, opts ChildOptions, cs Case, driver string) (out Output, err error) {
	out = Output{CaseID: cs.ID, Facts: map[string]any{}}
	if !modelScenarioSupported(cs.ID) {
		return out, fmt.Errorf("unsupported model scenario %s: full model gate NOT_RUN", cs.ID)
	}
	if !filepath.IsAbs(driver) {
		return out, errors.New("absolute model driver required")
	}
	f, err := evalfixture.New(
		ctx,
		evalfixture.Options{
			Offline:     true,
			StateDir:    opts.StateDir,
			KeepState:   true,
			EffectsFile: opts.EffectsFile,
			Fixtures:    opts.Corpus,
		},
	)
	if err != nil {
		return out, err
	}
	capture := &modelCapture{allowed: map[string]bool{}}
	for _, name := range cs.AllowedTools {
		capture.allowed[name] = true
	}
	f.Server.AddReceivingMiddleware(capture.middleware)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
		f.Close()
		return out, err
	}
	server := &http.Server{
		Handler:           mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return f.Server }, nil),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer func() {
		closeErr := server.Close()
		serveErr := <-done
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		fixtureErr := f.Close()
		capture.mu.Lock()
		out.Trace = capture.trace
		violated := capture.violated
		capture.mu.Unlock()
		if violated {
			err = errors.Join(err, errors.New("model tool budget or allowlist violation"))
		}
		cleanupErr := errors.Join(closeErr, serveErr, fixtureErr)
		out.Cleanup = cleanupErr == nil
		err = errors.Join(err, cleanupErr)
		out.Complete = err == nil
	}()
	caseFile := filepath.Join(opts.StateDir, "model-case.json")
	if err = os.WriteFile(caseFile, marshaled(cs), 0600); err != nil {
		return out, err
	}
	raw, logs, err := process(
		ctx,
		driver,
		[]string{"--mcp-url", "http://" + listener.Addr().String(), "--case-file", caseFile},
		4<<20,
		false,
	)
	for name, data := range map[string][]byte{"model-driver.stdout.json": raw, "model-driver.stderr.log": logs} {
		if e := os.WriteFile(filepath.Join(filepath.Dir(opts.EffectsFile), name), data, 0600); e != nil {
			err = errors.Join(err, e)
		}
	}
	if err == nil {
		var response any
		err = json.Unmarshal(raw, &response)
		if err == nil {
			out.Facts["driver_result"] = response
		}
	}
	return out, err
}

// Only the protocol smoke scenario has a model setup today. Offline completeness
// and the full model gate are independent; unsupported cases never run.
func modelScenarioSupported(id string) bool { return id == "normal-search" }

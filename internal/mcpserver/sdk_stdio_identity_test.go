package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type sdkFrame struct {
	wire []byte
	err  error
}
type sdkBoundedStderr struct {
	mu   sync.Mutex
	data []byte
}

func (w *sdkBoundedStderr) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remaining := (64 << 10) - len(w.data)
	if remaining > 0 {
		w.data = append(w.data, data[:min(remaining, len(data))]...)
	}
	return len(data), nil
}
func (w *sdkBoundedStderr) String() string { w.mu.Lock(); defer w.mu.Unlock(); return string(w.data) }

type sdkRawChild struct {
	input  io.WriteCloser
	frames <-chan sdkFrame
	stderr *sdkBoundedStderr
	ctx    context.Context
}

//nolint:gocognit // This test helper checks the complete fixture/metric shape without dropping nested boundary assertions.
func sdkStartRawBridge(t *testing.T, endpoint, token string) *sdkRawChild {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "zl-mcp")
	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", bin, "./cmd/zl-mcp")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %v %s", err, output)
	}
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "config.toml")
	state := filepath.Join(dir, "never-open-state")
	contents := fmt.Sprintf(
		"state_dir = %q\n[collection]\ngroup_ids = []\n[mcp]\nlisten = %q\ntoken_file = %q\n",
		state,
		strings.TrimPrefix(endpoint, "http://"),
		tokenPath,
	)
	if err := os.WriteFile(config, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	child := exec.CommandContext(ctx, bin, "-config", config, "serve")
	// Config computes defaults from the actual home; explicit config/token/state paths remain synthetic.
	child.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "TMPDIR=" + t.TempDir(), "TZ=UTC"}
	stderr := &sdkBoundedStderr{}
	child.Stderr = stderr
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	frames := make(chan sdkFrame, 16)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 256<<10)
		for scanner.Scan() {
			frame := sdkFrame{wire: append([]byte(nil), scanner.Bytes()...)}
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		select {
		case frames <- sdkFrame{err: err}:
		case <-ctx.Done():
		}
	}()
	t.Cleanup(func() {
		input.Close()
		done := make(chan error, 1)
		go func() { done <- child.Wait() }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			cancel()
			<-done
		}
		cancel()
		output.Close()
		select {
		case <-readerDone:
		case <-time.After(time.Second):
			t.Error("STDIO reader cleanup failed")
		}
		if _, err := os.Stat(state); !os.IsNotExist(err) {
			t.Error("bridge opened its own native state", err)
		}
	})
	bridge := &sdkRawChild{input: input, frames: frames, stderr: stderr, ctx: ctx}
	bridge.send(
		t,
		[]byte(
			`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"synthetic-sdk-identity","version":"test"}}}`,
		),
	)
	frame := bridge.next(t)
	var response struct {
		ID     json.RawMessage `json:"ID"`
		Result json.RawMessage `json:"Result"`
		Error  json.RawMessage `json:"Error"`
	}
	if err := json.Unmarshal(frame, &response); err != nil || len(response.Error) > 0 || len(response.Result) == 0 {
		t.Fatalf("initialize %s error%v", frame, err)
	}
	assertSDKIdentity(t, response.ID, `"init"`)
	bridge.send(t, []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	return bridge
}
func (c *sdkRawChild) send(t *testing.T, body []byte) {
	t.Helper()
	done := make(chan error, 1)
	go func() { _, err := c.input.Write(append(append([]byte(nil), body...), '\n')); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-c.ctx.Done():
		t.Fatal("STDIO write deadline", c.ctx.Err())
	}
}
func (c *sdkRawChild) next(t *testing.T) []byte {
	t.Helper()
	select {
	case frame := <-c.frames:
		if frame.err != nil {
			t.Fatal("STDIO framing", frame.err, c.stderr.String())
		}
		return frame.wire
	case <-c.ctx.Done():
		t.Fatal("STDIO response deadline", c.ctx.Err())
		return nil
	}
}
func sdkReadFrameResult(t *testing.T, frame []byte) sdkHTTPResult {
	t.Helper()
	var envelope struct {
		ID json.RawMessage `json:"ID"`
	}
	if err := json.Unmarshal(frame, &envelope); err != nil {
		t.Fatal(err)
	}
	return sdkHTTPResult{id: envelope.ID, body: frame}
}
func TestSDK01ActualSTDIOIdentityBoundsAndOutOfOrder(t *testing.T) {
	// Arrange: real bridge subprocess borrows the synthetic HTTP owner.
	handler, token, reader := sdkBlockingHTTP(t)
	endpoint := httptest.NewServer(handler)
	t.Cleanup(endpoint.Close)
	child := sdkStartRawBridge(t, endpoint.URL, token)
	// Act/Assert: boundary and notation IDs traverse both transports without float64.
	for _, tc := range sdkIdentityCases {
		child.send(
			t,
			sdkRawBody(
				"tools/call",
				tc.input,
				map[string]any{"name": "zalo_get_status", "arguments": map[string]any{}},
			),
		)
		reply := sdkReadFrameResult(t, child.next(t))
		assertSDKIdentity(t, reply.id, tc.want)
		var envelope struct {
			Result json.RawMessage `json:"Result"`
			Error  json.RawMessage `json:"Error"`
		}
		if err := json.Unmarshal(
			reply.body,
			&envelope,
		); err != nil || len(envelope.Error) > 0 ||
			len(envelope.Result) == 0 {
			t.Fatalf("roundtrip %s error%v", reply.body, err)
		}
	}
	before := reader.calls.Load()
	for _, id := range []string{"1.5", "9223372036854775808", "-9223372036854775809", "1e999999999", "1e-999999999", "0." + strings.Repeat("0", 127), "0." + strings.Repeat("0", 100000)} {
		child.send(t, sdkRawBody("tools/call", id, sdkContextArgs("first")))
		frame := child.next(t)
		var envelope struct {
			ID    json.RawMessage `json:"ID"`
			Error *protocolError  `json:"Error"`
		}
		if err := json.Unmarshal(
			frame,
			&envelope,
		); err != nil || string(envelope.ID) != "null" || envelope.Error == nil ||
			envelope.Error.Code != -32600 {
			t.Fatalf("invalid length%d frame%s error%v", len(id), frame, err)
		}
	}
	if reader.calls.Load() != before {
		t.Fatal("invalid STDIO ID reached application")
	}
	// Act: two neighboring IDs which collided in the original SDK, second completes first.
	child.send(t, sdkRawBody("tools/call", "9007199254740992", sdkContextArgs("first")))
	child.send(t, sdkRawBody("tools/call", "9007199254740993", sdkContextArgs("second")))
	a, b := sdkNextInvocation(t, reader), sdkNextInvocation(t, reader)
	if a.message == b.message {
		t.Fatal("STDIO calls collapsed")
	}
	close(reader.release["second"])
	// Assert.
	sdkAssertContextReply(t, sdkReadFrameResult(t, child.next(t)), "9007199254740993", "second")
	close(reader.release["first"])
	sdkAssertContextReply(t, sdkReadFrameResult(t, child.next(t)), "9007199254740992", "first")
	child.send(t, sdkRawBody("resources/read", "9223372036854775807", map[string]any{"uri": "zalo://capabilities"}))
	assertSDKIdentity(t, sdkReadFrameResult(t, child.next(t)).id, "9223372036854775807")
	child.send(t, sdkRawBody("unknown/synthetic", "-9223372036854775808", nil))
	reply := sdkReadFrameResult(t, child.next(t))
	assertSDKIdentity(t, reply.id, "-9223372036854775808")
	var errorReply struct {
		Error *protocolError `json:"Error"`
	}
	if err := json.Unmarshal(
		reply.body,
		&errorReply,
	); err != nil || errorReply.Error == nil ||
		errorReply.Error.Code != -32601 {
		t.Fatalf("protocol error %s", reply.body)
	}
}
func TestSDK01ActualSTDIOCancellationAndIDReusePrivacy(t *testing.T) {
	// Arrange: enable global DEBUG to prove SDK defaults and root logs do not expose IDs/reasons.
	var logs sdkBoundedStderr
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)
	handler, token, reader := sdkBlockingHTTP(t)
	endpoint := httptest.NewServer(handler)
	t.Cleanup(endpoint.Close)
	child := sdkStartRawBridge(t, endpoint.URL, token)
	child.send(t, sdkRawBody("tools/call", "9007199254740993", sdkContextArgs("first")))
	child.send(t, sdkRawBody("tools/call", "9007199254740992", sdkContextArgs("second")))
	invocations := map[string]sdkInvocation{}
	for range 2 {
		v := sdkNextInvocation(t, reader)
		invocations[v.message] = v
	}
	// Act: numeric requestId dynamic type must remain exact across SDK notification decoding.
	child.send(
		t,
		[]byte(
			`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":9007199254740993,"reason":"SDK_PRIVATE_CANCEL_REASON_界🍀"}}`,
		),
	)
	// Assert: only selected invocation is cancelled, response correlation remains exact.
	select {
	case <-invocations["first"].ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("STDIO cancellation did not reach application")
	}
	if invocations["second"].ctx.Err() != nil {
		t.Fatal("neighbor cancelled")
	}
	cancelled := sdkReadFrameResult(t, child.next(t))
	assertSDKIdentity(t, cancelled.id, "9007199254740993")
	child.send(t, sdkRawBody("tools/call", "9007199254740993", sdkContextArgs("reused")))
	v := sdkNextInvocation(t, reader)
	if v.message != "reused" || v.ctx.Err() != nil {
		t.Fatal("old cancellation damaged reused ID")
	}
	close(reader.release["reused"])
	sdkAssertContextReply(t, sdkReadFrameResult(t, child.next(t)), "9007199254740993", "reused")
	close(reader.release["second"])
	sdkAssertContextReply(t, sdkReadFrameResult(t, child.next(t)), "9007199254740992", "second")
	if strings.Contains(logs.String(), "SDK_PRIVATE_CANCEL_REASON") ||
		strings.Contains(child.stderr.String(), "SDK_PRIVATE_CANCEL_REASON") {
		t.Fatal("SDK cancellation text leaked")
	}
}

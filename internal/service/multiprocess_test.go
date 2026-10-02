package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"net"
)

func TestTwoMCPProcessesReadCollectorWrites(t *testing.T) {
	// Arrange: two real STDIO bridge processes and one unified HTTP service; only Zalo delivery is synthetic.
	dir, err := os.MkdirTemp("/tmp", "zl-two-bridges-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	var c config.Config
	c.StateDir = dir
	c.Collection.GroupIDs = []string{"g"}
	c.Storage.RetentionDays = 90
	c.Permissions.AllowJoin = true
	c.MCP.Listen = "127.0.0.1:0"
	c.MCP.TokenFile = filepath.Join(dir, "token")
	c.Logging.File = filepath.Join(dir, "logs", "service.log")
	c.Logging.MaxSizeMB = 5
	c.Logging.MaxBackups = 3
	token := strings.Repeat("t", 64)
	if err := os.WriteFile(c.MCP.TokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "zl-mcp")
	build := exec.Command("go", "build", "-o", bin, "./cmd/zl-mcp")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", output, err)
	}
	configPath := filepath.Join(c.StateDir, "config.toml")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	serviceCtx, stopService := context.WithCancel(ctx)
	client := &replayListener{messages: make(chan messageCommand)}
	bound := make(chan net.Addr, 1)
	finished := make(chan error, 1)
	go func() {
		finished <- run(serviceCtx, c, func(context.Context, string) (collector.ListenerUpstream, error) { return client, nil }, func(collector.ListenerUpstream) error { return nil }, func(addr net.Addr) { bound <- addr })
	}()
	t.Cleanup(func() {
		stopService()
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("service shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("service shutdown hung")
		}
	})
	var addr net.Addr
	select {
	case addr = <-bound:
	case <-ctx.Done():
		t.Fatal("service not ready")
	}
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf("state_dir = %q\n[collection]\ngroup_ids = [\"g\"]\n[mcp]\nlisten = %q\ntoken_file = %q\n", c.StateDir, addr.String(), c.MCP.TokenFile)), 0600); err != nil {
		t.Fatal(err)
	}
	for {
		status := serviceRPC(t, "http://"+addr.String()+"/mcp", token, "tools/call", map[string]any{"name": "zalo_get_status", "arguments": map[string]any{}})["structuredContent"].(map[string]any)
		if status["collector_state"] == "connected" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("collector not connected")
		case <-time.After(150 * time.Millisecond):
		}
	}

	sessions := make([]*mcp.ClientSession, 2)
	logs := make([]bytes.Buffer, 2)
	for i := range sessions {
		child := exec.Command(bin, "-config", configPath, "serve")
		child.Stderr = &logs[i]
		sdkClient := mcp.NewClient(&mcp.Implementation{Name: fmt.Sprintf("parallel-reader-%d", i), Version: "1"}, nil)
		session, err := sdkClient.Connect(ctx, &mcp.CommandTransport{Command: child}, nil)
		if err != nil {
			t.Fatal(err)
		}
		sessions[i] = session
		t.Cleanup(func() { session.Close() })
	}
	call := func(session *mcp.ClientSession, name string, args map[string]any) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("%s result=%+v error=%v", name, result, err)
		}
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		if err = json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		return wire
	}
	// Act: both already-running readers observe a subsequent collector write.
	for _, session := range sessions {
		status := call(session, "zalo_get_status", map[string]any{})
		if status["stored_message_count"] != float64(0) || status["collector_state"] != "connected" {
			t.Fatalf("initial status=%+v", status)
		}
	}
	ack := make(chan error, 1)
	client.messages <- messageCommand{message: domain.Message{GroupID: "g", ID: "live-message", SenderID: "sender", SentAt: time.Now().UTC(), Text: "Ремонт кондиционера подтверждён", Source: "live"}, done: ack}
	select {
	case err := <-ack:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("message was not collected")
	}

	// Assert: each independent MCP process reads the same committed data and uses the internal membership port.
	for _, session := range sessions {
		search := call(session, "zalo_search_messages", map[string]any{"query": "РЕМОНТ", "group_id": "g"})
		messages := search["messages"].([]any)
		if len(messages) != 1 || messages[0].(map[string]any)["message_id"] != "live-message" {
			t.Fatalf("search=%+v", search)
		}
		contextResult := call(session, "zalo_get_message_context", map[string]any{"group_id": "g", "message_id": "live-message"})
		if contextResult["anchor"].(map[string]any)["text"] != "Ремонт кондиционера подтверждён" {
			t.Fatalf("context=%+v", contextResult)
		}
		// Unapproved secret-looking input must produce a safe error without logging its value.
		denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_join_group", Arguments: map[string]any{"plan_token": "SYNTHETIC_TOKEN_MUST_NOT_BE_LOGGED", "request_id": "d9a901ed-b070-4712-a72f-4a5942b3312c"}})
		if err != nil || !denied.IsError {
			t.Fatalf("unapproved join: %+v %v", denied, err)
		}
		group := call(session, "zalo_get_group", map[string]any{"group_id": "g"})
		if group["group"].(map[string]any)["name"] != "Test group" {
			t.Fatalf("control group=%+v", group)
		}
	}
	// Close waits for the subprocess streams before inspecting captured logs.
	for i, session := range sessions {
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
		logText := logs[i].String()
		for _, forbidden := range []string{"SYNTHETIC_TOKEN_MUST_NOT_BE_LOGGED", "Ремонт кондиционера подтверждён", "live-message"} {
			if strings.Contains(logText, forbidden) {
				t.Fatalf("reader %d leaked synthetic private value", i)
			}
		}
		if strings.TrimSpace(logText) == "" {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(logText), "\n") {
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("reader %d malformed audit log: %v", i, err)
			}
			if entry["msg"] != "mcp_call" || entry["tool"] == nil || entry["correlation_id"] == nil {
				t.Fatalf("reader %d missing audit fields: %+v", i, entry)
			}
		}
	}
	logData, err := os.ReadFile(c.Logging.File)
	if err != nil {
		t.Fatal(err)
	}
	auditCalls := 0
	for _, line := range bytes.Split(bytes.TrimSpace(logData), []byte("\n")) {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal(err)
		}
		if entry["msg"] == "mcp_call" {
			auditCalls++
			if entry["tool"] == nil || entry["correlation_id"] == nil {
				t.Fatal("service audit lacks correlation")
			}
		}
	}
	if auditCalls < 8 || strings.Contains(string(logData), "SYNTHETIC_TOKEN_MUST_NOT_BE_LOGGED") || strings.Contains(string(logData), "Ремонт кондиционера подтверждён") {
		t.Fatal("missing or unsafe service audit")
	}
	if client.calls.Load() != 1 {
		t.Fatal("two bridges created multiple Zalo listeners")
	}
}

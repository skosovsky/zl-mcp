package collector

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
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestTwoMCPProcessesReadCollectorWrites(t *testing.T) {
	// Arrange: real CLI processes and SQLite/control socket; only Zalo delivery is synthetic.
	c, s := sessionStore(t)
	bin := filepath.Join(t.TempDir(), "zl-mcp")
	build := exec.Command("go", "build", "-o", bin, "./cmd/zl-mcp")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", output, err)
	}
	configPath := filepath.Join(c.StateDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf("state_dir = %q\n[collection]\ngroup_ids = [\"g\"]\n", c.StateDir)), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	collectorCtx, stopCollector := context.WithCancel(ctx)
	ready, deliver, collected := make(chan struct{}), make(chan struct{}), make(chan struct{})
	client := &scriptedListener{fakeZalo: &fakeZalo{name: "Test group", joined: true}}
	client.listen = func(ctx context.Context, onMessage func(domain.Message) error, _ func(string, string) error, onConnected func() error) error {
		if err := onConnected(); err != nil {
			return err
		}
		close(ready)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deliver:
		}
		if err := onMessage(domain.Message{GroupID: "g", ID: "live-message", SenderID: "sender", SentAt: time.Now().UTC(), Text: "Ремонт кондиционера подтверждён", Source: "live"}); err != nil {
			return err
		}
		close(collected)
		<-ctx.Done()
		return ctx.Err()
	}
	finished := make(chan error, 1)
	go func() { finished <- runSession(collectorCtx, c, s, client) }()
	t.Cleanup(func() {
		stopCollector()
		if err := <-finished; err != nil {
			t.Errorf("collector shutdown: %v", err)
		}
	})
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("collector did not connect")
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
	close(deliver)
	select {
	case <-collected:
	case <-ctx.Done():
		t.Fatal("message was not collected")
	}
	// Assert: each independent MCP process reads the same committed data and uses control API.
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

}

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type authListener struct{ calls atomic.Int32 }

func (*authListener) AccountID() string                              { return "test-account" }
func (*authListener) Groups(context.Context) ([]domain.Group, error) { return []domain.Group{}, nil }
func (*authListener) Group(context.Context, string) (domain.Group, *string, error) {
	panic("unexpected upstream call")
}
func (*authListener) Inspect(context.Context, string) (domain.Invite, error) {
	panic("unexpected upstream call")
}
func (*authListener) Join(context.Context, string) error { panic("unexpected upstream call") }

func (a *authListener) Listen(
	ctx context.Context,
	_ func(domain.Message) error,
	_ func(string, string) error,
	connected func() error,
) error {
	a.calls.Add(1)
	if err := connected(); err != nil {
		return err
	}
	return domain.ErrAuthenticationRequired
}

//nolint:cyclop,gocognit,gocyclo // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestServicePreservesMCPWhenAuthenticationIsRequired(t *testing.T) {
	for _, stage := range []string{"restore", "listener"} {
		t.Run(stage, func(t *testing.T) {
			// Arrange: real HTTP, SQLite and service lifecycle; only Zalo is synthetic.
			dir, err := os.MkdirTemp("/tmp", "zl-service-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			var c config.Config
			c.StateDir = dir
			c.Collection.GroupIDs = []string{"g"}
			c.Storage.RetentionDays = 90
			c.MCP.Listen = "127.0.0.1:0"
			c.MCP.TokenFile = filepath.Join(dir, "token")
			c.Logging.File = filepath.Join(dir, "logs", "service.log")
			c.Logging.EventTraceFile = filepath.Join(dir, "logs", "events-trace.jsonl")
			c.Logging.MaxSizeMB = 5
			c.Logging.MaxBackups = 3
			token := strings.Repeat("a", 64)
			if writeFileErr := os.WriteFile(c.MCP.TokenFile, []byte(token), 0600); writeFileErr != nil {
				t.Fatal(writeFileErr)
			}
			s, err := storage.Open(context.Background(), filepath.Join(dir, "messages.sqlite"), []string{"g"}, 90)
			if err != nil {
				t.Fatal(err)
			}
			err = s.Put(
				context.Background(),
				domain.Message{
					GroupID:  "g",
					ID:       "saved",
					SenderID: "author",
					SentAt:   time.Now().UTC(),
					Text:     "service searchable corpus",
					Source:   "live",
				},
			)
			s.Close()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			bound := make(chan net.Addr, 1)
			var restores atomic.Int32
			client := &authListener{}
			go func() {
				done <- run(ctx, c, func(context.Context, string) (collector.ListenerUpstream, error) {
					restores.Add(1)
					if stage == "restore" {
						return nil, domain.ErrAuthenticationRequired
					}
					return client, nil
				}, func(collector.ListenerUpstream) error { return nil }, func(addr net.Addr) { bound <- addr })
			}()
			var address net.Addr
			select {
			case address = <-bound:
			case operationErr := <-done:
				t.Fatalf("startup failed: %v", operationErr)
			case <-time.After(5 * time.Second):
				t.Fatal("startup timed out")
			}
			rpc := func(name string, args map[string]any) map[string]any {
				t.Helper()
				body, _ := json.Marshal(
					map[string]any{
						"jsonrpc": "2.0",
						"id":      1,
						"method":  "tools/call",
						"params": map[string]any{
							"name":      name,
							"arguments": args,
							"_meta": map[string]any{
								"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
								"io.modelcontextprotocol/clientCapabilities": map[string]any{},
							},
						},
					},
				)
				req, _ := http.NewRequest(http.MethodPost, "http://"+address.String()+"/mcp", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "application/json, text/event-stream")
				req.Header.Set("Mcp-Method", "tools/call")
				req.Header.Set("Mcp-Name", name)
				req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
				hc := &http.Client{Timeout: 3 * time.Second}
				response, doErr := hc.Do(req)
				if doErr != nil {
					t.Fatal(doErr)
				}
				defer response.Body.Close()
				var reply map[string]any
				if decodeErr := json.NewDecoder(response.Body).Decode(&reply); decodeErr != nil {
					t.Fatal(decodeErr)
				}
				if reply["error"] != nil {
					t.Fatalf("RPC failure: %v", reply)
				}
				return reply["result"].(map[string]any)
			}
			// Act: await the explicit auth-required state through the actual MCP endpoint.
			deadline := time.Now().Add(3 * time.Second)
			for {
				reply := rpc("zalo_get_status", map[string]any{})
				status, ok := reply["structuredContent"].(map[string]any)
				if !ok {
					// Startup may bind the account between host binding and scoped execution.
					// A stale scope must be denied without reading the new account; retry only
					// that explicit denial, within the original readiness deadline.
					encoded, marshalErr := json.Marshal(reply)
					if marshalErr != nil {
						t.Fatal(marshalErr)
					}
					var rejected struct {
						IsError bool `json:"isError"`
						Content []struct {
							Text string `json:"text"`
						} `json:"content"`
					}
					if marshalErr = json.Unmarshal(encoded, &rejected); marshalErr != nil {
						t.Fatal(marshalErr)
					}
					var failure domain.Error
					if rejected.IsError && len(rejected.Content) == 1 &&
						json.Unmarshal([]byte(rejected.Content[0].Text), &failure) == nil &&
						failure.Code == "PERMISSION_DENIED" &&
						time.Now().Before(deadline) {
						time.Sleep(150 * time.Millisecond)
						continue
					}
					t.Fatalf("collector status did not return structured content: %v", reply)
				}
				if status["collector_state"] == "auth_required" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("state did not settle: %v", status)
				}
				time.Sleep(150 * time.Millisecond)
			}
			search := rpc("zalo_search_messages", map[string]any{"query": "searchable"})
			contextResult := rpc("zalo_get_message_context", map[string]any{"group_id": "g", "message_id": "saved"})
			inspect := rpc("zalo_inspect_invite", map[string]any{"invite_url": "https://zalo.me/g/example"})
			_, lockErr := local.Lock(dir)
			time.Sleep(350 * time.Millisecond)
			// Assert: MCP stays alive, corpus is accessible, account ownership is exclusive,
			// and rejected network operations cannot trigger another Zalo restore/listener.
			var rejected domain.Error
			contents := inspect["content"].([]any)
			if unmarshalErr := json.Unmarshal([]byte(contents[0].(map[string]any)["text"].(string)), &rejected); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			hits := search["structuredContent"].(map[string]any)["messages"].([]any)
			anchor := contextResult["structuredContent"].(map[string]any)["anchor"].(map[string]any)
			if len(hits) != 1 || anchor["text"] != "service searchable corpus" ||
				rejected.Code != "NOT_AUTHENTICATED" ||
				inspect["isError"] != true ||
				lockErr == nil ||
				restores.Load() != 1 {
				t.Fatalf("search=%v inspect=%v lock=%v restores=%d", search, inspect, lockErr, restores.Load())
			}
			if stage == "listener" && client.calls.Load() != 1 {
				t.Fatal("listener was restarted after auth loss")
			}
			cancel()
			select {
			case shutdownErr := <-done:
				if shutdownErr != nil {
					t.Fatal(shutdownErr)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown hung")
			}
			info, err := os.Stat(c.Logging.EventTraceFile)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("full trace is missing or not private")
			}
			logData, err := os.ReadFile(c.Logging.File)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(logData), "Zalo authentication required") != 1 ||
				!strings.Contains(string(logData), "service started") ||
				!strings.Contains(string(logData), "service stopped") ||
				strings.Contains(string(logData), token) ||
				strings.Contains(string(logData), "searchable corpus") {
				t.Fatal("service log lacks lifecycle diagnostics or leaks private data")
			}
			for line := range bytes.SplitSeq(bytes.TrimSpace(logData), []byte("\n")) {
				if !json.Valid(line) {
					t.Fatalf("invalid JSON log: %s", line)
				}
			}
			unlock, err := local.Lock(dir)
			if err != nil {
				t.Fatal("service retained account lock", err)
			}
			unlock()
			if _, err = os.Stat(filepath.Join(dir, "collector.sock")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("CLI socket survived shutdown", err)
			}
		})
	}
}

func TestStartupStorageFailurePersistsDiagnosticAndDoesNotRequestLogin(t *testing.T) {
	// Arrange: saved-session I/O fails while SQLite remains available.
	dir := t.TempDir()
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(dir, "messages.sqlite"), nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c := config.Config{StateDir: dir}
	failure := &domain.Error{Code: "STORAGE_ERROR", Message: "synthetic private-file failure"}
	calls := 0
	// Act: exercise the unified restore lifecycle rather than the removed legacy collector.
	err = collect(
		ctx,
		c,
		store,
		&membershipPort{store: store},
		func(context.Context, string) (collector.ListenerUpstream, error) { calls++; return nil, failure },
		func(collector.ListenerUpstream) error { return nil },
	)
	state, readErr := store.State(ctx)
	// Assert: fatal storage errors are distinguishable from auth/network problems.
	if !errors.Is(err, failure) || readErr != nil || calls != 1 || state["collector_state"] != "stopped" ||
		state["last_error"].(map[string]any)["code"] != "STORAGE_ERROR" {
		t.Fatalf("err=%v read=%v state=%v calls=%d", err, readErr, state, calls)
	}
}

func TestHeartbeatPreservesReconnectingDuringSlowRestoreAndCrashGap(t *testing.T) {
	// Arrange: stale state from the previous process, and a blocked Restore.
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := storage.Open(parent, filepath.Join(t.TempDir(), "messages.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	previous := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	if setStateErr := store.SetState(
		parent,
		map[string]any{"authenticated": true, "collector_state": "connected"},
	); setStateErr != nil {
		t.Fatal(setStateErr)
	}
	if _, execErr := store.DB.Exec(
		"UPDATE collector_state SET heartbeat=?",
		previous.Format(time.RFC3339Nano),
	); execErr != nil {
		t.Fatal(execErr)
	}
	entered := make(chan struct{})
	done := make(chan error, 1)
	c := config.Config{StateDir: t.TempDir()}
	go func() {
		done <- collectWithHeartbeat(parent, c, store, &membershipPort{store: store}, func(ctx context.Context, _ string) (collector.ListenerUpstream, error) {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		}, func(collector.ListenerUpstream) error { return nil }, 10*time.Millisecond)
	}()
	select {
	case <-entered:
	case startupErr := <-done:
		t.Fatalf("startup: %v", startupErr)
	case <-time.After(time.Second):
		t.Fatal("Restore not entered")
	}
	// Act: simulate exceeding the stale threshold while Restore is still in flight.
	if _, nestedExecErr := store.DB.Exec(
		"UPDATE collector_state SET heartbeat=?",
		previous.Format(time.RFC3339Nano),
	); nestedExecErr != nil {
		t.Fatal(nestedExecErr)
	}
	deadline := time.Now().Add(time.Second)
	var observed map[string]any
	for {
		observed, err = store.State(parent)
		if err != nil {
			t.Fatal(err)
		}
		if observed["collector_state"] == "reconnecting" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("heartbeat did not refresh during Restore")
		}
		time.Sleep(time.Millisecond)
	}
	coverage, err := store.Coverage(parent, "g")
	cancel()
	select {
	case finalShutdownErr := <-done:
		if finalShutdownErr != nil {
			t.Fatal(finalShutdownErr)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not stop")
	}
	// Assert: liveness preserves the state and the pre-crash gap, not connected.
	if err != nil || observed["authenticated"] != false || len(coverage.KnownGaps) != 1 ||
		!coverage.KnownGaps[0].From.Equal(previous) {
		t.Fatalf("state=%v coverage=%v error=%v", observed, coverage, err)
	}
}

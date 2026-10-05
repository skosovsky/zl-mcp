package service

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type historyServiceSource struct {
	authListener
	started chan struct{}
	pages   atomic.Int32
}

func (s *historyServiceSource) Listen(ctx context.Context, _ func(domain.Message) error, _ func(string, string) error, connected func() error) error {
	if err := connected(); err != nil {
		return err
	}
	close(s.started)
	<-ctx.Done()
	return ctx.Err()
}

func (s *historyServiceSource) HistoryPage(ctx context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
	s.pages.Add(1)
	more := false
	return domain.HistoryPage{Messages: []domain.Message{{Conversation: ref, ID: "historical", SenderID: "synthetic-author", SentAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Text: "Synthetic service history"}}, HasMore: &more}, nil
}

func (s *historyServiceSource) PreloadHistoryPage(ctx context.Context, ref domain.ConversationRef, limit int) (domain.HistoryPage, error) {
	s.pages.Add(1)
	return domain.HistoryPage{Messages: []domain.Message{{Conversation: ref, ID: "historical", SenderID: "synthetic-author", SentAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Text: "Synthetic preload service history"}}, LimitedSnapshot: true}, nil
}

func TestHTTPHistoryToolsUseExistingCollectorSession(t *testing.T) {
	for _, tc := range []struct{ name, kind, source, state string }{
		{"group paging", "group", "", "completed"},
		{"direct snapshot", "direct", "conversation_preload", "partial"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: production service lifecycle/HTTP, one restore and one listener.
			dir, err := os.MkdirTemp("/tmp", "zl-history-service-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			var c config.Config
			c.StateDir, c.Collection.Mode, c.Storage.RetentionDays = dir, "all", 90
			c.MCP.Listen, c.MCP.TokenFile = "127.0.0.1:0", filepath.Join(dir, "token")
			token := strings.Repeat("a", 64)
			if err := os.WriteFile(c.MCP.TokenFile, []byte(token), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var restores atomic.Int32
			source := &historyServiceSource{started: make(chan struct{})}
			bound, done := make(chan net.Addr, 1), make(chan error, 1)
			go func() {
				done <- run(ctx, c, func(context.Context, string) (collector.ListenerUpstream, error) {
					restores.Add(1)
					return source, nil
				}, func(collector.ListenerUpstream) error { return nil }, func(addr net.Addr) { bound <- addr })
			}()
			defer func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(5 * time.Second):
					t.Error("history service did not stop")
				}
			}()
			var addr net.Addr
			select {
			case addr = <-bound:
			case <-time.After(5 * time.Second):
				t.Fatal("HTTP service not ready")
			}
			select {
			case <-source.started:
			case <-time.After(5 * time.Second):
				t.Fatal("collector session not ready")
			}
			endpoint := "http://" + addr.String() + "/mcp"
			args := map[string]any{"conversation_type": tc.kind, "conversation_id": "synthetic-group", "request_id": "00000000-0000-4000-8000-000000000001", "since": "2026-09-01T00:00:00Z", "until": "2026-11-01T00:00:00Z"}
			if tc.source != "" {
				args["source"] = tc.source
			}
			call := func(name string, arguments map[string]any) map[string]any {
				return serviceConversationRPC(t, endpoint, token, "tools/call", map[string]any{"name": name, "arguments": arguments})["structuredContent"].(map[string]any)
			}
			// Act
			accepted := call("zalo_import_conversation_history", args)
			id := accepted["operation_id"].(string)
			deadline := time.Now().Add(5 * time.Second)
			var status map[string]any
			for {
				status = call("zalo_get_history_import_status", map[string]any{"operation_id": id})
				if status["state"] == tc.state {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("service import did not complete")
				}
				time.Sleep(100 * time.Millisecond)
			}
			if tc.source != "" && (status["source_kind"] != tc.source || status["stop_reason"] != "source_window_limited" || status["source_has_more"] != nil) {
				t.Fatal("snapshot source evidence lost")
			}
			read := call("zalo_get_conversation_message_context", map[string]any{"conversation_type": tc.kind, "conversation_id": "synthetic-group", "message_id": "historical"})
			again := call("zalo_import_conversation_history", args)
			// Assert
			if restores.Load() != 1 || source.pages.Load() != 1 || status["notification_policy"] != "none" || status["inserted_count"] != float64(1) || status["history_complete"] != false || again["operation_id"] != id || read["anchor"] == nil {
				t.Fatalf("shared service history mismatch: restores=%d pages=%d status=%#v", restores.Load(), source.pages.Load(), status)
			}
			diagnostics := serviceConversationRPC(t, endpoint, token, "resources/read", map[string]any{"uri": "zalo://events/diagnostics"})
			if diagnostics["contents"] == nil {
				t.Fatal("event diagnostics unavailable after silent import")
			}
			contents := diagnostics["contents"].([]any)
			var snapshot struct {
				JournalPending      int            `json:"journal_pending"`
				PendingPayloadBytes int            `json:"pending_payload_bytes"`
				StateCounts         map[string]int `json:"state_counts"`
			}
			if err := json.Unmarshal([]byte(contents[0].(map[string]any)["text"].(string)), &snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.JournalPending != 0 || snapshot.PendingPayloadBytes != 0 {
				t.Fatalf("historical import created event work: %#v", snapshot)
			}
			for state, count := range snapshot.StateCounts {
				if count != 0 {
					t.Fatalf("historical import created %s deliveries: %d", state, count)
				}
			}

		})
	}
}

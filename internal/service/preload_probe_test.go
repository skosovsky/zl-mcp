package service

import (
	"context"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/control"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type probeServiceSource struct {
	*historyServiceSource
	calls atomic.Int32
}

func (s *probeServiceSource) ConversationPreload(context.Context) (domain.PreloadSnapshot, error) {
	s.calls.Add(1)
	return domain.PreloadSnapshot{Entries: []domain.PreloadEntry{{Conversation: domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}}}, Messages: []domain.Message{{Conversation: domain.ConversationRef{Type: "direct", ID: "synthetic-peer"}, Text: "Synthetic available source text"}}, DirectMessagesAvailable: true}, nil
}

func TestPrivatePreloadProbeUsesServiceSessionWithoutPersistingCorpus(t *testing.T) {
	// Arrange: one production service session and its owner-only Unix route.
	dir, err := os.MkdirTemp("/tmp", "zl-preload-probe-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	var c config.Config
	c.StateDir, c.Collection.Mode, c.Storage.RetentionDays = dir, "all", 90
	c.MCP.Listen, c.MCP.TokenFile = "127.0.0.1:0", filepath.Join(dir, "token")
	if err := os.WriteFile(c.MCP.TokenFile, []byte(strings.Repeat("a", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var restores atomic.Int32
	source := &probeServiceSource{historyServiceSource: &historyServiceSource{started: make(chan struct{})}}
	ready, done := make(chan net.Addr, 1), make(chan error, 1)
	go func() {
		done <- run(ctx, c, func(context.Context, string) (collector.ListenerUpstream, error) { restores.Add(1); return source, nil }, func(collector.ListenerUpstream) error { return nil }, func(addr net.Addr) { ready <- addr })
	}()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("probe service did not stop")
		}
	}()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("probe service not ready")
	}
	select {
	case <-source.started:
	case <-time.After(5 * time.Second):
		t.Fatal("probe listener not ready")
	}
	// Wait for the separate startup metadata refresh before probing.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "messages.sqlite")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM conversations").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("startup catalogue refresh not completed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Act
	request, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	result, err := control.New(dir).Call(request, "cli_probe_preload", map[string]any{})
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if restores.Load() != 1 || source.calls.Load() != 2 || result["source_status"] != "available" || result["direct_message_count"] != float64(1) || result["metadata_persisted"] != false || result["messages_persisted"] != false {
		t.Fatal("private probe did not share session or preserve policy")
	}
	for _, table := range []string{"messages", "message_identities", "message_events", "event_deliveries", "peer_first_incoming", "send_operations", "event_subscriptions"} {
		var count int
		if err := db.QueryRowContext(request, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("probe persisted rows into %s", table)
		}
	}
}

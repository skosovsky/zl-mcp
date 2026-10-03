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
)

type sendingServiceSource struct {
	authListener
	started chan struct{}
	calls   *atomic.Int32
	quotes  chan *domain.SendQuote
}

func (s *sendingServiceSource) ListenConversations(ctx context.Context, message func(domain.Message) error, _ func(domain.ConversationRef, string) error, connected func() error) error {
	if err := connected(); err != nil {
		return err
	}
	if err := message(domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: "source", SenderID: "peer", Text: "synthetic source", SentAt: time.Now().UTC(), Source: "live", Direction: "incoming", QuoteMetadata: &domain.QuoteMetadata{ClientMessageID: "client-source", MessageType: "webchat", Timestamp: "1791028800000"}}); err != nil {
		return err
	}
	close(s.started)
	<-ctx.Done()
	return ctx.Err()
}

func (s *sendingServiceSource) SendDirect(ctx context.Context, peer, text string, quote *domain.SendQuote) (string, error) {
	s.calls.Add(1)
	s.quotes <- quote
	if text == "synthetic shutdown" {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if text == "synthetic ambiguous" {
		return "", errors.New("synthetic connection lost after request")
	}
	return "accepted-synthetic", nil
}

func TestServiceSendAndReplyUseCollectorSessionAndPersistAcrossRestart(t *testing.T) {
	// Arrange: the production service and MCP transport use one synthetic session.
	dir, err := os.MkdirTemp("/tmp", "zl-send-service-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	var c config.Config
	c.StateDir = dir
	c.Collection.Mode = "all"
	c.Storage.RetentionDays = 90
	c.MCP.Listen = "127.0.0.1:0"
	c.MCP.TokenFile = filepath.Join(dir, "token")
	c.Permissions.AllowSend = true
	c.Permissions.SendRecipientIDs = []string{"peer", "new-peer"}
	token := strings.Repeat("a", 64)
	if err = os.WriteFile(c.MCP.TokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	var calls, restores atomic.Int32
	quotes := make(chan *domain.SendQuote, 4)
	start := func() (string, func()) {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		bound := make(chan net.Addr, 1)
		source := &sendingServiceSource{started: make(chan struct{}), calls: &calls, quotes: quotes}
		go func() {
			done <- run(ctx, c, func(context.Context, string) (collector.ListenerUpstream, error) { restores.Add(1); return source, nil }, func(collector.ListenerUpstream) error { return nil }, func(addr net.Addr) { bound <- addr })
		}()
		stop := func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(5 * time.Second):
				t.Error("service shutdown timeout")
			}
		}
		var addr net.Addr
		select {
		case addr = <-bound:
		case err := <-done:
			cancel()
			t.Fatalf("service startup: %v", err)
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("service startup timeout")
		}
		select {
		case <-source.started:
		case <-time.After(5 * time.Second):
			stop()
			t.Fatal("collector startup timeout")
		}
		return "http://" + addr.String() + "/mcp", stop
	}
	endpoint, stop := start()
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	request := map[string]any{"recipient_id": "peer", "text": "synthetic reply", "reply_to_message_id": "source", "request_id": "80000000-0000-4000-8000-000000000008"}
	call := func(name string, args map[string]any) map[string]any {
		return serviceConversationRPC(t, endpoint, token, "tools/call", map[string]any{"name": name, "arguments": args})["structuredContent"].(map[string]any)
	}
	// Act: a quoted send, exact repeat, restart and status/repeat use the ledger.
	first := call("zalo_send_direct_message", request)
	repeat := call("zalo_send_direct_message", request)
	stop()
	stop = nil
	endpoint, stop = start()
	status := call("zalo_get_send_status", map[string]any{"request_id": request["request_id"]})
	restored := call("zalo_send_direct_message", request)
	// Assert: no extra Zalo session or network send was created by MCP calls.
	for _, result := range []map[string]any{first, repeat, status, restored} {
		if result["status"] != "sent" || result["message_id"] != "accepted-synthetic" {
			t.Fatal("send result not preserved")
		}
	}
	if calls.Load() != 1 || restores.Load() != 2 {
		t.Fatalf("send calls=%d restored sessions=%d", calls.Load(), restores.Load())
	}
	quote := <-quotes
	if quote == nil || quote.MessageID != "source" || quote.SenderID != "peer" || quote.Text != "synthetic source" || quote.Metadata.ClientMessageID != "client-source" {
		t.Fatal("incorrect collector quote")
	}
	// Act: known recipient ID does not require a previously collected dialogue.
	plain := call("zalo_send_direct_message", map[string]any{"recipient_id": "new-peer", "text": "synthetic first unquoted message", "request_id": "90000000-0000-4000-8000-000000000009"})
	// Assert: this is a distinct explicit operation using the existing session.
	if plain["status"] != "sent" || calls.Load() != 2 || restores.Load() != 2 || <-quotes != nil {
		t.Fatal("new-dialogue send required another session or introduced a quote")
	}
	// Act: simulate a lost result, then repeat before and after service restart.
	ambiguousRequest := map[string]any{"recipient_id": "peer", "text": "synthetic ambiguous", "request_id": "a0000000-0000-4000-8000-00000000000a"}
	ambiguous := call("zalo_send_direct_message", ambiguousRequest)
	ambiguousRepeat := call("zalo_send_direct_message", ambiguousRequest)
	stop()
	stop = nil
	endpoint, stop = start()
	ambiguousRestored := call("zalo_send_direct_message", ambiguousRequest)
	// Assert: the persisted uncertainty cannot become a second send.
	for _, result := range []map[string]any{ambiguous, ambiguousRepeat, ambiguousRestored} {
		if result["status"] != "unknown" || result["reason"] != "upstream_ambiguous" || result["message_id"] != nil || result["retry_safe"] != false {
			t.Fatal("ambiguous MCP result was misreported")
		}
	}
	if calls.Load() != 3 || restores.Load() != 3 || <-quotes != nil {
		t.Fatal("ambiguous MCP operation retried after restart")
	}
	// Act: stop the service while an upstream send is waiting for a response.
	shutdownRequest := map[string]any{"recipient_id": "peer", "text": "synthetic shutdown", "request_id": "b0000000-0000-4000-8000-00000000000b"}
	params := map[string]any{"name": "zalo_send_direct_message", "arguments": shutdownRequest, "_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": map[string]any{}}}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", "zalo_send_direct_message")
	req.Header.Set("MCP-Protocol-Version", "2026-07-28")
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err == nil {
			response.Body.Close()
		}
	}()
	select {
	case <-quotes:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown send did not start")
	}
	stop()
	stop = nil
	select {
	case <-requestDone:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown left an HTTP request running")
	}
	endpoint, stop = start()
	shutdownStatus := call("zalo_get_send_status", map[string]any{"request_id": shutdownRequest["request_id"]})
	shutdownRepeat := call("zalo_send_direct_message", shutdownRequest)
	// Assert: cancellation persisted uncertainty before SQLite/session shutdown.
	for _, result := range []map[string]any{shutdownStatus, shutdownRepeat} {
		if result["status"] != "unknown" || result["reason"] != "upstream_ambiguous" {
			t.Fatal("shutdown failed to persist ambiguous result")
		}
	}
	if calls.Load() != 4 || restores.Load() != 4 {
		t.Fatal("interrupted send was repeated after restart")
	}
}

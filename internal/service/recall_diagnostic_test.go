package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type recallTestSender struct {
	calls     atomic.Int32
	ambiguous bool
}

func (s *recallTestSender) AccountID() string { return "10" }
func (s *recallTestSender) SendDirect(context.Context, string, string, *domain.SendQuote) (string, error) {
	panic("diagnostic must not send")
}
func (s *recallTestSender) UndoDiagnosticDirect(_ context.Context, peer, message, client string) (int, error) {
	if peer != "12" || message != "101" || client != "102" {
		panic("wrong recall target")
	}
	s.calls.Add(1)
	if s.ambiguous {
		return 0, errors.New("private upstream marker")
	}
	return 0, nil
}
func recallTestPort(t *testing.T, text, sender string) (*membershipPort, *recallTestSender, string) {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	store, e := storage.OpenWithPolicy(ctx, filepath.Join(dir, "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	if e = store.BindAccount(ctx, "10"); e != nil {
		t.Fatal(e)
	}
	id := "00000000-0000-4000-8000-000000000031"
	if _, e = store.PrepareSend(ctx, domain.SendRequest{RequestID: id, RecipientID: "12", Text: text}); e != nil {
		t.Fatal(e)
	}
	if _, e = store.ClaimSend(ctx, id); e != nil {
		t.Fatal(e)
	}
	message := "101"
	if e = store.CompleteSend(ctx, id, "sent", &message, nil); e != nil {
		t.Fatal(e)
	}
	if e = store.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "12"}, ID: message, SenderID: sender, Text: text, SentAt: time.Now().UTC(), Source: "live", Direction: "outgoing", QuoteMetadata: &domain.QuoteMetadata{ClientMessageID: "102", MessageType: "webchat", Timestamp: "1791280000000"}}); e != nil {
		t.Fatal(e)
	}
	upstream := &recallTestSender{}
	return &membershipPort{stateDir: dir, store: store, sender: upstream}, upstream, id
}
func TestRecallDiagnosticBoundTargetAndRestart(t *testing.T) {
	// Arrange: one exact sent own diagnostic message and an existing session port.
	p, upstream, id := recallTestPort(t, recallDiagnosticText, "10")
	// Act: repeat after rebuilding the port, as on service restart.
	first, e := p.probeRecall(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	next := &membershipPort{stateDir: p.stateDir, store: p.store, sender: upstream}
	second, e := next.probeRecall(context.Background(), id)
	// Assert: one upstream invocation; durable result and send ledger unchanged.
	if e != nil || first.State != "upstream_response" || second.State != first.State || upstream.calls.Load() != 1 {
		t.Fatal("recall redispatched", e, first.State, second.State)
	}
	if op, e := p.store.SendStatus(context.Background(), id); e != nil || op.Status != "sent" {
		t.Fatal("diagnostic changed send ledger", e)
	}
}
func TestRecallDiagnosticAmbiguityDoesNotRedispatch(t *testing.T) {
	// Arrange.
	p, upstream, id := recallTestPort(t, recallDiagnosticText, "10")
	upstream.ambiguous = true
	// Act.
	first, e := p.probeRecall(context.Background(), id)
	if e != nil {
		t.Fatal(e)
	}
	second, e := p.probeRecall(context.Background(), id)
	// Assert.
	if e != nil || first.State != "unknown" || second.State != "unknown" || upstream.calls.Load() != 1 {
		t.Fatal("ambiguous recall retried", e)
	}
}
func TestRecallDiagnosticRejectsUnapprovedTargets(t *testing.T) {
	for _, mode := range []string{"different-text", "foreign-sender", "unknown-operation", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			text, sender := recallDiagnosticText, "10"
			if mode == "different-text" {
				text = "another message"
			}
			if mode == "foreign-sender" {
				sender = "12"
			}
			p, upstream, id := recallTestPort(t, text, sender)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "unknown-operation" {
				id = "00000000-0000-4000-8000-000000000032"
			}
			if mode == "cancelled" {
				cancel()
			}
			// Act.
			_, e := p.probeRecall(ctx, id)
			// Assert.
			if e == nil || upstream.calls.Load() != 0 {
				t.Fatal("unapproved recall reached upstream", e)
			}
		})
	}
}
func TestRecallDiagnosticConcurrentReservation(t *testing.T) {
	// Arrange.
	p, upstream, id := recallTestPort(t, recallDiagnosticText, "10")
	var wg sync.WaitGroup
	// Act: concurrent callers may observe a reserved/incomplete receipt, but cannot dispatch twice.
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); p.probeRecall(context.Background(), id) }()
	}
	wg.Wait()
	// Assert.
	if upstream.calls.Load() != 1 {
		t.Fatal("duplicate recall", upstream.calls.Load())
	}
}

func TestRecallDiagnosticReservedReceiptSurvivesRestart(t *testing.T) {
	// Arrange: crash after durable reservation, before any known upstream outcome.
	p, upstream, id := recallTestPort(t, recallDiagnosticText, "10")
	dir := filepath.Join(p.stateDir, "diagnostic-recalls")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256([]byte("10\x00" + id))
	name := hex.EncodeToString(digest[:]) + ".receipt"
	if e := os.WriteFile(filepath.Join(dir, name), []byte(`{"state":"reserved"}`), 0600); e != nil {
		t.Fatal(e)
	}
	// Act.
	result, e := p.probeRecall(context.Background(), id)
	// Assert: uncertain dispatch ownership cannot be reacquired.
	if e != nil || result.State != "reserved" || upstream.calls.Load() != 0 {
		t.Fatal("reserved receipt redispatched", e)
	}
}
func TestRecallControlRejectsAmbiguousArguments(t *testing.T) {
	for _, body := range []string{
		`{"method":"cli_probe_recall","arguments":{"send_request_id":"00000000-0000-4000-8000-000000000031","message_id":"101"}}`,
		`{"method":"cli_probe_recall","arguments":{"send_request_id":"00000000-0000-4000-8000-000000000031","send_request_id":"00000000-0000-4000-8000-000000000032"}}`,
	} {
		// Arrange: exact source exists, but the owner request violates its executable contract.
		p, upstream, _ := recallTestPort(t, recallDiagnosticText, "10")
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("POST", "/rpc", strings.NewReader(body))
		// Act.
		p.cliHandler().ServeHTTP(recorder, request)
		// Assert.
		if recorder.Code != 400 || upstream.calls.Load() != 0 {
			t.Fatal("ambiguous request reached upstream", recorder.Code)
		}
	}
}

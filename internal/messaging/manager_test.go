package messaging

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestStorageFailureDoesNotMasqueradeAsMissingSendOrQuote(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	s, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "state.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	calls := 0
	m := Manager{Store: s, Enabled: true, Sender: senderFunc(func(context.Context, string, string, *domain.SendQuote) (string, error) {
		calls++
		return "unexpected", nil
	})}
	args := map[string]any{"recipient_id": "peer", "text": "reply", "reply_to_message_id": "original", "request_id": "70000000-0000-4000-8000-000000000007"}
	if _, err = s.DB.Exec("DROP TABLE message_quote_metadata"); err != nil {
		t.Fatal(err)
	}
	// Act.
	_, quoteError := m.Call(ctx, "zalo_send_direct_message", args)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	_, statusError := m.Call(ctx, "zalo_get_send_status", map[string]any{"request_id": args["request_id"]})
	// Assert.
	for _, observed := range []error{quoteError, statusError} {
		var failure *domain.Error
		if !errors.As(observed, &failure) || failure.Code != "STORAGE_ERROR" {
			t.Fatalf("misclassified storage error: %v", observed)
		}
	}
	if calls != 0 {
		t.Fatal("failed quote lookup triggered send")
	}
}

type senderFunc func(context.Context, string, string, *domain.SendQuote) (string, error)

func (f senderFunc) SendDirect(c context.Context, p, t string, q *domain.SendQuote) (string, error) {
	return f(c, p, t, q)
}

func TestManagerNeverRetriesAmbiguousSend(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	s, err := storage.Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var calls atomic.Int32
	m := Manager{Store: s, Enabled: true, Sender: senderFunc(func(context.Context, string, string, *domain.SendQuote) (string, error) {
		calls.Add(1)
		return "", errors.New("private-error-marker")
	})}
	a := map[string]any{"recipient_id": "peer", "text": "hello", "request_id": "30000000-0000-4000-8000-000000000003"}
	// Act.
	first, err := m.Call(ctx, "zalo_send_direct_message", a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Call(ctx, "zalo_send_direct_message", a)
	if err != nil {
		t.Fatal(err)
	}
	// Assert.
	if calls.Load() != 1 || first["status"] != "unknown" || second["status"] != "unknown" || second["reason"] != "upstream_ambiguous" {
		t.Fatal("ambiguous operation repeated or exposed raw error")
	}
}

func TestManagerReplyRequiresExactRetainedSourceAndPermission(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	s, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "state.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	msg := domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: "message", SenderID: "peer", Text: "source", SentAt: time.Now(), Source: "live", QuoteMetadata: &domain.QuoteMetadata{ClientMessageID: "client-id", MessageType: "webchat", Timestamp: "1791029400000"}}
	if err = s.Put(ctx, msg); err != nil {
		t.Fatal(err)
	}
	calls := 0
	m := Manager{Store: s, Enabled: true, Recipients: map[string]bool{"peer": true}, Sender: senderFunc(func(_ context.Context, p, text string, q *domain.SendQuote) (string, error) {
		calls++
		if p != "peer" || q == nil || q.MessageID != "message" || q.Text != "source" || q.Metadata.ClientMessageID != "client-id" {
			t.Fatal("quote crossed source boundary")
		}
		return "accepted", nil
	})}
	a := map[string]any{"recipient_id": "other", "text": "reply", "reply_to_message_id": "message", "request_id": "40000000-0000-4000-8000-000000000004"}
	// Act / Assert: denied receiver never sends.
	if _, err = m.Call(ctx, "zalo_send_direct_message", a); err == nil || calls != 0 {
		t.Fatal("permission ignored")
	}
	// Act / Assert: permission to another peer cannot quote this peer's record.
	m.Recipients["other"] = true
	_, err = m.Call(ctx, "zalo_send_direct_message", a)
	var quoteError *domain.Error
	if !errors.As(err, &quoteError) || quoteError.Code != "QUOTE_UNAVAILABLE" || calls != 0 {
		t.Fatal("quote crossed an allowed recipient boundary")
	}
	a["recipient_id"] = "peer"
	a["request_id"] = "40000000-0000-4000-8000-000000000014"
	r, err := m.Call(ctx, "zalo_send_direct_message", a)
	if err != nil || r["status"] != "sent" || calls != 1 {
		t.Fatal("reply failed", err)
	}
	if err = s.DeleteConversation(ctx, msg.Ref(), msg.ID); err != nil {
		t.Fatal(err)
	}
	a["request_id"] = "50000000-0000-4000-8000-000000000005"
	if _, err = m.Call(ctx, "zalo_send_direct_message", a); err == nil || calls != 1 {
		t.Fatal("deleted quote was sent")
	}
	var n int
	if err = s.DB.QueryRow("SELECT count(*) FROM message_quote_metadata").Scan(&n); err != nil || n != 0 {
		t.Fatal("quote metadata survived deletion")
	}
}

func TestConfirmedResultSurvivesCallerCancellation(t *testing.T) {
	// Arrange.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := storage.Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := Manager{Store: s, Enabled: true, Sender: senderFunc(func(context.Context, string, string, *domain.SendQuote) (string, error) {
		cancel()
		return "confirmed", nil
	})}
	a := map[string]any{"recipient_id": "peer", "text": "hello", "request_id": "60000000-0000-4000-8000-000000000006"}
	// Act.
	r, err := m.Call(ctx, "zalo_send_direct_message", a)
	// Assert.
	if err != nil || r["status"] != "sent" || r["message_id"] != "confirmed" {
		t.Fatal("confirmed result lost after cancellation", err)
	}
}

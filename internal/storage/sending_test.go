package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestReplayRepairsQuoteOnlyForMatchingRetainedMessage(t *testing.T) {
	// Arrange: an old retained message has no upstream quote metadata.
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "state.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "peer"}, ID: "original", SenderID: "peer", Text: "retained synthetic text", SentAt: time.Now().UTC(), Source: "live", Direction: "incoming"}
	if err = s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	replay := m
	replay.Source = "replay"
	replay.QuoteMetadata = &domain.QuoteMetadata{ClientMessageID: "client-original", MessageType: "webchat", Timestamp: "1791029400000"}
	replay.Text = "conflicting body"
	// Act: mismatching replay cannot attach metadata; matching replay can.
	if err = s.Put(ctx, replay); err != nil {
		t.Fatal(err)
	}
	_, mismatch := s.SendQuote(ctx, "peer", m.ID)
	replay.Text = m.Text
	if err = s.Put(ctx, replay); err != nil {
		t.Fatal(err)
	}
	quote, err := s.SendQuote(ctx, "peer", m.ID)
	var messages, events int
	if e := s.DB.QueryRow("SELECT count(*) FROM messages").Scan(&messages); e != nil {
		t.Fatal(e)
	}
	if e := s.DB.QueryRow("SELECT count(*) FROM message_events").Scan(&events); e != nil {
		t.Fatal(e)
	}
	// Assert: quote repair does not duplicate the message or event, or change text.
	if mismatch == nil || err != nil || quote.Text != m.Text || quote.Metadata.ClientMessageID != "client-original" || messages != 1 || events != 1 {
		t.Fatalf("quote repair invariant: mismatch=%v err=%v messages=%d events=%d", mismatch, err, messages, events)
	}
}

func TestSendJournalClaimsOnceAndRecoversAmbiguousResult(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s, err := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	r := domain.SendRequest{RecipientID: "peer", Text: "private synthetic text", RequestID: "10000000-0000-4000-8000-000000000001"}
	if _, err = s.PrepareSend(ctx, r); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	// Act.
	for range 20 {
		wg.Go(func() {
			ok, e := s.ClaimSend(ctx, r.RequestID)
			if e != nil {
				t.Error(e)
			}
			if ok {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.RecoverInterruptedSends(ctx); err != nil {
		t.Fatal(err)
	}
	op, err := s.PrepareSend(ctx, r)
	claimed, claimErr := s.ClaimSend(ctx, r.RequestID)
	// Assert.
	if wins.Load() != 1 || err != nil || op.Status != "unknown" || op.Reason == nil || *op.Reason != "interrupted" || claimed || claimErr != nil {
		t.Fatalf("wins=%d status=%s claimed=%v err=%v", wins.Load(), op.Status, claimed, err)
	}
	var fingerprint string
	if err = s.DB.QueryRow("SELECT fingerprint FROM send_operations").Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	if fingerprint == r.Text || strings.Contains(fingerprint, r.Text) {
		t.Fatal("ledger retained text")
	}
}

func TestSendJournalRejectsChangedArgumentsAndNeverResendsTerminal(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := domain.SendRequest{RecipientID: "unknown-peer", Text: "hello", RequestID: "20000000-0000-4000-8000-000000000002"}
	if _, err = s.PrepareSend(ctx, r); err != nil {
		t.Fatal(err)
	}
	// Act.
	ok, err := s.ClaimSend(ctx, r.RequestID)
	if !ok || err != nil {
		t.Fatal("claim failed", err)
	}
	id := "upstream-id"
	if err = s.CompleteSend(ctx, r.RequestID, "sent", &id, nil); err != nil {
		t.Fatal(err)
	}
	op, err := s.PrepareSend(ctx, r)
	r.Text = "changed"
	_, conflict := s.PrepareSend(ctx, r)
	claimed, claimErr := s.ClaimSend(ctx, r.RequestID)
	// Assert.
	if err != nil || op.Status != "sent" || op.MessageID == nil || *op.MessageID != id || op.RetrySafe || !errors.Is(conflict, ErrSendConflict) || claimed || claimErr != nil {
		t.Fatal("terminal send or conflict invariant failed")
	}
}

package collector

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type historyListener struct {
	*scriptedListener
	fetch func(context.Context, domain.ConversationRef, string, int) (domain.HistoryPage, error)
}

func (h *historyListener) HistoryPage(ctx context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
	return h.fetch(ctx, ref, cursor, limit)
}

func TestHistoryGuardSharesAuthenticationCancellation(t *testing.T) {
	// Arrange
	started := make(chan struct{})
	var calls atomic.Int32
	source := &historyListener{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}}
	source.fetch = func(ctx context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		return domain.HistoryPage{}, ctx.Err()
	}
	guard := newSessionGuard(context.Background(), source)
	defer guard.cancel()
	done := make(chan error, 1)
	go func() {
		_, err := guard.HistoryPage(context.Background(), domain.ConversationRef{Type: "group", ID: "group"}, "0", 2)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("history request did not start")
	}
	// Act
	guard.observe(domain.ErrAuthenticationRequired)
	// Assert
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrAuthenticationRequired) {
			t.Fatal("auth cancellation lost", err)
		}
	case <-time.After(time.Second):
		t.Fatal("history request not cancelled")
	}
	_, err := guard.HistoryPage(context.Background(), domain.ConversationRef{Type: "group", ID: "group"}, "0", 2)
	if !errors.Is(err, domain.ErrAuthenticationRequired) || calls.Load() != 1 {
		t.Fatal("new history call crossed stopped auth boundary")
	}
}

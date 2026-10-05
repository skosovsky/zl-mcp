package collector

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type preloadListener struct {
	*scriptedListener
	fetch func(context.Context) (domain.PreloadSnapshot, error)
}

func (s *preloadListener) ConversationPreload(ctx context.Context) (domain.PreloadSnapshot, error) {
	return s.fetch(ctx)
}

func TestPreloadGuardStopsAtSharedAuthBoundary(t *testing.T) {
	// Arrange
	started := make(chan struct{})
	var calls atomic.Int32
	source := &preloadListener{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}}
	source.fetch = func(ctx context.Context) (domain.PreloadSnapshot, error) {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		return domain.PreloadSnapshot{}, ctx.Err()
	}
	guard := newSessionGuard(context.Background(), source)
	defer guard.cancel()
	done := make(chan error, 1)
	go func() { _, err := guard.ConversationPreload(context.Background()); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("preload did not start")
	}
	// Act
	guard.observe(domain.ErrAuthenticationRequired)
	// Assert
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrAuthenticationRequired) {
			t.Fatal("preload auth cancellation lost")
		}
	case <-time.After(time.Second):
		t.Fatal("preload remained active after auth loss")
	}
	if _, err := guard.ConversationPreload(context.Background()); !errors.Is(err, domain.ErrAuthenticationRequired) || calls.Load() != 1 {
		t.Fatal("new preload request crossed stopped session")
	}
}

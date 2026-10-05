package collector

import (
	"context"
	"errors"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"sync/atomic"
	"testing"
	"time"
)

type blockedIdentitySource struct {
	*scriptedListener
	started chan struct{}
	calls   atomic.Int32
}

func (s *blockedIdentitySource) MapMobileBackupIdentities(ctx context.Context, _ domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
	s.calls.Add(1)
	close(s.started)
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestMobileIdentityGuardCancelsSharedSession(t *testing.T) {
	// Arrange.
	source := &blockedIdentitySource{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}, started: make(chan struct{})}
	source.listen = func(context.Context, func(domain.Message) error, func(string, string) error, func() error) error {
		return domain.ErrAuthenticationRequired
	}
	guard := newSessionGuard(context.Background(), source)
	defer guard.cancel()
	done := make(chan error, 1)
	go func() {
		_, err := guard.MapMobileBackupIdentities(context.Background(), domain.MobileIdentityRequest{})
		done <- err
	}()
	select {
	case <-source.started:
	case <-time.After(time.Second):
		t.Fatal("mapping not started")
	}
	// Act.
	if err := guard.Listen(context.Background(), nil, nil, nil); !errors.Is(err, domain.ErrAuthenticationRequired) {
		t.Fatal("missing auth failure")
	}
	// Assert: in-flight mapping stops and later calls never reach the SDK.
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrAuthenticationRequired) {
			t.Fatal("mapping ignored shared auth failure")
		}
	case <-time.After(time.Second):
		t.Fatal("mapping not cancelled")
	}
	if _, err := guard.MapMobileBackupIdentities(context.Background(), domain.MobileIdentityRequest{}); !errors.Is(err, domain.ErrAuthenticationRequired) || source.calls.Load() != 1 {
		t.Fatal("revoked session reached source")
	}
}

type completedIdentitySource struct {
	*scriptedListener
	revoke func()
}

func (s *completedIdentitySource) MapMobileBackupIdentities(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
	s.revoke()
	return []domain.MobileIdentityPair{{Plain: "1", Session: "2"}}, nil
}
func TestMobileIdentityGuardDiscardsSuccessAfterRevocation(t *testing.T) {
	// Arrange: another request revokes the session just before this result returns.
	source := &completedIdentitySource{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}}
	guard := newSessionGuard(context.Background(), source)
	defer guard.cancel()
	source.revoke = func() { guard.observe(domain.ErrAuthenticationRequired) }
	// Act.
	result, err := guard.MapMobileBackupIdentities(context.Background(), domain.MobileIdentityRequest{Direct: []string{"1"}})
	// Assert: successful upstream data cannot escape a revoked session.
	if !errors.Is(err, domain.ErrAuthenticationRequired) || result != nil {
		t.Fatal("revoked successful mapping exposed")
	}
}

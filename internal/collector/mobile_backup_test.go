package collector

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type blockedMobileSource struct {
	*scriptedListener
	started chan struct{}
	calls   atomic.Int32
}

func (s *blockedMobileSource) ReceiveMobileBackupOffer(ctx context.Context, _ *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
	s.calls.Add(1)
	close(s.started)
	<-ctx.Done()
	return domain.MobileBackupOffer{}, ctx.Err()
}

func TestMobileBackupGuardCancelsOnSharedAuthenticationFailure(t *testing.T) {
	// Arrange: an in-flight mobile wait uses the same session as collection.
	upstream := &blockedMobileSource{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}, started: make(chan struct{})}
	upstream.listen = func(context.Context, func(domain.Message) error, func(string, string) error, func() error) error {
		return domain.ErrAuthenticationRequired
	}
	guard := newSessionGuard(context.Background(), upstream)
	defer guard.cancel()
	done := make(chan error, 1)
	go func() { _, err := guard.ReceiveMobileBackupOffer(context.Background(), nil); done <- err }()
	select {
	case <-upstream.started:
	case <-time.After(time.Second):
		t.Fatal("wait not started")
	}
	// Act.
	err := guard.Listen(context.Background(), nil, nil, nil)
	// Assert: active wait is cancelled and later dispatch cannot reach the source.
	if !errors.Is(err, domain.ErrAuthenticationRequired) {
		t.Fatal("auth failure missing")
	}
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrAuthenticationRequired) {
			t.Fatal("wait auth failure missing")
		}
	case <-time.After(time.Second):
		t.Fatal("wait did not cancel")
	}
	_, err = guard.ReceiveMobileBackupOffer(context.Background(), nil)
	if !errors.Is(err, domain.ErrAuthenticationRequired) || upstream.calls.Load() != 1 {
		t.Fatal("new request reached revoked session")
	}
}

type completedMobileSource struct {
	*scriptedListener
	revoke func()
}

func (s *completedMobileSource) ReceiveMobileBackupOffer(context.Context, *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
	s.revoke()
	return domain.MobileBackupOffer{URL: "https://synthetic.invalid/backup", KeyText: "synthetic"}, nil
}
func TestMobileBackupGuardDiscardsSuccessAfterRevocation(t *testing.T) {
	// Arrange: the session is revoked while a successful offer returns.
	source := &completedMobileSource{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}}
	guard := newSessionGuard(context.Background(), source)
	defer guard.cancel()
	source.revoke = func() { guard.observe(domain.ErrAuthenticationRequired) }
	// Act.
	offer, err := guard.ReceiveMobileBackupOffer(context.Background(), nil)
	// Assert: no key-bearing offer escapes the revoked session.
	if !errors.Is(err, domain.ErrAuthenticationRequired) || offer != (domain.MobileBackupOffer{}) {
		t.Fatal("revoked offer exposed")
	}
}

func TestMobileBackupScopeCancelsNonSDKStagesOnRevocation(t *testing.T) {
	// Arrange: an archive download/parse borrows the same scope as SDK operations.
	upstream := &scriptedListener{fakeZalo: &fakeZalo{}}
	upstream.listen = func(context.Context, func(domain.Message) error, func(string, string) error, func() error) error {
		return domain.ErrAuthenticationRequired
	}
	guard := newSessionGuard(context.Background(), upstream)
	defer guard.cancel()
	operation, stop, e := guard.MobileBackupContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer stop()
	// Act: collector receives authentication loss while a non-SDK stage owns scope.
	e = guard.Listen(context.Background(), nil, nil, nil)
	// Assert: borrowed scope cancels, later stages/scopes cannot use revoked session.
	if !errors.Is(e, domain.ErrAuthenticationRequired) {
		t.Fatal("auth revocation missing")
	}
	select {
	case <-operation.Done():
	case <-time.After(time.Second):
		t.Fatal("archive stage not cancelled")
	}
	if _, _, e = guard.MobileBackupContext(context.Background()); !errors.Is(e, domain.ErrAuthenticationRequired) {
		t.Fatal("revoked scope granted")
	}
}

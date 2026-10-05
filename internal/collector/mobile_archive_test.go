package collector

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type revokedArchiveSource struct {
	*scriptedListener
	revoke func()
	data   []byte
}

func (s *revokedArchiveSource) ConsumeMobileArchive(context.Context, *http.Request, *http.Client, func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	s.revoke()
	return s.data, nil
}
func TestMobileArchiveGuardDiscardsLateCiphertext(t *testing.T) {
	// Arrange: authentication is revoked as the archive's successful read completes.
	source := &revokedArchiveSource{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}, data: []byte("synthetic ciphertext")}
	guard := newSessionGuard(context.Background(), source)
	defer guard.cancel()
	source.revoke = func() { guard.observe(domain.ErrAuthenticationRequired) }
	// Act.
	result, e := guard.ConsumeMobileArchive(context.Background(), nil, nil, nil)
	// Assert: neither stale ciphertext nor a successful prefix escapes shared revocation.
	if !errors.Is(e, domain.ErrAuthenticationRequired) || result != nil || !bytes.Equal(source.data, make([]byte, len(source.data))) {
		t.Fatal("revoked archive exposed")
	}
}

type scopedArchiveBodySource struct {
	*scriptedListener
	entered chan context.Context
}

func (s *scopedArchiveBodySource) ConsumeMobileArchive(ctx context.Context, _ *http.Request, _ *http.Client, consume func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	return consume(ctx, &http.Response{StatusCode: 200})
}
func TestMobileArchiveRevocationCancelsBodyConsumer(t *testing.T) {
	// Arrange: a body consumer holds the same guarded session request open.
	source := &scopedArchiveBodySource{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}, entered: make(chan context.Context, 1)}
	guard := newSessionGuard(context.Background(), source)
	defer guard.cancel()
	done := make(chan error, 1)
	data := []byte("synthetic partial archive")
	go func() {
		result, e := guard.ConsumeMobileArchive(context.Background(), nil, nil, func(ctx context.Context, _ *http.Response) ([]byte, error) {
			source.entered <- ctx
			<-ctx.Done()
			return data, nil
		})
		if result != nil {
			done <- errors.New("partial result escaped")
			return
		}
		done <- e
	}()
	var active context.Context
	select {
	case active = <-source.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("body consumer not started")
	}
	// Act: another operation observes authentication loss while body consumption waits.
	guard.observe(domain.ErrAuthenticationRequired)
	// Assert: scope covers the whole body and clears late successful bytes.
	var e error
	select {
	case e = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("revoked consumer did not stop")
	}
	if !errors.Is(e, domain.ErrAuthenticationRequired) || active.Err() == nil || !bytes.Equal(data, make([]byte, len(data))) {
		t.Fatal("body consumer survived revocation", e)
	}
}

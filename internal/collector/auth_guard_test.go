package collector

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type blockingGroupListener struct {
	*fakeZalo
	started  chan struct{}
	groups   atomic.Int32
	listener atomic.Int32
}

func (b *blockingGroupListener) Group(ctx context.Context, _ string) (domain.Group, *string, error) {
	b.groups.Add(1)
	close(b.started)
	<-ctx.Done()
	return domain.Group{}, nil, ctx.Err()
}
func (b *blockingGroupListener) Listen(context.Context, func(domain.Message) error, func(string, string) error, func() error) error {
	b.listener.Add(1)
	return domain.ErrAuthenticationRequired
}
func TestAuthGuardCancelsInFlightAndRejectsNewMembershipCalls(t *testing.T) {
	// Arrange: a group read is already in flight when listener authentication fails.
	manager, _ := manager(t)
	upstream := &blockingGroupListener{fakeZalo: &fakeZalo{}, started: make(chan struct{})}
	guard := newSessionGuard(context.Background(), upstream)
	defer guard.cancel()
	manager.API = guard
	done := make(chan error, 1)
	go func() { _, _, err := guard.Group(context.Background(), "g"); done <- err }()
	select {
	case <-upstream.started:
	case <-time.After(time.Second):
		t.Fatal("group read did not start")
	}
	// Act: mark the shared session unavailable before collector cleanup begins.
	listenerErr := guard.Listen(context.Background(), nil, nil, nil)
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrAuthenticationRequired) {
			t.Fatal("in-flight read returned wrong failure", err)
		}
	case <-time.After(time.Second):
		t.Fatal("in-flight read was not cancelled")
	}
	for _, request := range []struct {
		method string
		args   map[string]any
	}{
		{"zalo_get_group", map[string]any{"group_id": "g"}},
		{"zalo_inspect_invite", map[string]any{"invite_url": "https://zalo.me/g/example"}},
		{"zalo_join_group", map[string]any{"plan_token": "synthetic-unused-token", "request_id": uuid.NewString()}},
	} {
		_, err := manager.Call(context.Background(), request.method, request.args)
		var typed *domain.Error
		if !errors.As(err, &typed) || typed.Code != "NOT_AUTHENTICATED" {
			t.Fatalf("%s: %v", request.method, err)
		}
	}
	err := guard.Listen(context.Background(), nil, nil, nil)
	// Assert: no additional forwarding or ledger mutation after the auth boundary.
	var requests int
	if scanErr := manager.Store.DB.QueryRow("SELECT count(*) FROM join_requests").Scan(&requests); scanErr != nil {
		t.Fatal(scanErr)
	}
	if !errors.Is(listenerErr, domain.ErrAuthenticationRequired) || !errors.Is(err, domain.ErrAuthenticationRequired) || upstream.groups.Load() != 1 || upstream.listener.Load() != 1 || requests != 0 {
		t.Fatal("auth guard permitted another call or mutation")
	}
	// A saved-result read is still dispatched locally, despite missing upstream.
	_, err = manager.Call(context.Background(), "zalo_get_join_status", map[string]any{"operation_id": "missing"})
	var typed *domain.Error
	if errors.As(err, &typed) && typed.Code == "NOT_AUTHENTICATED" {
		t.Fatal("saved result read incorrectly needs authentication")
	}
}
func TestAuthGuardKeepsNetworkFailuresRetryable(t *testing.T) {
	// Arrange: an ordinary connection failure, rather than an explicit auth rejection.
	var calls atomic.Int32
	upstream := &scriptedListener{fakeZalo: &fakeZalo{}}
	upstream.listen = func(context.Context, func(domain.Message) error, func(string, string) error, func() error) error {
		calls.Add(1)
		return errors.New("synthetic connection failure")
	}
	guard := newSessionGuard(context.Background(), upstream)
	defer guard.cancel()
	// Act
	first := guard.Listen(context.Background(), nil, nil, nil)
	second := guard.Listen(context.Background(), nil, nil, nil)
	// Assert: a network failure cannot disable the account or force login.
	if first == nil || second == nil || guard.AuthenticationRequired() || calls.Load() != 2 {
		t.Fatal("network failure was treated as auth failure")
	}
}

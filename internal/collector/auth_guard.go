package collector

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// sessionGuard closes the network boundary immediately on a known auth failure,
// independently of state persistence and draining pending join operations.
type sessionGuard struct {
	upstream     ListenerUpstream
	ctx          context.Context
	cancel       context.CancelFunc
	authRequired atomic.Bool
}

func newSessionGuard(parent context.Context, client ListenerUpstream) *sessionGuard {
	ctx, cancel := context.WithCancel(parent)
	return &sessionGuard{upstream: client, ctx: ctx, cancel: cancel}
}
func (g *sessionGuard) AuthenticationRequired() bool { return g.authRequired.Load() }
func (g *sessionGuard) AccountID() string            { return g.upstream.AccountID() }
func (g *sessionGuard) request(parent context.Context) (context.Context, func(), error) {
	if g.AuthenticationRequired() {
		return nil, nil, domain.ErrAuthenticationRequired
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(g.ctx, cancel)
	if g.ctx.Err() != nil {
		cancel()
	}
	if g.AuthenticationRequired() {
		stop()
		cancel()
		return nil, nil, domain.ErrAuthenticationRequired
	}
	return ctx, func() { stop(); cancel() }, nil
}
func (g *sessionGuard) observe(err error) error {
	if errors.Is(err, domain.ErrAuthenticationRequired) {
		g.authRequired.Store(true)
		g.cancel()
	}
	// In-flight requests cancelled by another operation's auth failure are auth
	// failures too, rather than misleading generic network errors.
	if err != nil && g.AuthenticationRequired() {
		return domain.ErrAuthenticationRequired
	}
	return err
}
func (g *sessionGuard) Groups(parent context.Context) ([]domain.Group, error) {
	ctx, stop, err := g.request(parent)
	if err != nil {
		return nil, err
	}
	defer stop()
	groups, err := g.upstream.Groups(ctx)
	return groups, g.observe(err)
}
func (g *sessionGuard) Group(parent context.Context, id string) (domain.Group, *string, error) {
	ctx, stop, err := g.request(parent)
	if err != nil {
		return domain.Group{}, nil, err
	}
	defer stop()
	group, description, err := g.upstream.Group(ctx, id)
	return group, description, g.observe(err)
}
func (g *sessionGuard) Inspect(parent context.Context, url string) (domain.Invite, error) {
	ctx, stop, err := g.request(parent)
	if err != nil {
		return domain.Invite{}, err
	}
	defer stop()
	invite, err := g.upstream.Inspect(ctx, url)
	return invite, g.observe(err)
}
func (g *sessionGuard) Join(parent context.Context, url string) error {
	ctx, stop, err := g.request(parent)
	if err != nil {
		return err
	}
	defer stop()
	return g.observe(g.upstream.Join(ctx, url))
}
func (g *sessionGuard) Listen(parent context.Context, message func(domain.Message) error, deletion func(string, string) error, connected func() error) error {
	ctx, stop, err := g.request(parent)
	if err != nil {
		return err
	}
	defer stop()
	return g.observe(g.upstream.Listen(ctx, message, deletion, connected))
}

package collector

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (g *sessionGuard) MapMobileBackupIdentities(parent context.Context, request domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
	source, ok := g.upstream.(domain.MobileIdentitySource)
	if !ok {
		return nil, domain.ErrHistoryUnsupported
	}
	ctx, stop, err := g.request(parent)
	if err != nil {
		return nil, err
	}
	defer stop()
	result, err := source.MapMobileBackupIdentities(ctx, request)
	if g.AuthenticationRequired() {
		err = domain.ErrAuthenticationRequired
	} else if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err = g.observe(err); err != nil {
		return nil, err
	}
	return result, nil
}

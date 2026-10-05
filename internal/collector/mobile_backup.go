package collector

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (g *sessionGuard) ReceiveMobileBackupOffer(parent context.Context, progress *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
	source, ok := g.upstream.(domain.MobileBackupSource)
	if !ok {
		return domain.MobileBackupOffer{}, domain.ErrHistoryUnsupported
	}
	ctx, stop, err := g.request(parent)
	if err != nil {
		return domain.MobileBackupOffer{}, err
	}
	defer stop()
	offer, err := source.ReceiveMobileBackupOffer(ctx, progress)
	if g.AuthenticationRequired() {
		err = domain.ErrAuthenticationRequired
	} else if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err = g.observe(err); err != nil {
		return domain.MobileBackupOffer{}, err
	}
	return offer, nil
}

func (g *sessionGuard) MobileBackupContext(parent context.Context) (context.Context, func(), error) {
	if parent == nil {
		return nil, nil, domain.ErrMobileBackupInvalid
	}
	return g.request(parent)
}

package collector

import (
	"context"
	"net/http"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (g *sessionGuard) ConsumeMobileArchive(parent context.Context, req *http.Request, client *http.Client, consume func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	source, ok := g.upstream.(domain.MobileArchiveTransport)
	if !ok {
		return nil, domain.ErrHistoryUnsupported
	}
	ctx, stop, err := g.request(parent)
	if err != nil {
		return nil, err
	}
	defer stop()
	data, err := source.ConsumeMobileArchive(ctx, req, client, consume)
	if g.AuthenticationRequired() {
		err = domain.ErrAuthenticationRequired
	} else if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err = g.observe(err); err != nil {
		clear(data)
		return nil, err
	}
	return data, nil
}

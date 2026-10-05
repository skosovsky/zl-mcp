package zalo

import (
	"context"
	"net/http"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type mobileArchiveRequester interface {
	ConsumeMobileArchive(context.Context, *http.Request, *http.Client, func(context.Context, *http.Response) ([]byte, error)) ([]byte, error)
}

func (c *Client) ConsumeMobileArchive(ctx context.Context, req *http.Request, client *http.Client, consume func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	source, ok := c.api.(mobileArchiveRequester)
	if !ok {
		return nil, domain.ErrHistoryUnsupported
	}
	owner := c.AccountID()
	if owner == "" {
		return nil, domain.ErrAuthenticationRequired
	}
	data, err := source.ConsumeMobileArchive(ctx, req, client, consume)
	if err != nil || ctx == nil || ctx.Err() != nil || c.AccountID() != owner {
		clear(data)
		return nil, domain.ErrMobileBackupInvalid
	}
	return data, nil
}

package zalo

import (
	"context"
	"net/http"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup/archiveport"
)

var _ archiveport.Transport = (*Client)(nil)

func (c *Client) ConsumeMobileArchive(ctx context.Context, req *http.Request, client *http.Client, consume func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	source, ok := c.api.(archiveport.Transport)
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

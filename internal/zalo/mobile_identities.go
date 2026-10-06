package zalo

import (
	"context"
	"errors"
	"log/slog"

	"github.com/amrakk/zcago/errs"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
)

type mobileIdentityRequester interface {
	GetMobileIdentityMapping(context.Context, []string, []string) ([]byte, error)
}

func (c *Client) MapMobileBackupIdentities(ctx context.Context, request domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
	mapper, ok := c.api.(mobileIdentityRequester)
	if !ok {
		return nil, domain.ErrHistoryUnsupported
	}
	owner := c.AccountID()
	if owner == "" {
		return nil, domain.ErrAuthenticationRequired
	}
	result, err := mapMobileIdentities(ctx, request, mapper)
	if c.AccountID() != owner {
		return nil, domain.ErrMobileBackupInvalid
	}
	return result, err
}

func mapMobileIdentities(ctx context.Context, request domain.MobileIdentityRequest, mapper mobileIdentityRequester) ([]domain.MobileIdentityPair, error) {
	if ctx == nil || mapper == nil {
		return nil, domain.ErrMobileBackupInvalid
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	payload, err := mobilebackup.IdentityPayload(request)
	if err != nil {
		return nil, domain.ErrMobileBackupInvalid
	}
	clear(payload)
	body, err := mapper.GetMobileIdentityMapping(ctx, request.Direct, request.Groups)
	defer clear(body)
	if errors.Is(err, errs.ErrAuthenticationRequired) {
		return nil, domain.ErrAuthenticationRequired
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		slog.Warn("mobile_identity_failed", "stage", "SDK_REQUEST")
		return nil, domain.ErrMobileBackupInvalid
	}
	result, err := mobilebackup.DecodeIdentities(request, body)
	if err != nil {
		slog.Warn("mobile_identity_failed", "stage", "MAPPING_SHAPE")
		return nil, domain.ErrMobileBackupInvalid
	}
	return result, nil
}

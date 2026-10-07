package service

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func (p *membershipPort) probeMobileOffer(ctx context.Context, id string, revision int64) (map[string]any, error) {
	attempt, err := p.store.MobileBackupAttempt(ctx, id)
	if err != nil {
		return nil, err
	}
	if attempt.Request.ArchiveScope != "" || attempt.State != "prepared" || attempt.Revision != revision {
		return nil, storage.ErrMobileBackupState
	}
	request, stop := context.WithTimeout(ctx, 185*time.Second)
	defer stop()
	offer, err := p.runPreparedMobileOffer(request, id)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(offer.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" && u.Port() != "443" || u.Fragment != "" || offer.FileSize == 0 {
		return nil, domain.Invalid("Mobile offer metadata is unavailable.")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || net.ParseIP(host) != nil || strings.HasSuffix(host, ".") {
		return nil, domain.Invalid("Mobile offer host is not a DNS name.")
	}
	attempt, err = p.store.MobileBackupAttempt(request, id)
	if err != nil {
		return nil, err
	}
	if request.Err() != nil || attempt.State != "offer_ready" {
		return nil, storage.ErrMobileBackupState
	}
	return map[string]any{"attempt": attempt, "download_host": host, "archive_bytes": strconv.FormatUint(offer.FileSize, 10), "within_archive_budget": offer.FileSize <= uint64(attempt.Request.MaxArchiveBytes), "download_performed": false, "import_performed": false}, nil
}

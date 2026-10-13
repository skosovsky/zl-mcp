package storage

import (
	"context"
	"errors"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// ObservationQueueDepth borrows the native ledgers. These SELECTs cannot claim,
// recover or replay operations, and are used only with recording enabled.
func (s *Store) ObservationQueueDepth(ctx context.Context, kind domain.WorkKind) (int64, error) {
	var query string
	switch kind {
	case domain.WorkSend:
		query = "SELECT count(*) FROM send_operations WHERE state IN ('pending','sending')"
	case domain.WorkJoin:
		query = "SELECT count(*) FROM join_requests WHERE json_extract(payload,'$.status')='running'"
	case domain.WorkHistory:
		query = "SELECT count(*) FROM history_operations WHERE state IN ('queued','running','paused')"
	case domain.WorkEvents:
		query = "SELECT count(*) FROM event_deliveries WHERE state IN ('pending','sending')"
	case domain.WorkCollector, domain.WorkOther:
		return 0, errors.New("unsupported observation queue")
	default:
		return 0, errors.New("unsupported observation queue")
	}
	var depth int64
	err := s.DB.QueryRowContext(ctx, query).Scan(&depth)
	return depth, err
}

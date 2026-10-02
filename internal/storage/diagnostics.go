package storage

import (
	"context"
	"database/sql"
	"time"
)

type DeliveryDiagnostics struct {
	GeneratedAt         string           `json:"generated_at"`
	ActiveSubscriptions int64            `json:"active_subscriptions"`
	JournalPending      int64            `json:"journal_pending"`
	PendingPayloadBytes int64            `json:"pending_payload_bytes"`
	StateCounts         map[string]int64 `json:"state_counts"`
	ReasonCounts        map[string]int64 `json:"reason_counts"`
}

// DeliveryDiagnostics returns only bounded aggregate categories. SQL never
// selects message bodies, callbacks or credentials for this resource.
func (s *Store) DeliveryDiagnostics(ctx context.Context, at time.Time) (DeliveryDiagnostics, error) {
	d := DeliveryDiagnostics{GeneratedAt: at.UTC().Format(time.RFC3339Nano), StateCounts: map[string]int64{"pending": 0, "sending": 0, "delivered": 0, "failed": 0, "cancelled": 0, "other": 0}, ReasonCounts: map[string]int64{}}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return d, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM event_subscriptions WHERE active=1 AND (expires_at IS NULL OR julianday(expires_at)>julianday(?))", d.GeneratedAt).Scan(&d.ActiveSubscriptions); err != nil {
		return d, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM message_events WHERE seq>(SELECT seq FROM event_fanout WHERE id=1)").Scan(&d.JournalPending); err != nil {
		return d, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT coalesce(sum(length(payload)),0) FROM event_deliveries WHERE state IN ('pending','sending')").Scan(&d.PendingPayloadBytes); err != nil {
		return d, err
	}
	counts := func(query string, dest map[string]int64) error {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var category string
			var count int64
			if err := rows.Scan(&category, &count); err != nil {
				return err
			}
			dest[category] = count
		}
		return rows.Err()
	}
	if err = counts("SELECT CASE WHEN state IN ('pending','sending','delivered','failed','cancelled') THEN state ELSE 'other' END AS category,count(*) FROM event_deliveries GROUP BY category", d.StateCounts); err != nil {
		return d, err
	}
	if err = counts("SELECT CASE WHEN last_reason IN ('access_revoked','invalid_payload','deadline','queue_capacity','subscription_inactive','attempts','callback_gone','payload_too_large','http_rejected','http_transient','network','accepted','cancelled','record_deleted','retention') THEN last_reason ELSE 'other' END AS category,count(*) FROM event_deliveries WHERE last_reason IS NOT NULL GROUP BY category", d.ReasonCounts); err != nil {
		return d, err
	}
	return d, tx.Commit()
}

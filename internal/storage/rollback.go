package storage

import (
	"context"
	"database/sql"
)

// RollbackReport contains counts only, never subscription IDs or credentials.
// Compatibility targets the pre-rich-content (legacy/v2-only) runtime.
type RollbackReport struct {
	Compatible                       bool  `json:"compatible"`
	UnsupportedActiveSubscriptions   int64 `json:"unsupported_active_subscriptions"`
	UnsupportedUnprocessedDeliveries int64 `json:"unsupported_unprocessed_deliveries"`
	UnfannedActivitySources          int64 `json:"unfanned_activity_sources"`
	RetainedActivityPayloads         int64 `json:"retained_activity_payloads"`
}

// CheckLegacyRollback is read-only. Its result is valid for a stopped state only;
// a caller must hold the account lock through check and binary replacement.
func CheckLegacyRollback(ctx context.Context, db *sql.DB) (RollbackReport, error) {
	var report RollbackReport
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM event_subscriptions WHERE active=1 AND profile NOT IN('zalo.message.created','zalo.conversation.message.created.v2')`).
		Scan(&report.UnsupportedActiveSubscriptions); err != nil {
		return report, err
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM event_deliveries d JOIN event_subscriptions s ON s.id=d.subscription_id LEFT JOIN event_processing p ON p.subscription_id=d.subscription_id AND p.generation=d.generation WHERE s.profile NOT IN('zalo.message.created','zalo.conversation.message.created.v2') AND (d.state IN('pending','sending') OR d.id>COALESCE(p.ack_id,0))`).
		Scan(&report.UnsupportedUnprocessedDeliveries); err != nil {
		return report, err
	}
	var activities int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='activity_observations'`).
		Scan(&activities); err != nil {
		return report, err
	}
	if activities > 0 {
		// The previous runtime cannot apply activity retention, even after fanout
		// and acknowledgement. Refuse to strand retained source/outbox content.
		if err := db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM activity_observations WHERE length(payload_json)>0)+(SELECT count(*) FROM event_deliveries WHERE activity_seq IS NOT NULL AND length(payload)>0)`).
			Scan(&report.RetainedActivityPayloads); err != nil {
			return report, err
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM activity_observations WHERE expired=0 AND seq>(SELECT seq FROM activity_fanout WHERE id=1)`).
			Scan(&report.UnfannedActivitySources); err != nil {
			return report, err
		}
	}
	report.Compatible = report.UnsupportedActiveSubscriptions == 0 && report.UnsupportedUnprocessedDeliveries == 0 &&
		report.UnfannedActivitySources == 0 && report.RetainedActivityPayloads == 0
	return report, nil
}

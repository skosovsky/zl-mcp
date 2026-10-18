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
	ActiveRecoverySubscriptions      int64 `json:"active_recovery_subscriptions"`
	UndrainedRecoveryWork            int64 `json:"undrained_recovery_work"`
	RetainedRecoveryPayloads         int64 `json:"retained_recovery_payloads"`
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
	if err := recoveryRollbackCounts(ctx, db, &report); err != nil {
		return report, err
	}
	report.Compatible = report.UnsupportedActiveSubscriptions == 0 && report.UnsupportedUnprocessedDeliveries == 0 &&
		report.UnfannedActivitySources == 0 && report.RetainedActivityPayloads == 0 &&
		report.ActiveRecoverySubscriptions == 0 && report.UndrainedRecoveryWork == 0 && report.RetainedRecoveryPayloads == 0
	return report, nil
}

// recoveryRollbackCounts checks optional tables independently, including orphaned
// delivery rows. The legacy runtime cannot reconcile or retain these records.
func recoveryRollbackCounts(ctx context.Context, db *sql.DB, report *RollbackReport) error {
	active, err := optionalRollbackCount(
		ctx,
		db,
		"recovery_subscriptions",
		`SELECT count(*) FROM recovery_subscriptions WHERE active=1`,
	)
	if err != nil {
		return err
	}
	episodes, err := optionalRollbackCount(
		ctx,
		db,
		"recovery_episodes",
		`SELECT count(*) FROM recovery_episodes WHERE state IN ('armed','ready','exhausted')`,
	)
	if err != nil {
		return err
	}
	deliveries, err := optionalRollbackCount(
		ctx,
		db,
		"recovery_deliveries",
		`SELECT count(*) FROM recovery_deliveries WHERE state IN ('pending','sending')`,
	)
	if err != nil {
		return err
	}
	payloads, err := optionalRollbackCount(
		ctx,
		db,
		"recovery_deliveries",
		`SELECT count(*) FROM recovery_deliveries WHERE length(payload)>0`,
	)
	if err != nil {
		return err
	}
	report.ActiveRecoverySubscriptions = active
	report.UndrainedRecoveryWork = episodes + deliveries
	report.RetainedRecoveryPayloads = payloads
	return nil
}
func optionalRollbackCount(ctx context.Context, db *sql.DB, table, query string) (int64, error) {
	var present int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).
		Scan(&present); err != nil {
		return 0, err
	}
	if present == 0 {
		return 0, nil
	}
	var count int64
	err := db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

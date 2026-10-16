package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type pendingActivity struct {
	seq               int64
	id, body, created string
	expired           bool
}

// FanoutActivityEvents owns an independent sequence and watermark. It commits
// outbox rows and advancement atomically, without touching messages or novelty.
//
//nolint:funlen,gocognit,nestif // Keep validation, capacity accounting, outbox effects and watermark within one atomic ledger transaction.
func (s *Store) FanoutActivityEvents(
	ctx context.Context,
	at time.Time,
	p DeliveryPolicy,
	encode func(string, string, domain.Activity) ([]byte, error),
) (int, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the operation failure.
	rows, err := tx.QueryContext(
		ctx,
		`SELECT seq,event_id,payload_json,created_at,expired FROM activity_observations WHERE seq>(SELECT seq FROM activity_fanout WHERE id=1) ORDER BY seq LIMIT 32`,
	)
	if err != nil {
		return 0, err
	}
	batch := []pendingActivity{}
	for rows.Next() {
		var a pendingActivity
		if err = rows.Scan(&a.seq, &a.id, &a.body, &a.created, &a.expired); err != nil {
			//nolint:gosec,sqlclosecheck // Preserve the primary query error on immediate read-only cleanup.
			rows.Close()
			return 0, err
		}
		batch = append(batch, a)
	}
	err = rows.Err()
	//nolint:gosec // Preserve the primary query error on read-only cleanup.
	rows.Close()
	if err != nil {
		return 0, err
	}
	var jobs int
	var queueBytes int64
	if err = tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(length(payload)),0) FROM event_deliveries WHERE state IN ('pending','sending')`).
		Scan(&jobs, &queueBytes); err != nil {
		return 0, err
	}
	for _, record := range batch {
		if !record.expired {
			var activity domain.Activity
			if err = json.Unmarshal([]byte(record.body), &activity); err != nil {
				return 0, err
			}
			if err = activity.Validate(); err != nil {
				return 0, err
			}
			created, parseErr := time.Parse(time.RFC3339Nano, record.created)
			if parseErr != nil {
				return 0, parseErr
			}
			targets, targetErr := activityTargets(ctx, tx, activity, record.seq, at)
			if targetErr != nil {
				return 0, targetErr
			}
			for _, target := range targets {
				payload, encodingErr := encode(activity.Profile(), record.id, activity)
				state := statePendingText
				var reason, completed any
				switch {
				case !s.AllowsConversation(activity.Ref()):
					state, reason = stateCancelledText, "access_revoked"
				case encodingErr != nil:
					state, reason = stateFailedText, reasonInvalidPayload
				case !at.Before(created.Add(p.MaxAge)):
					state, reason = stateFailedText, "deadline"
				case jobs >= p.MaxJobs || queueBytes+int64(len(payload)) > p.MaxBytes:
					state, reason = stateFailedText, "queue_capacity"
				}
				if state != statePendingText {
					payload = []byte{}
					completed = at.UTC().Format(time.RFC3339Nano)
				}
				result, insertErr := tx.ExecContext(
					ctx,
					`INSERT OR IGNORE INTO event_deliveries(event_id,subscription_id,generation,payload,state,next_attempt_at,deadline,last_reason,completed_at,activity_seq) VALUES(?,?,?,?,?,?,?,?,?,?)`,
					record.id,
					target.id,
					target.generation,
					payload,
					state,
					at.UTC().Format(time.RFC3339Nano),
					created.Add(p.MaxAge).UTC().Format(time.RFC3339Nano),
					reason,
					completed,
					record.seq,
				)
				if insertErr != nil {
					return 0, insertErr
				}
				n, countErr := result.RowsAffected()
				if countErr != nil {
					return 0, countErr
				}
				if state == statePendingText && n > 0 {
					jobs++
					queueBytes += int64(len(payload))
				}
			}
		}
		if _, err = tx.ExecContext(ctx, `UPDATE activity_fanout SET seq=? WHERE id=1`, record.seq); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(batch), nil
}

type activityTarget struct{ id, generation string }

func activityTargets(
	ctx context.Context,
	tx *sql.Tx,
	a domain.Activity,
	seq int64,
	at time.Time,
) ([]activityTarget, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT id,generation FROM event_subscriptions WHERE active=1 AND profile=? AND (scope='all' OR scope=? OR(scope='conversation' AND conversation_type=? AND group_id=?)) AND(direction='all' OR direction=?) AND first_incoming_only=0 AND start_seq<? AND(expires_at IS NULL OR julianday(expires_at)>julianday(?)) ORDER BY id`,
		a.Profile(),
		a.ConversationType,
		a.ConversationType,
		a.ConversationID,
		a.Direction,
		seq,
		at.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []activityTarget{}
	for rows.Next() {
		var t activityTarget
		if err := rows.Scan(&t.id, &t.generation); err != nil {
			return nil, err
		}
		values = append(values, t)
	}
	return values, rows.Err()
}

// PruneActivities erases source/outbox content, retaining a permanent identity
// tombstone so replay after retention cannot create another event.
func (s *Store) PruneActivities(ctx context.Context, at time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the operation failure.
	// Earlier call suppression stored an empty JSON object. Canonicalize only
	// that exact expired, delivery-free marker; retained payloads stay intact.
	if _, err = tx.ExecContext(ctx, `UPDATE activity_observations SET payload_json=''
WHERE family='call' AND expired=1 AND payload_json='{}'
AND NOT EXISTS(SELECT 1 FROM event_deliveries d WHERE d.activity_seq=activity_observations.seq)`); err != nil {
		return err
	}
	if s.retention <= 0 {
		return tx.Commit()
	}
	cutoff := at.AddDate(0, 0, -s.retention).UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(
		ctx,
		`UPDATE event_deliveries SET payload=X'',state=CASE WHEN state IN ('pending','sending') THEN 'cancelled' ELSE state END,completed_at=COALESCE(completed_at,?),lease_until=NULL,last_reason='retention_expired' WHERE activity_seq IN(SELECT seq FROM activity_observations WHERE expired=0 AND julianday(created_at)<julianday(?))`,
		at.UTC().Format(time.RFC3339Nano),
		cutoff,
	)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(
		ctx,
		`UPDATE activity_observations SET payload_json='',expired=1 WHERE expired=0 AND julianday(created_at)<julianday(?)`,
		cutoff,
	); err != nil {
		return err
	}
	return tx.Commit()
}

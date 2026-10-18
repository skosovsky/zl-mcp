package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type recoveryWatch struct{ id, principal, business, generation, controlGeneration string }
type recoveryHead struct {
	id    int64
	ready bool
}

// SweepRecovery reconstructs watches from the durable business checkpoint on startup
// and every worker pass. Reading a journal is never required to arm recovery.
func (s *Store) SweepRecovery(ctx context.Context, at time.Time, p DeliveryPolicy) (int, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the original operation error.
	watches, err := loadRecoveryWatches(ctx, tx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, watch := range watches {
		n, e := s.sweepRecoveryWatch(ctx, tx, watch, at, p)
		if e != nil {
			return 0, e
		}
		count += n
	}
	if err = pruneRecovery(ctx, tx, at); err != nil {
		return 0, err
	}
	return count, tx.Commit()
}

func loadRecoveryWatches(ctx context.Context, tx *sql.Tx) ([]recoveryWatch, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT id,principal,business_id,business_generation,generation FROM recovery_subscriptions s WHERE active=1 OR EXISTS(SELECT 1 FROM recovery_episodes e WHERE e.subscription_id=s.id AND e.state IN ('armed','ready','exhausted')) OR EXISTS(SELECT 1 FROM recovery_deliveries d WHERE d.subscription_id=s.id AND d.state IN ('pending','sending'))`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	watches := []recoveryWatch{}
	for rows.Next() {
		var w recoveryWatch
		if err = rows.Scan(&w.id, &w.principal, &w.business, &w.generation, &w.controlGeneration); err != nil {
			return nil, err
		}
		watches = append(watches, w)
	}
	return watches, rows.Err()
}

func (s *Store) sweepRecoveryWatch(
	ctx context.Context,
	tx *sql.Tx,
	w recoveryWatch,
	at time.Time,
	p DeliveryPolicy,
) (int, error) {
	allowed, err := s.recoveryWatchAllowed(ctx, tx, w, at)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return 0, suppressRecovery(ctx, tx, w.id, "", recoveryStamp(at))
	}
	head, err := s.recoveryHead(ctx, tx, w.business, w.generation)
	if err != nil {
		return 0, err
	}
	if head.id == 0 {
		return 0, suppressRecovery(ctx, tx, w.id, "", recoveryStamp(at))
	}
	episode, err := ensureRecoveryEpisode(ctx, tx, w, head, at)
	if err != nil {
		return 0, err
	}
	if err = suppressRecovery(ctx, tx, w.id, episode, recoveryStamp(at)); err != nil {
		return 0, err
	}
	if !head.ready {
		return 0, nil
	}
	return enqueueRecoveryRound(ctx, tx, w, episode, at, p)
}

func (s *Store) recoveryWatchAllowed(ctx context.Context, tx *sql.Tx, w recoveryWatch, at time.Time) (bool, error) {
	var valid bool
	err := tx.QueryRowContext(ctx, `SELECT active=1 AND generation=? AND (expires_at IS NULL OR julianday(expires_at)>julianday(?)) FROM recovery_subscriptions WHERE id=?`, w.controlGeneration, recoveryStamp(at), w.id).
		Scan(&valid)
	if err != nil {
		return false, err
	}
	if !valid {
		return false, nil
	}
	err = s.checkRecoveryTarget(ctx, tx, w.principal, w.business, w.generation, at)
	if err != nil {
		if _, ok := errors.AsType[*domain.Error](err); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) recoveryHead(ctx context.Context, tx *sql.Tx, id, generation string) (recoveryHead, error) {
	var ack, pruned int64
	err := tx.QueryRowContext(ctx, `SELECT ack_id,pruned_id FROM event_processing WHERE subscription_id=? AND generation=?`, id, generation).
		Scan(&ack, &pruned)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return recoveryHead{}, err
	}
	if pruned > ack {
		return recoveryHead{id: -pruned, ready: true}, nil
	}
	var head recoveryHead
	var state string
	err = tx.QueryRowContext(ctx, `SELECT id,state FROM event_deliveries WHERE subscription_id=? AND generation=? AND id>? ORDER BY id LIMIT 1`, id, generation, ack).
		Scan(&head.id, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return recoveryHead{}, nil
	}
	if err != nil {
		return recoveryHead{}, err
	}
	// Broad subscriptions may expose revoked records only as receipt-bearing gaps.
	// Such terminal heads must wake the consumer so allowed later records remain reachable.
	// Exact subscription revocation is enforced by checkRecoveryTarget instead.
	head.ready = state != statePendingText && state != recoveryStateSending
	return head, nil
}

func ensureRecoveryEpisode(
	ctx context.Context,
	tx *sql.Tx,
	w recoveryWatch,
	head recoveryHead,
	at time.Time,
) (string, error) {
	id, err := eventRandomID()
	if err != nil {
		return "", err
	}
	state := recoveryStateArmed
	if head.ready {
		state = recoveryStateReady
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO recovery_episodes(id,subscription_id,generation,head,state,next_at,deadline) VALUES(?,?,?,?,?,?,?)`,
		id,
		w.id,
		w.controlGeneration,
		head.id,
		state,
		recoveryStamp(at),
		recoveryStamp(at.Add(recoveryLifetime)),
	)
	if err != nil {
		return "", err
	}
	err = tx.QueryRowContext(ctx, `SELECT id FROM recovery_episodes WHERE subscription_id=? AND generation=? AND head=?`, w.id, w.controlGeneration, head.id).
		Scan(&id)
	if err != nil {
		return "", err
	}
	if head.ready {
		_, err = tx.ExecContext(
			ctx,
			`UPDATE recovery_episodes SET state='ready',reason='awaiting_ack',next_at=?,deadline=? WHERE id=? AND state='armed'`,
			recoveryStamp(at),
			recoveryStamp(at.Add(recoveryLifetime)),
			id,
		)
	}
	return id, err
}

func enqueueRecoveryRound(
	ctx context.Context,
	tx *sql.Tx,
	w recoveryWatch,
	episode string,
	at time.Time,
	p DeliveryPolicy,
) (int, error) {
	var due bool
	var round int
	err := tx.QueryRowContext(ctx, `SELECT state='ready' AND julianday(next_at)<=julianday(?) AND julianday(deadline)>julianday(?) AND NOT EXISTS(SELECT 1 FROM recovery_deliveries WHERE episode_id=? AND state IN ('pending','sending')),round FROM recovery_episodes WHERE id=?`, recoveryStamp(at), recoveryStamp(at), episode, episode).
		Scan(&due, &round)
	if err != nil || !due {
		return 0, err
	}
	eventID, err := eventRandomID()
	if err != nil {
		return 0, err
	}
	eventID = "recovery_" + eventID
	payload, err := json.Marshal(
		map[string]any{
			"eventId":         eventID,
			"timestamp":       recoveryStamp(at),
			"cursor":          nil,
			fieldRecoveryName: recoveryProfile,
			"data": map[string]any{
				fieldRecoverySubscriptionID: w.business,
				"generation_reference":      w.generation,
				"recovery_reference":        episode,
			},
		},
	)
	if err != nil {
		return 0, err
	}
	var jobs int
	var size int64
	if err = tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(length(payload)),0) FROM recovery_deliveries `).
		Scan(&jobs, &size); err != nil {
		return 0, err
	}
	if jobs >= p.MaxJobs || size+int64(len(payload)) > p.MaxBytes {
		_, err = tx.ExecContext(ctx, `UPDATE recovery_episodes SET reason='capacity' WHERE id=?`, episode)
		return 0, err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO recovery_deliveries(event_id,episode_id,subscription_id,generation,payload,state,next_attempt_at,deadline) SELECT ?,?,?,?,?,'pending',?,deadline FROM recovery_episodes WHERE id=?`,
		eventID,
		episode,
		w.id,
		w.controlGeneration,
		payload,
		recoveryStamp(at),
		episode,
	)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(
		ctx,
		`UPDATE recovery_episodes SET round=?,reason='awaiting_ack' WHERE id=?`,
		round+1,
		episode,
	)
	return 1, err
}

func pruneRecovery(ctx context.Context, tx *sql.Tx, at time.Time) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE recovery_episodes SET state='exhausted',reason='episode_expired' WHERE state='ready' AND julianday(deadline)<=julianday(?)`,
		recoveryStamp(at),
	)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		`UPDATE recovery_deliveries SET state='failed',last_reason='deadline',payload=X'',lease_until=NULL,completed_at=? WHERE state IN ('pending','sending') AND julianday(deadline)<=julianday(?)`,
		recoveryStamp(at),
		recoveryStamp(at),
	)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		`DELETE FROM recovery_deliveries WHERE state NOT IN ('pending','sending') AND julianday(completed_at)<julianday(?)`,
		recoveryStamp(at.Add(-recoveryLifetime)),
	)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		`DELETE FROM recovery_episodes WHERE state='suppressed' AND julianday(deadline)<julianday(?) AND NOT EXISTS(SELECT 1 FROM recovery_deliveries d WHERE d.episode_id=recovery_episodes.id)`,
		recoveryStamp(at.Add(-recoveryLifetime)),
	)
	return err
}

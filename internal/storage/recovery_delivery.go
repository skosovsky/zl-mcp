package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ClaimRecoveryDelivery leases only the independent control outbox. An expired
// lease retries the identical round ID and bytes, never the business event.
func (s *Store) ClaimRecoveryDelivery(ctx context.Context, at time.Time, p DeliveryPolicy) (*RecoveryDelivery, error) {
	if _, err := s.SweepRecovery(ctx, at, p); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the original operation error.
	stamp := recoveryStamp(at)
	if _, err = tx.ExecContext(
		ctx,
		`UPDATE recovery_deliveries SET state='pending',lease_until=NULL WHERE state='sending' AND julianday(lease_until)<=julianday(?)`,
		stamp,
	); err != nil {
		return nil, err
	}
	if err = expireRecoveryAttempts(ctx, tx, at, p); err != nil {
		return nil, err
	}
	d, err := readRecoveryClaim(ctx, tx, at)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, tx.Commit()
	}
	d.Attempts++
	d.LeaseUntil = at.Add(p.Lease)
	_, err = tx.ExecContext(
		ctx,
		`UPDATE recovery_deliveries SET state='sending',attempts=?,lease_until=? WHERE id=? AND state='pending'`,
		d.Attempts,
		recoveryStamp(d.LeaseUntil),
		d.ID,
	)
	if err != nil {
		return nil, err
	}
	return d, tx.Commit()
}

func expireRecoveryAttempts(ctx context.Context, tx *sql.Tx, at time.Time, p DeliveryPolicy) error {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE recovery_episodes SET state='exhausted',reason='attempts' WHERE id IN (SELECT episode_id FROM recovery_deliveries WHERE state='pending' AND attempts>=?) AND state='ready'`,
		p.MaxAttempts,
	)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		`UPDATE recovery_deliveries SET state='failed',last_reason='attempts',completed_at=?,lease_until=NULL WHERE state='pending' AND attempts>=?`,
		recoveryStamp(at),
		p.MaxAttempts,
	)
	return err
}

func readRecoveryClaim(ctx context.Context, tx *sql.Tx, at time.Time) (*RecoveryDelivery, error) {
	var d RecoveryDelivery
	var deadline string
	err := tx.QueryRowContext(ctx, `SELECT d.id,d.event_id,d.subscription_id,d.generation,d.payload,d.attempts,d.deadline,s.callback,s.secret,s.principal,d.episode_id FROM recovery_deliveries d JOIN recovery_subscriptions s ON s.id=d.subscription_id JOIN recovery_episodes e ON e.id=d.episode_id WHERE d.state='pending' AND e.state='ready' AND julianday(d.next_attempt_at)<=julianday(?) AND NOT EXISTS(SELECT 1 FROM recovery_deliveries busy WHERE busy.subscription_id=d.subscription_id AND busy.state='sending') ORDER BY d.id LIMIT 1`, recoveryStamp(at)).
		Scan(&d.ID, &d.EventID, &d.SubscriptionID, &d.Generation, &d.Payload, &d.Attempts, &deadline, &d.Callback, &d.Secret, &d.Principal, &d.EpisodeID)
	if errors.Is(err, sql.ErrNoRows) {
		//nolint:nilnil // An empty control queue has no delivery and no failure.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.Deadline, err = time.Parse(time.RFC3339Nano, deadline)
	return &d, err
}

// RecoveryDispatchAllowed rechecks lease, attachment, ownership, policy, expiry and
// the exact unacknowledged head immediately before the HTTP call. A concurrent
// cancellation after this guard cannot recall an already dispatched request.
func (s *Store) RecoveryDispatchAllowed(ctx context.Context, d RecoveryDelivery) (bool, error) {
	return s.recoveryDispatchAllowedAt(ctx, d, time.Now().UTC())
}

func (s *Store) recoveryDispatchAllowedAt(ctx context.Context, d RecoveryDelivery, at time.Time) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the original operation error.
	valid, err := s.recoveryDispatchAllowed(ctx, tx, d, at)
	if err != nil {
		return false, err
	}
	return valid, tx.Commit()
}

func (s *Store) recoveryDispatchAllowed(
	ctx context.Context,
	tx *sql.Tx,
	d RecoveryDelivery,
	at time.Time,
) (bool, error) {
	var w recoveryWatch
	var head int64
	err := tx.QueryRowContext(ctx, `SELECT s.id,s.principal,s.business_id,s.business_generation,s.generation,e.head FROM recovery_deliveries d JOIN recovery_subscriptions s ON s.id=d.subscription_id JOIN recovery_episodes e ON e.id=d.episode_id WHERE d.id=? AND d.generation=? AND s.generation=d.generation AND d.state='sending' AND d.attempts=? AND d.lease_until=? AND julianday(d.lease_until)>julianday(?) AND e.state='ready' AND julianday(e.deadline)>julianday(?)`, d.ID, d.Generation, d.Attempts, recoveryStamp(d.LeaseUntil), recoveryStamp(at), recoveryStamp(at)).
		Scan(&w.id, &w.principal, &w.business, &w.generation, &w.controlGeneration, &head)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	allowed, err := s.recoveryWatchAllowed(ctx, tx, w, at)
	if err != nil || !allowed {
		return false, err
	}
	current, err := s.recoveryHead(ctx, tx, w.business, w.generation)
	return current.ready && current.id == head, err
}

func (s *Store) FinishRecoveryDelivery(
	ctx context.Context,
	d RecoveryDelivery,
	outcome DeliveryOutcome,
	at time.Time,
) error {
	if outcome.State != stateDeliveredText && outcome.State != statePendingText && outcome.State != stateFailedText {
		return errors.New("invalid recovery delivery outcome")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the original operation error.
	valid, err := s.recoveryDispatchAllowed(ctx, tx, d, at)
	if err != nil {
		return err
	}
	if !valid {
		return tx.Commit()
	}
	var completed any
	if outcome.State != statePendingText {
		completed = recoveryStamp(at)
	}
	retry := outcome.RetryAt
	if !retry.After(at) {
		retry = at.Add(recoveryMinimumInterval)
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE recovery_deliveries SET state=?,last_reason=?,next_attempt_at=?,completed_at=?,lease_until=NULL WHERE id=? AND state='sending' AND attempts=? AND lease_until=?`,
		outcome.State,
		safeDeliveryReason(outcome.Reason),
		recoveryStamp(retry),
		completed,
		d.ID,
		d.Attempts,
		recoveryStamp(d.LeaseUntil),
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated == 0 {
		return err
	}
	if err = advanceRecoveryRound(ctx, tx, d, outcome, at); err != nil {
		return err
	}
	return tx.Commit()
}

func advanceRecoveryRound(
	ctx context.Context,
	tx *sql.Tx,
	d RecoveryDelivery,
	outcome DeliveryOutcome,
	at time.Time,
) error {
	if outcome.Gone {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE recovery_subscriptions SET active=0 WHERE id=? AND generation=?`,
			d.SubscriptionID,
			d.Generation,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE event_subscription_revisions SET revision=revision+1 WHERE id=?`,
			d.SubscriptionID,
		); err != nil {
			return err
		}
		return suppressRecovery(ctx, tx, d.SubscriptionID, "", recoveryStamp(at))
	}
	if outcome.State == statePendingText {
		return nil
	}
	if outcome.State == stateFailedText {
		_, err := tx.ExecContext(
			ctx,
			`UPDATE recovery_episodes SET state='exhausted',reason=? WHERE id=?`,
			safeRecoveryReason(outcome.Reason),
			d.EpisodeID,
		)
		return err
	}
	var round int
	if err := tx.QueryRowContext(ctx, `SELECT round FROM recovery_episodes WHERE id=?`, d.EpisodeID).
		Scan(&round); err != nil {
		return err
	}
	interval := recoveryMinimumInterval
	for n := 1; n < round && interval < recoveryMaximumInterval; n++ {
		interval *= 2
	}
	if interval > recoveryMaximumInterval {
		interval = recoveryMaximumInterval
	}
	_, err := tx.ExecContext(
		ctx,
		`UPDATE recovery_episodes SET next_at=? WHERE id=?`,
		recoveryStamp(at.Add(interval)),
		d.EpisodeID,
	)
	return err
}

package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

const recoveryProfile = "zalo.subscription.journal.ready.v1"
const recoveryLifetime = 7 * 24 * time.Hour
const recoveryMinimumInterval = 30 * time.Second
const recoveryMaximumInterval = time.Hour
const maximumRecoverySubscriptions = 128

const (
	fieldRecoveryMode            = "mode"
	fieldRecoveryName            = "name"
	fieldRecoveryReference       = "reference"
	fieldRecoverySubscriptionID  = "subscription_id"
	recoveryStateArmed           = "armed"
	recoveryStateReady           = "ready"
	recoveryStateSending         = "sending"
	recoveryStateExhausted       = "exhausted"
	recoveryReasonNotConfigured  = "not_configured"
	recoveryReasonInactive       = "subscription_inactive"
	recoveryReasonEpisodeExpired = "episode_expired"
)

// RecoverySubscription attaches a verified control receiver to one owned business generation.
// It never changes business subscription boundaries or processing checkpoints.
type RecoverySubscription struct {
	ID, Principal, BusinessID, BusinessGeneration, Callback, Secret, Generation string
	ExpiresAt                                                                   *time.Time
}

type RecoveryDelivery struct {
	EventDelivery

	EpisodeID string
}

func (s *Store) migrateRecovery(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the original operation error.
	_, err = tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS recovery_subscriptions(id TEXT PRIMARY KEY,principal TEXT NOT NULL,business_id TEXT NOT NULL,business_generation TEXT NOT NULL,callback TEXT NOT NULL,secret TEXT NOT NULL,generation TEXT NOT NULL,active INTEGER NOT NULL,expires_at TEXT,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS recovery_episodes(id TEXT PRIMARY KEY,subscription_id TEXT NOT NULL,generation TEXT NOT NULL,head INTEGER NOT NULL,state TEXT NOT NULL,round INTEGER NOT NULL DEFAULT 0,next_at TEXT NOT NULL,deadline TEXT NOT NULL,reason TEXT NOT NULL DEFAULT '',UNIQUE(subscription_id,generation,head));
CREATE TABLE IF NOT EXISTS recovery_deliveries(id INTEGER PRIMARY KEY AUTOINCREMENT,event_id TEXT NOT NULL UNIQUE,episode_id TEXT NOT NULL,subscription_id TEXT NOT NULL,generation TEXT NOT NULL,payload BLOB NOT NULL,state TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next_attempt_at TEXT NOT NULL,deadline TEXT NOT NULL,lease_until TEXT,last_reason TEXT NOT NULL DEFAULT '',completed_at TEXT);
CREATE INDEX IF NOT EXISTS recovery_delivery_ready ON recovery_deliveries(state,next_attempt_at);
CREATE UNIQUE INDEX IF NOT EXISTS recovery_single_active ON recovery_subscriptions(business_id,business_generation) WHERE active=1;
CREATE INDEX IF NOT EXISTS recovery_episode_subscription ON recovery_episodes(subscription_id,generation,state);
INSERT OR IGNORE INTO schema_migrations VALUES(20);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CheckRecoveryTarget(ctx context.Context, principal, id, generation string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the original operation error.
	if err = s.checkRecoveryTarget(ctx, tx, principal, id, generation, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) checkRecoveryTarget(
	ctx context.Context,
	tx *sql.Tx,
	principal, id, generation string,
	at time.Time,
) error {
	var scope, kind, peer string
	err := tx.QueryRowContext(ctx, `SELECT scope,conversation_type,group_id FROM event_subscriptions WHERE id=? AND principal=? AND generation=? AND active=1 AND (expires_at IS NULL OR julianday(expires_at)>julianday(?))`, id, principal, generation, recoveryStamp(at)).
		Scan(&scope, &kind, &peer)
	if errors.Is(err, sql.ErrNoRows) {
		return subscriptionPermission("Recovery target is unavailable.")
	}
	if err != nil {
		return err
	}
	if scope == wireConversation && !s.AllowsConversation(domain.ConversationRef{Type: kind, ID: peer}) {
		return subscriptionPermission("Recovery target access was revoked.")
	}
	return nil
}

func (s *Store) ActivateRecoverySubscription(
	ctx context.Context,
	sub RecoverySubscription,
	revision int64,
	at time.Time,
) (RecoverySubscription, error) {
	if sub.ID == "" || sub.Principal == "" || sub.BusinessID == "" || sub.BusinessGeneration == "" ||
		sub.Callback == "" ||
		sub.Secret == "" {
		return sub, domain.Invalid("Recovery identity and delivery are required.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return sub, err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the original operation error.
	var businessCollision bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM event_subscriptions WHERE id=?)", sub.ID).
		Scan(&businessCollision); err != nil {
		return sub, err
	}
	if businessCollision {
		return sub, subscriptionPermission("Recovery ID collides with a business subscription.")
	}
	if err = s.checkRecoveryTarget(ctx, tx, sub.Principal, sub.BusinessID, sub.BusinessGeneration, at); err != nil {
		return sub, err
	}
	if err = checkRecoveryRevision(ctx, tx, sub.ID, revision); err != nil {
		return sub, err
	}
	if _, err = tx.ExecContext(
		ctx,
		`UPDATE recovery_subscriptions SET active=0 WHERE active=1 AND expires_at IS NOT NULL AND julianday(expires_at)<=julianday(?)`,
		recoveryStamp(at),
	); err != nil {
		return sub, err
	}
	var attached bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM recovery_subscriptions WHERE business_id=? AND business_generation=? AND active=1 AND id!=?)`, sub.BusinessID, sub.BusinessGeneration, sub.ID).
		Scan(&attached); err != nil {
		return sub, err
	}
	if attached {
		return sub, subscriptionPermission("Business generation already has an active recovery subscription.")
	}
	sub.Generation, err = recoveryGeneration(ctx, tx, sub, at)
	if err != nil {
		return sub, err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO recovery_subscriptions(id,principal,business_id,business_generation,callback,secret,generation,active,expires_at,created_at) VALUES(?,?,?,?,?,?,?,1,?,?) ON CONFLICT(id) DO UPDATE SET secret=excluded.secret,generation=excluded.generation,active=1,expires_at=excluded.expires_at`,
		sub.ID,
		sub.Principal,
		sub.BusinessID,
		sub.BusinessGeneration,
		sub.Callback,
		sub.Secret,
		sub.Generation,
		recoveryExpiry(sub.ExpiresAt),
		recoveryStamp(at),
	)
	if err != nil {
		return sub, err
	}
	return sub, tx.Commit()
}

func checkRecoveryRevision(ctx context.Context, tx *sql.Tx, id string, revision int64) error {
	var actual int64
	if err := tx.QueryRowContext(ctx, "SELECT revision FROM event_subscription_revisions WHERE id=?", id).
		Scan(&actual); err != nil {
		return err
	}
	if actual != revision {
		return ErrSubscriptionCancelled
	}
	return nil
}

func recoveryGeneration(ctx context.Context, tx *sql.Tx, sub RecoverySubscription, at time.Time) (string, error) {
	var owner, business, businessGen, callback, generation string
	var active bool
	var expiry sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT principal,business_id,business_generation,callback,generation,active,expires_at FROM recovery_subscriptions WHERE id=?`, sub.ID).
		Scan(&owner, &business, &businessGen, &callback, &generation, &active, &expiry)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err == nil {
		if owner != sub.Principal || business != sub.BusinessID || businessGen != sub.BusinessGeneration ||
			callback != sub.Callback {
			return "", subscriptionPermission("Recovery subscription belongs to another identity.")
		}
		if active && (!expiry.Valid || recoveryBefore(at, expiry.String)) {
			return generation, nil
		}
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM recovery_subscriptions WHERE active=1 AND (expires_at IS NULL OR julianday(expires_at)>julianday(?))`, recoveryStamp(at)).
		Scan(&count); err != nil {
		return "", err
	}
	if count >= maximumRecoverySubscriptions {
		return "", errors.New("recovery subscription capacity exceeded")
	}
	return eventRandomID()
}

func (s *Store) CancelRecoverySubscription(ctx context.Context, id, principal string, at time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the original operation error.
	var owner string
	err = tx.QueryRowContext(ctx, "SELECT principal FROM recovery_subscriptions WHERE id=?", id).Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && owner != principal {
		return subscriptionPermission("Recovery subscription belongs to another identity.")
	}
	if errors.Is(err, sql.ErrNoRows) {
		var business bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM event_subscriptions WHERE id=?)", id).
			Scan(&business); err != nil {
			return err
		}
		if business {
			return subscriptionPermission("Recovery ID belongs to a business subscription.")
		}
	}

	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO event_subscription_revisions(id,revision) VALUES(?,1) ON CONFLICT(id) DO UPDATE SET revision=revision+1`,
		id,
	); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE recovery_subscriptions SET active=0 WHERE id=?", id); err != nil {
		return err
	}
	if err = suppressRecovery(ctx, tx, id, "", recoveryStamp(at)); err != nil {
		return err
	}
	return tx.Commit()
}

func suppressRecovery(ctx context.Context, tx *sql.Tx, id, keep, stamp string) error {
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE recovery_episodes SET state='suppressed' WHERE subscription_id=? AND id!=? AND state IN ('armed','ready','exhausted')`,
		id,
		keep,
	); err != nil {
		return err
	}
	_, err := tx.ExecContext(
		ctx,
		`UPDATE recovery_deliveries SET state='cancelled',payload=X'',lease_until=NULL,last_reason='subscription_inactive',completed_at=? WHERE subscription_id=? AND episode_id!=? AND state IN ('pending','sending')`,
		stamp,
		id,
		keep,
	)
	return err
}

func recoveryStamp(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }
func recoveryBefore(at time.Time, stamp string) bool {
	other, err := time.Parse(time.RFC3339Nano, stamp)
	return err == nil && at.Before(other)
}
func recoveryExpiry(at *time.Time) any {
	if at == nil {
		return nil
	}
	return recoveryStamp(*at)
}

func checkBusinessSubscriptionRevision(ctx context.Context, tx *sql.Tx, id string, revision int64) error {
	var collision bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM recovery_subscriptions WHERE id=?)", id).
		Scan(&collision); err != nil {
		return err
	}
	if collision {
		return subscriptionPermission("Subscription ID belongs to a recovery subscription.")
	}
	return checkRecoveryRevision(ctx, tx, id, revision)
}

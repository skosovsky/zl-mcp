package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// RecoveryStatus exposes only control references and closed diagnostic categories.
// Callback addresses, credentials, business content and native IDs are excluded.
func (s *Store) RecoveryStatus(ctx context.Context, principal, businessID string) (map[string]any, error) {
	result := map[string]any{
		fieldRecoveryMode:           stateUnsupportedText,
		"state":                     stateUnsupportedText,
		fieldRecoveryReference:      nil,
		"reason":                    recoveryReasonNotConfigured,
		fieldRecoverySubscriptionID: nil,
	}
	var id, generation, businessGeneration string
	var active bool
	var expiry sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT r.id,r.generation,r.business_generation,r.active,r.expires_at FROM recovery_subscriptions r JOIN event_subscriptions b ON b.id=r.business_id AND b.generation=r.business_generation WHERE r.principal=? AND r.business_id=? ORDER BY r.active DESC,r.created_at DESC LIMIT 1`, principal, businessID).
		Scan(&id, &generation, &businessGeneration, &active, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	result[fieldRecoveryMode] = "companion"
	result[fieldRecoverySubscriptionID] = id
	result["reason"] = nil
	if !active || (expiry.Valid && !recoveryBefore(time.Now().UTC(), expiry.String)) {
		result["state"] = stateCancelledText
		result["reason"] = recoveryReasonInactive
		return result, nil
	}
	if err = s.CheckRecoveryTarget(ctx, principal, businessID, businessGeneration); err != nil {
		if _, ok := errors.AsType[*domain.Error](err); !ok {
			return nil, err
		}
		result["state"] = stateCancelledText
		result["reason"] = recoveryReasonInactive
		return result, nil
	}
	var reference, state, reason string
	var waiting bool
	err = s.DB.QueryRowContext(ctx, `SELECT e.id,e.state,e.reason,EXISTS(SELECT 1 FROM recovery_deliveries d WHERE d.episode_id=e.id AND d.state IN ('pending','sending')) FROM recovery_episodes e WHERE e.subscription_id=? AND e.generation=? ORDER BY rowid DESC LIMIT 1`, id, generation).
		Scan(&reference, &state, &reason, &waiting)
	if errors.Is(err, sql.ErrNoRows) {
		result["state"] = "idle"
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	result[fieldRecoveryReference] = reference
	switch state {
	case recoveryStateArmed:
		result["state"] = recoveryStateArmed
		result["reason"] = "pending_delivery"
	case recoveryStateReady:
		result["state"] = recoveryStateReady
		if reason != "" {
			result["reason"] = safeRecoveryReason(reason)
		}
		if waiting {
			result["state"] = "waiting"
		}
	case recoveryStateExhausted:
		result["state"] = recoveryStateExhausted
		result["reason"] = recoveryReasonEpisodeExpired
		if reason != "" {
			result["reason"] = safeRecoveryReason(reason)
		}
	default:
		result["state"] = "completed"
	}
	return result, nil
}

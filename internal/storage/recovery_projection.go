package storage

import (
	"context"
	"errors"
)

type recoveryStatusReader interface {
	RecoveryStatus(context.Context, string, string) (map[string]any, error)
}

// recoveryMetadata contains no callback credentials, business payload or native
// IDs. The interface fallback permits isolated readers without the extension.
func (s *Store) recoveryMetadata(ctx context.Context, principal, id string) (map[string]any, error) {
	status := map[string]any{
		fieldRecoveryMode: stateUnsupportedText, "state": stateUnsupportedText, fieldRecoveryReference: nil,
		"reason": recoveryReasonNotConfigured, fieldRecoverySubscriptionID: nil,
	}
	if reader, ok := any(s).(recoveryStatusReader); ok {
		value, err := reader.RecoveryStatus(ctx, principal, id)
		if err != nil {
			return nil, err
		}
		status = value
	}
	mode, _ := status[fieldRecoveryMode].(string)
	state, _ := status["state"].(string)
	if mode != stateUnsupportedText && mode != "companion" {
		return nil, errors.New("invalid recovery mode")
	}
	switch state {
	case stateUnsupportedText,
		"idle",
		recoveryStateArmed,
		recoveryStateReady,
		"waiting",
		"completed",
		"cancelled",
		recoveryStateExhausted:
	default:
		return nil, errors.New("invalid recovery state")
	}
	for _, key := range []string{fieldRecoveryReference, fieldRecoverySubscriptionID} {
		if status[key] != nil {
			value, ok := status[key].(string)
			if !ok || len(value) > 256 {
				return nil, errors.New("invalid recovery reference")
			}
		}
	}
	if reason, ok := status["reason"].(string); ok {
		status["reason"] = safeRecoveryReason(reason)
	}
	return status, nil
}

func safeRecoveryReason(reason string) string {
	switch reason {
	case recoveryReasonNotConfigured, deliveryReasonAccepted, "pending_delivery", "awaiting_ack", "callback_gone",
		"payload_too_large", "http_rejected", "attempts", deliveryReasonDeadline, recoveryReasonEpisodeExpired,
		deliveryReasonRevoked, recoveryReasonInactive, "generation_changed", "head_changed",
		"acknowledged", "capacity":
		return reason
	default:
		return stateUnknownText
	}
}

func (s *Store) addRecoverySubscriptionDetails(ctx context.Context, principal string, entries []map[string]any) error {
	for _, entry := range entries {
		id, ok := entry[fieldRecoverySubscriptionID].(string)
		if !ok {
			return errors.New("invalid recovery subscription projection")
		}
		status, err := s.recoveryMetadata(ctx, principal, id)
		if err != nil {
			return err
		}
		entry["recovery_mode"] = status[fieldRecoveryMode]
		entry["recovery_subscription_id"] = status[fieldRecoverySubscriptionID]
	}
	return nil
}

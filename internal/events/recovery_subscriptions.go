package events

import (
	"context"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/storage"
)

// RecoverySubscriptions is optional: a store without the durable control queue
// explicitly declines the companion instead of implying eventual continuation.
type RecoverySubscriptions interface {
	CheckRecoveryTarget(context.Context, string, string, string) error
	SubscriptionRevision(context.Context, string) (int64, error)
	ActivateRecoverySubscription(
		context.Context,
		storage.RecoverySubscription,
		int64,
		time.Time,
	) (storage.RecoverySubscription, error)
	CancelRecoverySubscription(context.Context, string, string, time.Time) error
}

func (m *SubscriptionManager) changeRecoverySubscription(
	ctx context.Context,
	method, principal string,
	p subscriptionRequest,
) (any, error) {
	store, ok := m.Store.(RecoverySubscriptions)
	if !ok {
		return nil, eventRPCError(-32000, "Journal continuation is unsupported by this server.", "recovery_unsupported")
	}
	id := m.subscriptionID(principal, p)
	if method == methodEventsUnsubscribe {
		if err := store.CancelRecoverySubscription(ctx, id, principal, time.Now().UTC()); err != nil {
			return nil, eventRPCError(-32603, "Cannot cancel journal continuation.", "storage")
		}
		return map[string]any{}, nil
	}
	if err := store.CheckRecoveryTarget(
		ctx,
		principal,
		p.Arguments.SubscriptionID,
		p.Arguments.GenerationReference,
	); err != nil {
		return nil, eventRPCError(
			-32000,
			"Active owned subscription generation is required.",
			"recovery_target_unavailable",
		)
	}
	if _, err := m.Callbacks.validate(p.Delivery.URL); err != nil {
		return nil, eventRPCError(-32602, "Invalid callback URL.", "invalid_callback")
	}
	if _, err := DecodeSigningKey(p.Delivery.Secret); err != nil {
		return nil, eventRPCError(-32602, "Invalid signing key.", "invalid_secret")
	}
	revision, err := store.SubscriptionRevision(ctx, id)
	if err != nil {
		return nil, eventRPCError(-32603, "Cannot prepare journal continuation.", "storage")
	}
	if verifyErr := m.verifySubscriptionCallback(ctx, p, id); verifyErr != nil {
		return nil, verifyErr
	}
	at := time.Now().UTC()
	//nolint:exhaustruct_v5 // Storage assigns a new generation; expiration is optional.
	sub := storage.RecoverySubscription{
		ID:                 id,
		Principal:          principal,
		BusinessID:         p.Arguments.SubscriptionID,
		BusinessGeneration: p.Arguments.GenerationReference,
		Callback:           p.Delivery.URL,
		Secret:             p.Delivery.Secret,
	}
	if p.TTL != nil {
		expires := at.Add(time.Duration(*p.TTL) * time.Millisecond)
		sub.ExpiresAt = &expires
	}
	sub, err = store.ActivateRecoverySubscription(ctx, sub, revision, at)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionCancelled) {
			return nil, eventRPCError(-32000, "Journal continuation cancelled during verification.", "cancelled")
		}
		return nil, eventRPCError(-32603, "Cannot activate journal continuation.", "storage")
	}
	return map[string]any{"id": sub.ID, "refreshBefore": sub.ExpiresAt, fieldCursor: nil, "truncated": false}, nil
}

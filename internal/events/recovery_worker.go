package events

import (
	"context"
	"time"

	"github.com/skosovsky/zl-mcp/internal/storage"
)

// RecoveryOutbox is deliberately independent from message fanout and its journal.
// Implementations suppress stale generations and already acknowledged heads.
type RecoveryOutbox interface {
	SweepRecovery(context.Context, time.Time, storage.DeliveryPolicy) (int, error)
	ClaimRecoveryDelivery(context.Context, time.Time, storage.DeliveryPolicy) (*storage.RecoveryDelivery, error)
	RecoveryDispatchAllowed(context.Context, storage.RecoveryDelivery) (bool, error)
	FinishRecoveryDelivery(context.Context, storage.RecoveryDelivery, storage.DeliveryOutcome, time.Time) error
}

func (w *Worker) sweepRecovery(ctx context.Context, at time.Time) error {
	if queue, ok := w.Queue.(RecoveryOutbox); ok {
		_, err := queue.SweepRecovery(ctx, at, w.Policy)
		return err
	}
	return nil
}

// DeliverRecoveryOne dispatches only control traffic; it never inserts or
// acknowledges a business journal record and never counts as message delivery.
func (w *Worker) DeliverRecoveryOne(ctx context.Context, at time.Time) (bool, error) {
	queue, ok := w.Queue.(RecoveryOutbox)
	if !ok {
		return false, nil
	}
	d, err := queue.ClaimRecoveryDelivery(ctx, at, w.Policy)
	if err != nil || d == nil {
		return false, err
	}
	allowed, err := queue.RecoveryDispatchAllowed(ctx, *d)
	if err != nil || !allowed {
		return false, err
	}
	return w.deliverClaimedWithFinish(
		ctx,
		at,
		&d.EventDelivery,
		func(finishCtx context.Context, _ storage.EventDelivery, outcome storage.DeliveryOutcome, finished time.Time) error {
			return queue.FinishRecoveryDelivery(finishCtx, *d, outcome, finished)
		},
		"journal_recovery",
		d.EpisodeID,
	)
}

func (w *Worker) deliverBusinessAndRecovery(ctx context.Context, at time.Time) (bool, error) {
	business, err := w.DeliverOne(ctx, at)
	if err != nil {
		return business, err
	}
	recovery, err := w.DeliverRecoveryOne(ctx, at)
	return business || recovery, err
}

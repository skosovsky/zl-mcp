package store

import (
	"context"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
)

var _ eventaccess.DiagnosticsPort = (*Reader)(nil)

func (r *Reader) DeliveryDiagnostics(ctx context.Context, at time.Time) (eventaccess.DeliveryDiagnostics, error) {
	native, err := r.owner.DeliveryDiagnostics(ctx, at)
	if err != nil {
		return eventaccess.DeliveryDiagnostics{}, err
	}
	states := native.StateCounts
	return eventaccess.DeliveryDiagnostics{
		GeneratedAt: native.GeneratedAt, ActiveSubscriptions: native.ActiveSubscriptions,
		JournalPending: native.JournalPending, PendingPayloadBytes: native.PendingPayloadBytes,
		StateCounts:  eventaccess.DeliveryStates{Pending: states["pending"], Sending: states["sending"], Delivered: states["delivered"], Failed: states["failed"], Cancelled: states["cancelled"], Other: states["other"]},
		ReasonCounts: native.ReasonCounts,
	}, nil
}

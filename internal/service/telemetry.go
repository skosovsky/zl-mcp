package service

import (
	"context"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func sampleNativeQueues(ctx context.Context, owner shared.NativeObservationPort) {
	for _, kind := range []domain.WorkKind{domain.WorkSend, domain.WorkJoin, domain.WorkHistory, domain.WorkEvents} {
		shared.ObserveQueue(ctx, kind, owner)
	}
}

// observeNativeQueues belongs to the service lifecycle, never a command path.
func observeNativeQueues(ctx context.Context, owner shared.NativeObservationPort) error {
	ticker := time.NewTicker(telemetryQueueInterval)
	defer ticker.Stop()
	for {
		sampleNativeQueues(ctx, owner)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

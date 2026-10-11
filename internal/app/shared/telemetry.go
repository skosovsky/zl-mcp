package shared

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"time"
)

type Invocation func(context.Context) (domain.Outcome, error)

// TelemetryPort contains bounded business observations, never SDK DTOs/content.
// Native operation identities only index an ephemeral trace-reference cache.
type TelemetryPort interface {
	Recording() bool
	Execute(context.Context, string, Invocation) error
	ResponseBytes(context.Context, string, domain.Transport, int64)
	Upstream(context.Context, domain.WorkKind, func(context.Context) error) error
	Transition(context.Context, domain.WorkKind, domain.OperationState, domain.OperationState, *time.Duration)
	Queue(context.Context, domain.WorkKind, int64)
	Remember(context.Context, domain.WorkKind, string)
	Worker(context.Context, domain.WorkKind, string, func(context.Context) error) error
}

type telemetryContextKey struct{}

func WithTelemetry(ctx context.Context, observer TelemetryPort) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, telemetryContextKey{}, observer)
}
func Telemetry(ctx context.Context) TelemetryPort {
	observer, _ := ctx.Value(telemetryContextKey{}).(TelemetryPort)
	return observer
}

// Recording avoids observation-only reads/allocations when export is disabled.
func Recording(ctx context.Context) bool { o := Telemetry(ctx); return o != nil && o.Recording() }
func ObserveUpstream[T any](ctx context.Context, kind domain.WorkKind, next func(context.Context) (T, error)) (T, error) {
	o := Telemetry(ctx)
	if o == nil || !o.Recording() {
		return next(ctx)
	}
	var value T
	err := o.Upstream(ctx, kind, func(ctx context.Context) error { var err error; value, err = next(ctx); return err })
	return value, err
}
func ObserveWorker(ctx context.Context, kind domain.WorkKind, id string, next func(context.Context) error) error {
	o := Telemetry(ctx)
	if o == nil || !o.Recording() {
		return next(ctx)
	}
	return o.Worker(ctx, kind, id, next)
}

// NativeObservationPort only borrows existing state; it performs no migration.
type NativeObservationPort interface {
	ObservationQueueDepth(context.Context, domain.WorkKind) (int64, error)
}

func ObserveQueue(ctx context.Context, kind domain.WorkKind, owner NativeObservationPort) {
	o := Telemetry(ctx)
	if o == nil || !o.Recording() {
		return
	}
	read, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if depth, err := owner.ObservationQueueDepth(read, kind); err == nil {
		o.Queue(ctx, kind, depth)
	}
}
func ObserveTransition(ctx context.Context, kind domain.WorkKind, from, to string, created time.Time) {
	o := Telemetry(ctx)
	if o == nil || !o.Recording() {
		return
	}
	var age *time.Duration
	if !created.IsZero() {
		value := time.Since(created)
		age = &value
	}
	o.Transition(ctx, kind, domain.ObserveState(from), domain.ObserveState(to), age)
}

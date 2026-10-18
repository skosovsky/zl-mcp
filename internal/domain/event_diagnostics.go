package domain

import "context"

type eventDiagnosticsKey struct{}

// WithEventDiagnostics selects an opt-in read representation, never authority.
func WithEventDiagnostics(ctx context.Context) context.Context {
	return context.WithValue(ctx, eventDiagnosticsKey{}, true)
}

func EventDiagnostics(ctx context.Context) bool {
	enabled, _ := ctx.Value(eventDiagnosticsKey{}).(bool)
	return enabled
}

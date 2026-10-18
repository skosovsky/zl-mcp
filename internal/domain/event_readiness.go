package domain

import "context"

const (
	EventReadinessDeliveryTerminal = "delivery_terminal"
	EventReadinessPersisted        = "persisted"
)

type persistedEventReadinessKey struct{}

// WithPersistedEventReadiness selects a journal view, never authority or a new
// processing cursor. HTTP delivery and processing acknowledgement stay separate.
func WithPersistedEventReadiness(ctx context.Context) context.Context {
	return context.WithValue(ctx, persistedEventReadinessKey{}, true)
}

func PersistedEventReadiness(ctx context.Context) bool {
	enabled, _ := ctx.Value(persistedEventReadinessKey{}).(bool)
	return enabled
}

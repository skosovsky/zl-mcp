package domain

import "context"

type recoveryDetailsKey struct{}

// WithRecoveryDetails selects extra subscription metadata, never authorization.
func WithRecoveryDetails(ctx context.Context) context.Context {
	return context.WithValue(ctx, recoveryDetailsKey{}, true)
}

func RecoveryDetails(ctx context.Context) bool {
	enabled, _ := ctx.Value(recoveryDetailsKey{}).(bool)
	return enabled
}

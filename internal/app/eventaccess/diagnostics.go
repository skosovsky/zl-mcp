// Package eventaccess exposes read capabilities around the native Events journal.
package eventaccess

import (
	"context"
	"time"
)

type DeliveryStates struct {
	Pending   int64 `json:"pending"`
	Sending   int64 `json:"sending"`
	Delivered int64 `json:"delivered"`
	Failed    int64 `json:"failed"`
	Cancelled int64 `json:"cancelled"`
	Other     int64 `json:"other"`
}
type DeliveryDiagnostics struct {
	GeneratedAt         string         `json:"generated_at"`
	ActiveSubscriptions int64          `json:"active_subscriptions"`
	JournalPending      int64          `json:"journal_pending"`
	PendingPayloadBytes int64          `json:"pending_payload_bytes"`
	StateCounts         DeliveryStates `json:"state_counts"`
	// Native bounded reason categories and numeric counts, never arbitrary payloads.
	ReasonCounts map[string]int64 `json:"reason_counts"`
}
type DiagnosticsPort interface {
	DeliveryDiagnostics(context.Context, time.Time) (DeliveryDiagnostics, error)
}
type Handler struct {
	Commands    AcknowledgementPort
	Diagnostics DiagnosticsPort
	Journal     JournalPort
}

func (h *Handler) DeliveryDiagnostics(ctx context.Context, at time.Time) (DeliveryDiagnostics, error) {
	return h.Diagnostics.DeliveryDiagnostics(ctx, at)
}

package toolruntime

import (
	"context"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// Outcome projects typed business states, independently of the Go call error.
// It reads no reason/message/error text and never turns a result into a Go error.
func (r *Result) Outcome() domain.Outcome {
	if r == nil {
		return domain.OutcomeUnknown
	}
	switch {
	case r.SendStatus != nil:
		return stateOutcome(domain.ObserveState(r.SendStatus.Status))
	case r.ImportStatus != nil:
		return stateOutcome(domain.ObserveState(r.ImportStatus.State))
	case r.JoinStatus != nil:
		if r.JoinStatus.Error != nil {
			return domain.OutcomeError
		}
		return stateOutcome(domain.ObserveState(r.JoinStatus.Status))
	default:
		return domain.OutcomeSuccess
	}
}
func stateOutcome(state domain.OperationState) domain.Outcome {
	switch state {
	case domain.StateSent, domain.StateCompleted, domain.StateJoined, domain.StateAlreadyMember, domain.StateDelivered:
		return domain.OutcomeSuccess
	case domain.StateFailed, domain.StateUnsupported:
		return domain.OutcomeError
	case domain.StatePartial, domain.StatePaused:
		return domain.OutcomePartial
	case domain.StateCancelled:
		return domain.OutcomeCancelled
	default:
		return domain.OutcomeUnknown
	}
}
func (r *Result) remember(ctx context.Context, name string, observer shared.TelemetryPort) {
	if r == nil || (name != "zalo_send_direct_message" && name != "zalo_join_group" && name != "zalo_import_conversation_history") {
		return
	}
	switch {
	case r.SendStatus != nil:
		observer.Remember(ctx, domain.WorkSend, r.SendStatus.RequestID)
	case r.ImportStatus != nil:
		observer.Remember(ctx, domain.WorkHistory, r.ImportStatus.OperationID)
	case r.JoinStatus != nil:
		observer.Remember(ctx, domain.WorkJoin, r.JoinStatus.OperationID)
	}
}

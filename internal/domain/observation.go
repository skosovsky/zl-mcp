package domain

// Outcome is a finite business result category, independent of Go error status.
type Outcome uint8

const (
	OutcomeSuccess Outcome = iota
	OutcomeError
	OutcomePartial
	OutcomeUnknown
	OutcomeCancelled
	OutcomePanic
)

func (o Outcome) String() string {
	switch o {
	case OutcomeSuccess:
		return "success"
	case OutcomeError:
		return "error"
	case OutcomePartial:
		return "partial"
	case OutcomeCancelled:
		return activityCancelled
	case OutcomePanic:
		return "panic"
	case OutcomeUnknown:
		return stateUnknownText
	default:
		return stateUnknownText
	}
}

type WorkKind uint8

const (
	WorkSend WorkKind = iota
	WorkJoin
	WorkHistory
	WorkEvents
	WorkCollector
	WorkOther
)

func (k WorkKind) String() string {
	switch k {
	case WorkSend:
		return "send"
	case WorkJoin:
		return "join"
	case WorkHistory:
		return "history"
	case WorkEvents:
		return "events"
	case WorkCollector:
		return "collector"
	case WorkOther:
		return workOtherText
	default:
		return workOtherText
	}
}

type OperationState uint8

const (
	StateUnknown OperationState = iota
	StatePending
	StateSending
	StateSent
	StateFailed
	StateQueued
	StateRunning
	StatePaused
	StateCompleted
	StatePartial
	StateCancelled
	StateUnsupported
	StateJoined
	StatePendingApproval
	StateAlreadyMember
	StateDelivered
)

func (s OperationState) String() string {
	switch s {
	case StatePending:
		return "pending"
	case StateSending:
		return "sending"
	case StateSent:
		return "sent"
	case StateFailed:
		return "failed"
	case StateQueued:
		return "queued"
	case StateRunning:
		return "running"
	case StatePaused:
		return "paused"
	case StateCompleted:
		return "completed"
	case StatePartial:
		return "partial"
	case StateCancelled:
		return activityCancelled
	case StateUnsupported:
		return "unsupported"
	case StateJoined:
		return "joined"
	case StatePendingApproval:
		return "pending_approval"
	case StateAlreadyMember:
		return "already_member"
	case StateDelivered:
		return "delivered"
	case StateUnknown:
		return stateUnknownText
	default:
		return stateUnknownText
	}
}
func ObserveState(state string) OperationState {
	for candidate := StateUnknown; candidate <= StateDelivered; candidate++ {
		if candidate.String() == state {
			return candidate
		}
	}
	return StateUnknown
}

type Transport uint8

const (
	TransportHTTP Transport = iota
	TransportSTDIO
	TransportResource
)

func (t Transport) String() string {
	switch t {
	case TransportHTTP:
		return "http"
	case TransportSTDIO:
		return "stdio"
	case TransportResource:
		return "resource"
	default:
		return workOtherText
	}
}

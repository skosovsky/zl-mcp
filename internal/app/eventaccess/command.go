package eventaccess

import "context"

type Acknowledgement struct {
	Acknowledged bool `json:"acknowledged"`
}
type AcknowledgementPort interface {
	Acknowledge(context.Context, string, string, string) (Acknowledgement, error)
}

func (h *Handler) Acknowledge(ctx context.Context, principal, id, receipt string) (Acknowledgement, error) {
	return h.Commands.Acknowledge(ctx, principal, id, receipt)
}

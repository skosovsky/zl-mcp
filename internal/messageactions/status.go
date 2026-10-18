package messageactions

import (
	"context"
	"database/sql"
	"errors"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// Status never dispatches, requires no action grant and works without a source.
func (m *Manager) Status(ctx context.Context, id string) (domain.MessageActionOperation, error) {
	result, err := m.Store.AccessibleMessageActionStatus(ctx, id)
	return result, PublicError(err)
}

// PublicError preserves deliberate domain errors and hides local storage details.
func PublicError(err error) error {
	if err == nil {
		return nil
	}
	if domainError, ok := errors.AsType[*domain.Error](err); ok {
		return domainError
	}
	switch {
	case errors.Is(err, storage.ErrActionConflict):
		return actionError("REQUEST_CONFLICT", "This request UUID is already bound to different action arguments.")
	case errors.Is(err, storage.ErrActionCapacity):
		return actionError("CAPACITY_EXCEEDED", "The durable message action ledger is full; no action was dispatched.")
	case errors.Is(err, sql.ErrNoRows):
		return actionError("NOT_FOUND", "No accessible message action exists for this request UUID.")
	case errors.Is(err, domain.ErrAuthenticationRequired):
		return actionError(
			"NOT_AUTHENTICATED",
			"The collector has no active authenticated session; inspect the existing request UUID before retrying.",
		)
	default:
		return actionError(
			"STORAGE_ERROR",
			"Message action state is unavailable. Inspect the same request UUID; do not create another request after a possible dispatch.",
		)
	}
}

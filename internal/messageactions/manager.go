// Package messageactions owns explicit receipt/reaction writes and their ledger.
package messageactions

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

const actionReadKind = "read"
const upstreamActionTimeout = 30 * time.Second
const saveActionTimeout = 3 * time.Second

type Upstream interface {
	MarkSeen(context.Context, []domain.MessageActionTarget) (domain.ReadReceiptResult, error)
	React(context.Context, domain.MessageActionTarget, string, bool) error
}

type Manager struct {
	Store         *storage.Store
	Source        Upstream
	ReadChats     map[domain.ConversationRef]bool
	ReactionChats map[domain.ConversationRef]bool
}

func actionError(code, message string) error {
	//nolint:exhaustruct_v5 // Optional continuation fields stay absent; raw upstream details are never attached.
	return &domain.Error{Code: code, Message: message, Details: map[string]any{}}
}

func (m *Manager) allowed(intent domain.MessageActionIntent) bool {
	if !m.Store.AllowsConversation(intent.Conversation) {
		return false
	}
	if intent.Kind == actionReadKind {
		return m.ReadChats[intent.Conversation]
	}
	return m.ReactionChats[intent.Conversation]
}

func (m *Manager) Execute(
	ctx context.Context,
	intent domain.MessageActionIntent,
) (domain.MessageActionOperation, error) {
	var zero domain.MessageActionOperation
	if err := intent.Validate(); err != nil {
		return zero, err
	}
	if !m.allowed(intent) {
		return zero, actionError("PERMISSION_DENIED", "This action is not enabled for the exact conversation.")
	}
	op, err := m.Store.PrepareMessageAction(
		ctx,
		intent.RequestID,
		intent.Kind,
		intent.Plan,
		intent.Conversation,
		intent.Fingerprint(),
	)
	if err != nil {
		return zero, err
	}
	if op.Status != "pending" {
		return op, nil
	}
	// Validate all targets before acquiring a dispatch claim or calling upstream.
	targets := make([]domain.MessageActionTarget, len(intent.MessageIDs))
	for i, id := range intent.MessageIDs {
		targets[i], err = m.Store.MessageActionTarget(ctx, intent.Conversation, id, intent.Kind == actionReadKind)
		if errors.Is(err, sql.ErrNoRows) {
			return op, actionError(
				"ACTION_METADATA_UNAVAILABLE",
				"The exact retained message lacks verified action metadata.",
			)
		}
		if err != nil {
			return op, err
		}
		if !domain.ValidActionMessageID(targets[i].MessageID) ||
			!domain.ValidActionMessageID(targets[i].ClientMessageID) {
			return op, actionError(
				"ACTION_METADATA_UNAVAILABLE",
				"The exact retained message lacks supported native identifiers.",
			)
		}
	}
	if m.Source == nil {
		return op, domain.ErrAuthenticationRequired
	}
	claimed, err := m.Store.ClaimMessageAction(ctx, intent.RequestID)
	if err != nil {
		return op, err
	}
	if !claimed {
		return m.Store.MessageActionStatus(ctx, intent.RequestID)
	}
	// Recheck at the irreversible boundary. No daemon worker consumes pending
	// intents; a later write always requires another explicit tool invocation.
	if !m.allowed(intent) {
		return m.finish(ctx, intent.RequestID, "rejected", "permission_denied")
	}
	return m.dispatch(ctx, intent)
}

func (m *Manager) dispatch(
	ctx context.Context,
	intent domain.MessageActionIntent,
) (domain.MessageActionOperation, error) {
	stages := []string{intent.Plan}
	if intent.Plan == "clear_add" {
		stages = []string{"clear", "add"}
	}
	for completed, stage := range stages {
		targets, reason := m.refreshTargets(ctx, intent)
		if reason != "" {
			return m.finish(ctx, intent.RequestID, knownActionFailureState(completed), reason)
		}
		if err := m.Store.BeginMessageActionStep(ctx, intent.RequestID, stage, completed); err != nil {
			return m.finish(ctx, intent.RequestID, knownActionFailureState(completed), "storage_unavailable")
		}
		upstreamErr, storageErr := m.invokeActionStep(ctx, intent, stage, targets)
		if storageErr != nil {
			return m.finish(ctx, intent.RequestID, "unknown", "storage_unavailable")
		}
		if upstreamErr != nil {
			state, reason := classifyActionFailure(upstreamErr, completed)
			return m.finish(ctx, intent.RequestID, state, reason)
		}
		save, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveActionTimeout)
		err := m.Store.ConfirmMessageActionStep(save, intent.RequestID, stage, completed)
		cancel()
		if err != nil {
			return m.finish(ctx, intent.RequestID, "unknown", "storage_unavailable")
		}
	}
	return m.finish(ctx, intent.RequestID, "accepted", "")
}

func knownActionFailureState(completed int) string {
	if completed > 0 {
		return "partial"
	}
	return "rejected"
}

func classifyActionFailure(err error, completed int) (string, string) {
	if errors.Is(err, domain.ErrUnverifiedReadStatus) {
		return "unknown", "upstream_unverified"
	}
	if errors.Is(err, domain.ErrMessageActionRejected) {
		return knownActionFailureState(completed), "upstream_rejected"
	}
	return "unknown", "upstream_ambiguous"
}

func (m *Manager) refreshTargets(
	ctx context.Context,
	intent domain.MessageActionIntent,
) ([]domain.MessageActionTarget, string) {
	if ctx.Err() != nil {
		return nil, "cancelled"
	}
	if !m.allowed(intent) {
		return nil, "permission_denied"
	}
	targets := make([]domain.MessageActionTarget, len(intent.MessageIDs))
	for i, id := range intent.MessageIDs {
		var err error
		targets[i], err = m.Store.MessageActionTarget(ctx, intent.Conversation, id, intent.Kind == actionReadKind)
		if ctx.Err() != nil {
			return nil, "cancelled"
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, "storage_unavailable"
		}
		if err != nil || !domain.ValidActionMessageID(targets[i].MessageID) ||
			!domain.ValidActionMessageID(targets[i].ClientMessageID) {
			return nil, "metadata_unavailable"
		}
	}
	return targets, ""
}

func (m *Manager) invokeActionStep(
	ctx context.Context,
	intent domain.MessageActionIntent,
	stage string,
	targets []domain.MessageActionTarget,
) (error, error) {
	operation, cancel := context.WithTimeout(ctx, upstreamActionTimeout)
	defer cancel()
	switch stage {
	case actionReadKind:
		receipt, err := m.Source.MarkSeen(operation, targets)
		save, stop := context.WithTimeout(context.WithoutCancel(ctx), saveActionTimeout)
		defer stop()
		return err, m.Store.SaveReadReceiptStatus(save, intent.RequestID, receipt.Status)
	case "clear":
		return m.Source.React(operation, targets[0], "", true), nil
	case "add":
		return m.Source.React(operation, targets[0], intent.Reaction, false), nil
	default:
		return nil, storage.ErrActionState
	}
}

func (m *Manager) finish(ctx context.Context, id, state, reason string) (domain.MessageActionOperation, error) {
	save, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveActionTimeout)
	defer cancel()
	if err := m.Store.CompleteMessageAction(save, id, state, reason); err != nil {
		return domain.MessageActionOperation{}, err
	}
	return m.Store.MessageActionStatus(save, id)
}

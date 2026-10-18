package storage

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

var (
	ErrActionConflict = errors.New("action UUID conflicts with original arguments")
	ErrActionCapacity = errors.New("message action ledger is full")
	ErrActionState    = errors.New("invalid message action state transition")
)

const actionLedgerCapacity = 100000
const messageActionRead = "read"

func (s *Store) migrateMessageActions(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the primary migration failure.
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS message_action_operations(
 request_id TEXT PRIMARY KEY,kind TEXT NOT NULL CHECK(kind IN ('read','reaction')),
 plan TEXT NOT NULL CHECK(plan IN ('read','add','clear','clear_add')),
 conversation_type TEXT NOT NULL CHECK(conversation_type IN ('direct','group')),
 conversation_id TEXT NOT NULL,fingerprint TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('pending','dispatching','accepted','rejected','unknown','partial')),
 stage TEXT NOT NULL DEFAULT 'none',last_stage TEXT NOT NULL DEFAULT 'none',completed_steps INTEGER NOT NULL DEFAULT 0,
 reason TEXT,upstream_status INTEGER,updated_at TEXT NOT NULL);
 CREATE UNIQUE INDEX IF NOT EXISTS message_action_dispatch_claim ON message_action_operations(conversation_type,conversation_id) WHERE state='dispatching';
 INSERT OR IGNORE INTO schema_migrations VALUES(19);`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// PrepareMessageAction binds only a fingerprint, not raw protocol arguments.
// The caller validates the complete public request before preparing an intent.
func (s *Store) PrepareMessageAction(
	ctx context.Context,
	id, kind, plan string,
	ref domain.ConversationRef,
	fingerprint string,
) (domain.MessageActionOperation, error) {
	var zero domain.MessageActionOperation
	canonical, err := canonicalSendID(id)
	if err != nil {
		return zero, err
	}
	if _, err = historyAccount(ctx, s.DB); err != nil {
		return zero, err
	}
	if (kind == messageActionRead && plan != messageActionRead) ||
		(kind == "reaction" && plan != "add" && plan != "clear" && plan != "clear_add") {
		return zero, domain.Invalid("Invalid message action plan.")
	}
	_, fingerprintErr := hex.DecodeString(fingerprint)
	if (kind != messageActionRead && kind != "reaction") || !ref.Valid() || len(fingerprint) != 64 ||
		fingerprintErr != nil {
		return zero, domain.Invalid("Invalid message action intent.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback() //nolint:errcheck // Preserve the primary operation failure.
	var prior, priorKind, priorPlan, priorType, priorConversation string
	err = tx.QueryRowContext(ctx, `SELECT fingerprint,kind,plan,conversation_type,conversation_id FROM message_action_operations WHERE request_id=?`, canonical).
		Scan(&prior, &priorKind, &priorPlan, &priorType, &priorConversation)
	if err == nil &&
		(prior != fingerprint || priorKind != kind || priorPlan != plan || priorType != ref.Type || priorConversation != ref.ID) {
		return zero, ErrActionConflict
	}
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM message_action_operations`).Scan(&count); err != nil {
			return zero, err
		}
		if count >= actionLedgerCapacity {
			return zero, ErrActionCapacity
		}
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO message_action_operations(request_id,kind,plan,conversation_type,conversation_id,fingerprint,state,updated_at) VALUES(?,?,?,?,?,?,'pending',?)`,
			canonical,
			kind,
			plan,
			ref.Type,
			ref.ID,
			fingerprint,
			now(),
		)
	}
	if err != nil {
		return zero, err
	}
	if err = tx.Commit(); err != nil {
		return zero, err
	}
	return s.MessageActionStatus(ctx, canonical)
}

func (s *Store) MessageActionStatus(ctx context.Context, id string) (domain.MessageActionOperation, error) {
	var op domain.MessageActionOperation
	canonical, err := canonicalSendID(id)
	if err != nil {
		return op, err
	}
	var stamp string
	err = s.DB.QueryRowContext(ctx, `SELECT request_id,kind,plan,conversation_type,conversation_id,state,stage,completed_steps,reason,upstream_status,updated_at FROM message_action_operations WHERE request_id=?`, canonical).
		Scan(&op.RequestID, &op.Kind, &op.Plan, &op.Conversation.Type, &op.Conversation.ID, &op.Status, &op.Stage, &op.CompletedSteps, &op.Reason, &op.UpstreamStatus, &stamp)
	if err != nil {
		return op, err
	}
	op.UpdatedAt, err = time.Parse(time.RFC3339Nano, stamp)
	return op, err
}

// ClaimMessageAction serializes actions within one conversation. Busy intents
// remain pending: no attempted upstream step is repeated by this operation.
func (s *Store) ClaimMessageAction(ctx context.Context, id string) (bool, error) {
	canonical, err := canonicalSendID(id)
	if err != nil {
		return false, err
	}
	result, err := s.DB.ExecContext(
		ctx,
		`UPDATE message_action_operations AS candidate SET state='dispatching',updated_at=? WHERE request_id=? AND state='pending' AND NOT EXISTS(SELECT 1 FROM message_action_operations busy WHERE busy.state='dispatching' AND busy.conversation_type=candidate.conversation_type AND busy.conversation_id=candidate.conversation_id)`,
		now(),
		canonical,
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// BeginMessageActionStep checkpoints uncertainty BEFORE calling upstream.
func (s *Store) BeginMessageActionStep(ctx context.Context, id, stage string, completed int) error {
	if stage != messageActionRead && stage != "clear" && stage != "add" {
		return ErrActionState
	}
	canonical, err := canonicalSendID(id)
	if err != nil {
		return err
	}
	result, err := s.DB.ExecContext(
		ctx,
		`UPDATE message_action_operations SET stage=?,updated_at=? WHERE request_id=? AND state='dispatching' AND completed_steps=? AND stage='none' AND ((?='read' AND plan='read' AND completed_steps=0) OR (?='clear' AND plan IN ('clear','clear_add') AND completed_steps=0) OR (?='add' AND ((plan='add' AND completed_steps=0) OR (plan='clear_add' AND completed_steps=1 AND last_stage='clear'))))`,
		stage,
		now(),
		canonical,
		completed,
		stage,
		stage,
		stage,
	)
	return oneActionTransition(result, err)
}

// ConfirmMessageActionStep is called only after an unambiguous upstream success.
func (s *Store) ConfirmMessageActionStep(ctx context.Context, id, stage string, completed int) error {
	canonical, err := canonicalSendID(id)
	if err != nil {
		return err
	}
	result, err := s.DB.ExecContext(
		ctx,
		`UPDATE message_action_operations SET last_stage=stage,stage='none',completed_steps=completed_steps+1,updated_at=? WHERE request_id=? AND state='dispatching' AND stage=? AND completed_steps=? AND stage!='none'`,
		now(),
		canonical,
		stage,
		completed,
	)
	return oneActionTransition(result, err)
}

func (s *Store) CompleteMessageAction(ctx context.Context, id, state, reason string) error {
	canonical, err := canonicalSendID(id)
	if err != nil {
		return err
	}
	if state != "accepted" && state != "rejected" && state != "unknown" && state != statePartialText {
		return ErrActionState
	}
	switch reason {
	case "",
		"cancelled",
		"upstream_rejected",
		"upstream_ambiguous",
		"upstream_unverified",
		"metadata_unavailable",
		"storage_unavailable",
		"permission_denied",
		"not_authenticated":
	default:
		return ErrActionState
	}
	if (state == "accepted") != (reason == "") {
		return ErrActionState
	}
	var nullable *string
	if reason != "" {
		nullable = &reason
	}
	result, err := s.DB.ExecContext(
		ctx,
		`UPDATE message_action_operations SET state=?,reason=?,updated_at=? WHERE request_id=? AND state='dispatching' AND (?!='accepted' OR (stage='none' AND ((plan='read' AND completed_steps=1 AND last_stage='read') OR (plan='clear' AND completed_steps=1 AND last_stage='clear') OR (plan='add' AND completed_steps=1 AND last_stage='add') OR (plan='clear_add' AND completed_steps=2 AND last_stage='add')))) AND (?!='partial' OR completed_steps>0) AND (?!='rejected' OR completed_steps=0)`,
		state,
		nullable,
		now(),
		canonical,
		state,
		state,
		state,
	)
	return oneActionTransition(result, err)
}

func oneActionTransition(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrActionState
	}
	return nil
}

// RecoverInterruptedMessageActions runs only once under the service account lock.
// It never marks an interrupted write as accepted or automatically replays it.
func (s *Store) RecoverInterruptedMessageActions(ctx context.Context) error {
	_, err := s.DB.ExecContext(
		ctx,
		`UPDATE message_action_operations SET state='unknown',reason='interrupted',updated_at=? WHERE state='dispatching'`,
		now(),
	)
	return err
}

// SaveReadReceiptStatus retains only the native numeric status, never a payload.
func (s *Store) SaveReadReceiptStatus(ctx context.Context, id string, status *int) error {
	canonical, err := canonicalSendID(id)
	if err != nil {
		return err
	}
	result, err := s.DB.ExecContext(
		ctx,
		`UPDATE message_action_operations SET upstream_status=?,updated_at=? WHERE request_id=? AND kind='read' AND state='dispatching' AND stage='read'`,
		status,
		now(),
		canonical,
	)
	return oneActionTransition(result, err)
}

// AccessibleMessageActionStatus is the public read boundary. The internal ledger
// reader remains usable for completing an operation after access revocation.
func (s *Store) AccessibleMessageActionStatus(ctx context.Context, id string) (domain.MessageActionOperation, error) {
	var zero domain.MessageActionOperation
	if _, err := historyAccount(ctx, s.DB); err != nil {
		return zero, err
	}
	result, err := s.MessageActionStatus(ctx, id)
	if err != nil {
		return zero, err
	}
	if !s.AllowsConversation(result.Conversation) {
		return zero, sql.ErrNoRows
	}
	return result, nil
}

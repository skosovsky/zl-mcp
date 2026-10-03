// Package messaging owns explicit direct sends, separate from collection policy.
package messaging

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

var ErrRejected = errors.New("Zalo rejected send")

type Sender interface {
	SendDirect(context.Context, string, string, *domain.SendQuote) (string, error)
}

type Manager struct {
	Store      *storage.Store
	Sender     Sender
	Enabled    bool
	Recipients map[string]bool
}

func failure(code, message string) error {
	return &domain.Error{Code: code, Message: message, NextAction: domain.NextAction{Instruction: "Inspect send status before creating another request."}, Details: map[string]any{}}
}

func (m *Manager) Call(ctx context.Context, method string, args any) (map[string]any, error) {
	b, err := json.Marshal(args)
	if err != nil {
		return nil, domain.Invalid("Invalid send arguments.")
	}
	var r domain.SendRequest
	if json.Unmarshal(b, &r) != nil {
		return nil, domain.Invalid("Invalid send arguments.")
	}
	if method == "zalo_get_send_status" {
		op, err := m.Store.SendStatus(ctx, r.RequestID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, failure("NOT_FOUND", "Send operation is unavailable.")
			}
			var invalid *domain.Error
			if errors.As(err, &invalid) {
				return nil, invalid
			}
			return nil, failure("STORAGE_ERROR", "Cannot read send status; do not create another request.")
		}
		return view(op), nil
	}
	if method != "zalo_send_direct_message" {
		return nil, domain.Invalid("Unknown messaging method.")
	}
	if err = r.Validate(); err != nil {
		return nil, err
	}
	if !m.Enabled || (len(m.Recipients) > 0 && !m.Recipients[r.RecipientID]) {
		return nil, failure("PERMISSION_DENIED", "Direct sending is not permitted for this recipient.")
	}
	op, err := m.Store.PrepareSend(ctx, r)
	if errors.Is(err, storage.ErrSendConflict) {
		return nil, failure("REQUEST_CONFLICT", "request_id already belongs to different send arguments.")
	}
	if errors.Is(err, storage.ErrSendCapacity) {
		return nil, failure("CAPACITY_EXCEEDED", "Send operation ledger is full.")
	}
	if err != nil {
		return nil, err
	}
	if op.Status != "pending" {
		return view(op), nil
	}
	if m.Sender == nil {
		return nil, failure("NOT_AUTHENTICATED", "Zalo session is unavailable; the pending operation was not sent.")
	}
	var quote *domain.SendQuote
	if r.ReplyTo != nil {
		quote, err = m.Store.SendQuote(ctx, r.RecipientID, *r.ReplyTo)
		if err != nil {
			var invalid *domain.Error
			if errors.Is(err, sql.ErrNoRows) || errors.As(err, &invalid) {
				return nil, failure("QUOTE_UNAVAILABLE", "Retained direct message has no supported quote metadata; nothing was sent.")
			}
			return nil, failure("STORAGE_ERROR", "Cannot read reply source; nothing was sent.")
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	claimed, err := m.Store.ClaimSend(ctx, r.RequestID)
	if err != nil {
		return nil, err
	}
	if !claimed {
		op, err = m.Store.SendStatus(ctx, r.RequestID)
		if err != nil {
			return nil, err
		}
		return view(op), nil
	}
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	id, sendErr := m.Sender.SendDirect(sendCtx, r.RecipientID, r.Text, quote)
	cancel()
	state := "sent"
	var messageID, reason *string
	if sendErr == nil && id != "" {
		messageID = &id
	} else {
		state = "unknown"
		category := "upstream_ambiguous"
		if errors.Is(sendErr, ErrRejected) {
			state = "failed"
			category = "upstream_rejected"
		}
		reason = &category
	}
	// A cancelled client must not prevent recording a confirmed upstream result.
	finish, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = m.Store.CompleteSend(finish, r.RequestID, state, messageID, reason); err != nil {
		return nil, failure("STORAGE_ERROR", "Cannot record send result; do not create another request.")
	}
	op, err = m.Store.SendStatus(finish, r.RequestID)
	if err != nil {
		return nil, err
	}
	return view(op), nil
}

func view(op domain.SendOperation) map[string]any {
	b, _ := json.Marshal(op)
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	return v
}

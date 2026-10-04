package historyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type Manager struct{ Store *storage.Store }

func managerError(code, message string) error {
	return &domain.Error{Code: code, Message: message, NextAction: domain.NextAction{Instruction: "Inspect import status; reuse the original request UUID for retries. Check collector authentication and collection policy."}, Details: map[string]any{}}
}

func (m *Manager) Call(ctx context.Context, method string, arguments any) (map[string]any, error) {
	name := "history_import_operation"
	if method == "zalo_import_conversation_history" {
		name = "history_import_request"
	} else if method != "zalo_get_history_import_status" && method != "zalo_cancel_history_import" {
		return nil, domain.Invalid("Unknown history import method.")
	}
	b, err := json.Marshal(arguments)
	if err != nil {
		return nil, domain.Invalid("Invalid history import arguments.")
	}
	var wire any
	if json.Unmarshal(b, &wire) != nil {
		return nil, domain.Invalid("Invalid history import arguments.")
	}
	schema, err := contracts.Compile(name, "input")
	if err != nil {
		return nil, managerError("STORAGE_ERROR", "History contract is unavailable.")
	}
	if schema.Validate(wire) != nil {
		return nil, domain.Invalid("History arguments do not match the executable contract.")
	}
	var op storage.HistoryOperation
	if method == "zalo_import_conversation_history" {
		var r domain.HistoryImportRequest
		if json.Unmarshal(b, &r) != nil {
			return nil, domain.Invalid("Invalid history import request.")
		}
		op, err = m.Store.PrepareHistoryOperation(ctx, r)
	} else {
		var request struct {
			OperationID string `json:"operation_id"`
		}
		if json.Unmarshal(b, &request) != nil {
			return nil, domain.Invalid("Invalid operation identity.")
		}
		// Apply the same read policy before cancellation/status. Worker recovery
		// handles revoked operations without exposing them through client reads.
		op, err = m.Store.HistoryOperation(ctx, request.OperationID)
		if err == nil && method == "zalo_cancel_history_import" {
			op, err = m.Store.CancelHistoryOperation(ctx, request.OperationID)
		}
	}
	if err != nil {
		var typed *domain.Error
		switch {
		case errors.As(err, &typed):
			return nil, typed
		case errors.Is(err, storage.ErrHistoryConflict):
			return nil, managerError("REQUEST_CONFLICT", "request_id belongs to different history arguments.")
		case errors.Is(err, storage.ErrHistoryBusy):
			return nil, managerError("OPERATION_IN_PROGRESS", "This conversation already has active history work.")
		case errors.Is(err, storage.ErrHistoryCapacity):
			return nil, managerError("CAPACITY_EXCEEDED", "History operation ledger is full.")
		case errors.Is(err, sql.ErrNoRows) && method == "zalo_import_conversation_history":
			return nil, managerError("NOT_AUTHENTICATED", "No authenticated account is bound to this state directory.")
		case errors.Is(err, sql.ErrNoRows):
			return nil, managerError("NOT_FOUND", "No accessible history operation was found.")
		default:
			return nil, managerError("STORAGE_ERROR", "Cannot access the history operation; do not create a replacement UUID blindly.")
		}
	}
	encoded, err := json.Marshal(op.Status)
	if err != nil {
		return nil, managerError("STORAGE_ERROR", "Cannot encode history status.")
	}
	result := map[string]any{}
	if json.Unmarshal(encoded, &result) != nil {
		return nil, managerError("STORAGE_ERROR", "Cannot encode history status.")
	}
	return result, nil
}

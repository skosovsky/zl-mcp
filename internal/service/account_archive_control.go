package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func accountArchiveMethod(method string) bool {
	return method == "cli_restore_account_archive" || method == "cli_preserve_account_archive" ||
		method == "cli_prepare_account_archive" ||
		method == "cli_capture_account_archive" ||
		method == "cli_account_archive_status" ||
		method == "cli_inspect_account_archive" ||
		method == "cli_remove_account_archive"
}

func (p *membershipPort) accountArchiveControl(w http.ResponseWriter, r *http.Request, body []byte, method string) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(err error) {
		writeAccountArchiveFailure(w, err)
	}
	if r.Method != http.MethodPost || r.URL.Path != "/rpc" || !uniqueLedgerJSON(body) {
		fail(domain.Invalid("Use a unique JSON object with POST /rpc."))
		return
	}
	input, err := contracts.Compile(method, "input")
	var raw any
	if err != nil || json.Unmarshal(body, &raw) != nil || input.Validate(raw) != nil {
		fail(domain.Invalid("Account archive request violates its contract."))
		return
	}
	var envelope struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(body, &envelope) != nil || p.store == nil {
		fail(domain.Invalid("Account archive state is unavailable."))
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if p.lifecycle != nil {
		stop := context.AfterFunc(p.lifecycle, cancel)
		defer stop()
		if p.lifecycle.Err() != nil {
			cancel()
		}
	}
	response, err := p.runAccountArchiveMethod(ctx, method, envelope.Arguments)
	if err != nil {
		fail(err)
		return
	}
	encoded, err := json.Marshal(response)
	var output any
	schema, e := contracts.Compile(method, "output")
	if err != nil || e != nil || json.Unmarshal(encoded, &output) != nil || schema.Validate(output) != nil {
		fail(domain.Invalid("Account archive result violates its contract."))
		return
	}
	_, _ = w.Write(append(encoded, '\n'))
}

func writeAccountArchiveFailure(w http.ResponseWriter, err error) {
	code := "ARCHIVE_UNAVAILABLE"
	switch {
	case errors.Is(err, storage.ErrMobileBackupConflict), errors.Is(err, mobilebackup.ErrRetainedConflict):
		code = "REQUEST_CONFLICT"
	case errors.Is(err, storage.ErrMobileBackupBusy):
		code = "OPERATION_ACTIVE"
	case errors.Is(err, storage.ErrMobileBackupState):
		code = "INVALID_OPERATION_STATE"
	case errors.Is(err, storage.ErrMobileBackupCapacity), errors.Is(err, mobilebackup.ErrRetainedCapacity):
		code = "CAPACITY_EXCEEDED"
	case errors.Is(err, mobilebackup.ErrRetainedExpired):
		code = "SOURCE_EXPIRED"
	case errors.Is(err, mobilebackup.ErrSelectedConversationUnavailable):
		code = "SOURCE_CONVERSATION_UNAVAILABLE"
	case errors.Is(err, mobilebackup.ErrRetainedAbsent):
		code = "SOURCE_NOT_CAPTURED"
	}
	d := &domain.Error{
		Code:    code,
		Message: "Local account archive operation failed.",
		NextAction: domain.NextAction{
			Instruction: "Read the attempt or retained source status. Do not request synchronization automatically.",
		},
		Details: map[string]any{},
	}
	if typed, ok := errors.AsType[*domain.Error](err); ok {
		d = typed
	}
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(d)
}

func (p *membershipPort) runAccountArchiveMethod(ctx context.Context, method string, arguments json.RawMessage) (any, error) {
	var err error
	var response any
	switch method {
	case "cli_restore_account_archive":
		var args struct {
			ID     string `json:"source_id"`
			Backup string `json:"backup_name"`
			Digest string `json:"expected_digest"`
		}
		if json.Unmarshal(arguments, &args) != nil {
			return nil, domain.Invalid("Invalid backup reference.")
		}
		response, err = p.restoreAccountArchive(ctx, args.ID, args.Backup, args.Digest)
	case "cli_preserve_account_archive":
		var args struct {
			ID string `json:"source_id"`
		}
		if json.Unmarshal(arguments, &args) != nil {
			return nil, domain.Invalid("Invalid source reference.")
		}
		response, err = p.preserveAccountArchive(ctx, args.ID)
	case "cli_prepare_account_archive":
		if p.archives == nil {
			return nil, mobilebackup.ErrRetainedArchive
		}
		var request domain.MobileBackupRequest
		if json.Unmarshal(arguments, &request) != nil {
			return nil, domain.Invalid("Invalid account capture request.")
		}
		// Preparation is not phone dispatch. The explicit scope is part of the
		// durable fingerprint and cannot be introduced by a message payload.
		response, err = p.store.PrepareMobileBackup(ctx, request)
	case "cli_capture_account_archive":
		var args struct {
			ID       string `json:"operation_id"`
			Revision int64  `json:"revision"`
		}
		if json.Unmarshal(arguments, &args) != nil {
			return nil, domain.Invalid("Invalid account capture reference.")
		}
		response, err = p.captureAccountArchive(ctx, args.ID, args.Revision)
	case "cli_inspect_account_archive":
		var args struct {
			ID               string `json:"source_id"`
			Since            string `json:"since"`
			Until            string `json:"until"`
			Offset           int    `json:"offset"`
			Limit            int    `json:"limit"`
			ConversationType string `json:"conversation_type"`
			ConversationID   string `json:"conversation_id"`
			IncludeMetadata  bool   `json:"include_metadata_diagnostics"`
			Storage          string `json:"source_storage"`
		}
		if json.Unmarshal(arguments, &args) != nil {
			return nil, domain.Invalid("Invalid archive inspection.")
		}
		response, err = p.inspectAccountArchiveFrom(
			ctx,
			args.Storage,
			args.ID,
			args.Since,
			args.Until,
			args.Offset,
			args.Limit,
			domain.ConversationRef{Type: args.ConversationType, ID: args.ConversationID},
			args.IncludeMetadata,
		)
	case "cli_remove_account_archive":
		var args struct {
			ID      string `json:"source_id"`
			Storage string `json:"source_storage"`
		}
		if json.Unmarshal(arguments, &args) != nil {
			return nil, domain.Invalid("Invalid source reference.")
		}
		response, err = p.removeAccountArchiveFrom(ctx, args.ID, args.Storage)
	case "cli_account_archive_status":
		var args struct {
			ID string `json:"source_id"`
		}
		if json.Unmarshal(arguments, &args) != nil {
			return nil, domain.Invalid("Invalid retained source reference.")
		}
		response, err = p.accountArchiveStatus(ctx, args.ID)
	}
	return response, err
}

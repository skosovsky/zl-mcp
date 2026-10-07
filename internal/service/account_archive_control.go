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
	return method == "cli_prepare_account_archive" || method == "cli_capture_account_archive" || method == "cli_account_archive_status" || method == "cli_inspect_account_archive" || method == "cli_remove_account_archive"
}

func (p *membershipPort) accountArchiveControl(w http.ResponseWriter, r *http.Request, body []byte, method string) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(err error) {
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
		case errors.Is(err, mobilebackup.ErrRetainedAbsent):
			code = "SOURCE_NOT_CAPTURED"
		}
		d := &domain.Error{Code: code, Message: "Local account archive operation failed.", NextAction: domain.NextAction{Instruction: "Read the attempt or retained source status. Do not request synchronization automatically."}, Details: map[string]any{}}
		var typed *domain.Error
		if errors.As(err, &typed) {
			d = typed
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(d)
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
	var response any
	switch method {
	case "cli_prepare_account_archive":
		if p.archives == nil {
			fail(mobilebackup.ErrRetainedArchive)
			return
		}
		var request domain.MobileBackupRequest
		if json.Unmarshal(envelope.Arguments, &request) != nil {
			fail(domain.Invalid("Invalid account capture request."))
			return
		}
		// Preparation is not phone dispatch. The explicit scope is part of the
		// durable fingerprint and cannot be introduced by a message payload.
		response, err = p.store.PrepareMobileBackup(ctx, request)
	case "cli_capture_account_archive":
		var args struct {
			ID       string `json:"operation_id"`
			Revision int64  `json:"revision"`
		}
		if json.Unmarshal(envelope.Arguments, &args) != nil {
			fail(domain.Invalid("Invalid account capture reference."))
			return
		}
		response, err = p.captureAccountArchive(ctx, args.ID, args.Revision)
	case "cli_inspect_account_archive":
		var args struct {
			ID     string `json:"source_id"`
			Since  string `json:"since"`
			Until  string `json:"until"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		if json.Unmarshal(envelope.Arguments, &args) != nil {
			fail(domain.Invalid("Invalid archive inspection."))
			return
		}
		response, err = p.inspectAccountArchive(ctx, args.ID, args.Since, args.Until, args.Offset, args.Limit)
	case "cli_remove_account_archive":
		var args struct {
			ID string `json:"source_id"`
		}
		if json.Unmarshal(envelope.Arguments, &args) != nil {
			fail(domain.Invalid("Invalid source reference."))
			return
		}
		response, err = p.removeAccountArchive(ctx, args.ID)
	case "cli_account_archive_status":
		var args struct {
			ID string `json:"source_id"`
		}
		if json.Unmarshal(envelope.Arguments, &args) != nil {
			fail(domain.Invalid("Invalid retained source reference."))
			return
		}
		response, err = p.accountArchiveStatus(ctx, args.ID)
	}
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

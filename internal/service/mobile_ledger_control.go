package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func mobileLedgerMethod(method string) bool {
	return method == "cli_prepare_mobile_backup" || method == "cli_mobile_backup_status" || method == "cli_cancel_prepared_mobile_backup" || (method == "cli_probe_mobile_backup_offer" || method == "cli_probe_mobile_backup_archive")
}

// uniqueLedgerJSON rejects ambiguous object keys before normal contract decoding.
// Ledger envelopes contain only objects and scalars, so arrays are rejected.
func uniqueLedgerJSON(body []byte) bool {
	d := json.NewDecoder(bytes.NewReader(body))
	var value func(int) error
	value = func(depth int) error {
		if depth > 4 {
			return domain.Invalid("Invalid ledger request.")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delimiter, object := token.(json.Delim)
		if !object {
			return nil
		}
		if delimiter != '{' {
			return domain.Invalid("Invalid ledger request.")
		}
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return domain.Invalid("Invalid ledger request.")
			}
			seen[name] = true
			if err = value(depth + 1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return domain.Invalid("Invalid ledger request.")
		}
		return nil
	}
	if value(0) != nil {
		return false
	}
	_, err := d.Token()
	return errors.Is(err, io.EOF)
}

func (p *membershipPort) mobileLedgerControl(w http.ResponseWriter, r *http.Request, body []byte, method string) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(err error) {
		code := "OPERATION_FAILED"
		switch {
		case errors.Is(err, storage.ErrMobileBackupConflict):
			code = "REQUEST_CONFLICT"
		case errors.Is(err, storage.ErrMobileBackupBusy):
			code = "OPERATION_ACTIVE"
		case errors.Is(err, storage.ErrMobileBackupCapacity):
			code = "CAPACITY_EXCEEDED"
		case errors.Is(err, storage.ErrMobileBackupState):
			code = "INVALID_OPERATION_STATE"
		case errors.Is(err, sql.ErrNoRows):
			code = "NOT_FOUND"
		}
		de := &domain.Error{Code: code, Message: "Local mobile attempt request failed.", NextAction: domain.NextAction{Instruction: "Read the local attempt status; do not dispatch or retry phone work automatically."}, Details: map[string]any{}}
		var typed *domain.Error
		if errors.As(err, &typed) {
			de = typed
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(de)
	}
	if r.Method != http.MethodPost || r.URL.Path != "/rpc" || !uniqueLedgerJSON(body) {
		fail(domain.Invalid("Use a unique JSON object with POST /rpc."))
		return
	}
	input, e := contracts.Compile(method, "input")
	var raw any
	if e != nil || json.Unmarshal(body, &raw) != nil || input.Validate(raw) != nil {
		fail(domain.Invalid("Local ledger request does not match its contract."))
		return
	}
	var envelope struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(body, &envelope) != nil || p.store == nil {
		fail(domain.Invalid("Local ledger is unavailable."))
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
	var probe map[string]any
	var result storage.MobileBackupAttempt
	if method == "cli_prepare_mobile_backup" {
		var request domain.MobileBackupRequest
		if json.Unmarshal(envelope.Arguments, &request) != nil {
			fail(domain.Invalid("Invalid mobile attempt request."))
			return
		}
		result, e = p.store.PrepareMobileBackup(ctx, request)
	} else {
		var args struct {
			ID       string `json:"operation_id"`
			Revision int64  `json:"revision"`
		}
		if json.Unmarshal(envelope.Arguments, &args) != nil {
			fail(domain.Invalid("Invalid mobile attempt reference."))
			return
		}
		result, e = p.store.MobileBackupAttempt(ctx, args.ID)
		if e == nil && (method == "cli_probe_mobile_backup_offer" || method == "cli_probe_mobile_backup_archive") {
			if method == "cli_probe_mobile_backup_archive" {
				probe, e = p.probeMobileArchive(ctx, args.ID, args.Revision)
			} else {
				probe, e = p.probeMobileOffer(ctx, args.ID, args.Revision)
			}
		}
		if e == nil && method == "cli_cancel_prepared_mobile_backup" {
			if result.Revision != args.Revision || result.State != "prepared" && result.State != "cancelled" {
				e = storage.ErrMobileBackupState
			} else if result.State == "prepared" {
				result, e = p.store.ProgressMobileBackup(ctx, args.ID, args.Revision, "cancelled")
			}
		}
	}
	if e != nil {
		fail(e)
		return
	}
	var response any = result
	if method == "cli_probe_mobile_backup_offer" || method == "cli_probe_mobile_backup_archive" {
		response = probe
	}
	encoded, e := json.Marshal(response)
	var output any
	schema, schemaErr := contracts.Compile(method, "output")
	if e != nil || json.Unmarshal(encoded, &output) != nil || schemaErr != nil || schema.Validate(output) != nil {
		fail(domain.Invalid("Local ledger result does not match its contract."))
		return
	}
	_, _ = w.Write(append(encoded, '\n'))
}

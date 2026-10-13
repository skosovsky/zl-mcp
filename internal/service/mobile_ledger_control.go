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
	return accountArchiveMethod(method) || method == operationPrepareMobileBackup ||
		method == "cli_mobile_backup_status" ||
		method == operationCancelPreparedMobileBackup ||
		(method == operationProbeMobileBackupOffer || method == operationProbeMobileBackupArchive)
}

// uniqueLedgerJSON rejects ambiguous object keys before normal contract decoding.
// Ledger envelopes contain only objects and scalars, so arrays are rejected.
//
//nolint:gocognit // The bounded JSON token recognizer keeps nesting and duplicate-key rejection in one lexical pass; duplicate keys must not be normalized away.
func uniqueLedgerJSON(body []byte) bool {
	d := json.NewDecoder(bytes.NewReader(body))
	var value func(int) error
	value = func(depth int) error {
		if depth > maximumLedgerJSONDepth {
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
			key, tokenErr := d.Token()
			if tokenErr != nil {
				return tokenErr
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return domain.Invalid("Invalid ledger request.")
			}
			seen[name] = true
			if tokenErr = value(depth + 1); tokenErr != nil {
				return tokenErr
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
	if accountArchiveMethod(method) {
		p.accountArchiveControl(w, r, body, method)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fail := func(err error) {
		writeMobileLedgerFailure(w, err)
	}
	if r.Method != http.MethodPost || r.URL.Path != controlRPCPath || !uniqueLedgerJSON(body) {
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
	response, e := p.runMobileLedgerMethod(ctx, method, envelope.Arguments)
	if e != nil {
		fail(e)
		return
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

func writeMobileLedgerFailure(w http.ResponseWriter, err error) {
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
	//nolint:exhaustruct_v5 // Omitted error fields retain their non-retryable/absent defaults in the existing response contract.
	de := &domain.Error{
		Code:    code,
		Message: "Local mobile attempt request failed.",
		//nolint:exhaustruct_v5 // Optional tool/arguments stay absent until an actionable follow-up exists; preserve the response contract.
		NextAction: domain.NextAction{
			Instruction: "Read the local attempt status; do not dispatch or retry phone work automatically.",
		},
		Details: map[string]any{},
	}
	if typed, ok := errors.AsType[*domain.Error](err); ok {
		de = typed
	}
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(de)
}

func (p *membershipPort) runMobileLedgerMethod(
	ctx context.Context,
	method string,
	arguments json.RawMessage,
) (any, error) {
	var e error
	var probe map[string]any
	var result storage.MobileBackupAttempt
	if method == operationPrepareMobileBackup {
		var request domain.MobileBackupRequest
		if json.Unmarshal(arguments, &request) != nil {
			return nil, domain.Invalid("Invalid mobile attempt request.")
		}
		return p.store.PrepareMobileBackup(ctx, request)
	}

	var args struct {
		ID               string `json:"operation_id"`
		Revision         int64  `json:"revision"`
		ComparisonSendID string `json:"comparison_send_request_id"`
	}
	if json.Unmarshal(arguments, &args) != nil {
		return nil, domain.Invalid("Invalid mobile attempt reference.")
	}
	result, e = p.store.MobileBackupAttempt(ctx, args.ID)
	if e == nil && (method == operationProbeMobileBackupOffer || method == operationProbeMobileBackupArchive) {
		if method == operationProbeMobileBackupArchive {
			probe, e = p.probeMobileArchiveComparison(ctx, args.ID, args.Revision, args.ComparisonSendID)
		} else {
			probe, e = p.probeMobileOffer(ctx, args.ID, args.Revision)
		}
	}
	if e == nil && method == operationCancelPreparedMobileBackup {
		if result.Revision != args.Revision || result.State != statePreparedText && result.State != "cancelled" {
			e = storage.ErrMobileBackupState
		} else if result.State == statePreparedText {
			result, e = p.store.ProgressMobileBackup(ctx, args.ID, args.Revision, "cancelled")
		}
	}
	if e != nil {
		return nil, e
	}
	var response any = result
	if method == operationProbeMobileBackupOffer || method == operationProbeMobileBackupArchive {
		response = probe
	}
	return response, nil
}

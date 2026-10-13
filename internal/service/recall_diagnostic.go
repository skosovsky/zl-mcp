package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

const recallDiagnosticText = "zl-mcp: проверка архивного отзыва, сообщение будет отозвано. Отвечать не нужно."

type diagnosticRecaller interface {
	AccountID() string
	UndoDiagnosticDirect(context.Context, string, string, string) (int, error)
}

type recallReceipt struct {
	State  string `json:"state"`
	Status *int   `json:"upstream_status,omitempty"`
}

func (p *membershipPort) recallControl(w http.ResponseWriter, r *http.Request, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	fail := func() {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).
			//nolint:exhaustruct_v5 // Omitted error fields retain their non-retryable/absent defaults in the existing response contract. Optional tool/arguments stay absent until an actionable follow-up exists; preserve the response contract.
			Encode(&domain.Error{Code: "RECALL_DIAGNOSTIC_UNAVAILABLE", Message: "Local recall diagnostic is unavailable.", NextAction: domain.NextAction{Instruction: "Read the diagnostic receipt or collector status; never redispatch an ambiguous recall."}, Details: map[string]any{}})
	}
	schema, err := contracts.Compile(operationProbeRecall, "input")
	var raw any
	if err != nil || r.Method != http.MethodPost || r.URL.Path != controlRPCPath || !uniqueLedgerJSON(body) ||
		json.Unmarshal(body, &raw) != nil ||
		schema.Validate(raw) != nil {
		fail()
		return
	}
	var request struct {
		Arguments struct {
			SendID string `json:"send_request_id"`
		} `json:"arguments"`
	}
	if json.Unmarshal(body, &request) != nil {
		fail()
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), membershipPortrecallControlTimeout)
	defer cancel()
	if p.lifecycle != nil {
		stop := context.AfterFunc(p.lifecycle, cancel)
		defer stop()
		if p.lifecycle.Err() != nil {
			cancel()
		}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	result, err := p.probeRecall(ctx, request.Arguments.SendID)
	if err != nil {
		fail()
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

//nolint:cyclop,funlen,gocognit,gocyclo // This one-effect owner keeps the exclusive receipt open from fsynced reservation through dispatch and outcome sync; receipt reading, quote validation and writing are separate helpers.
func (p *membershipPort) probeRecall(ctx context.Context, id string) (recallReceipt, error) {
	bad := errors.New("recall diagnostic unavailable")
	//nolint:exhaustruct_v5 // recallReceipt initializes only this owner/phase; optional collaborators, buffers and counters keep their zero defaults.
	empty := recallReceipt{}
	normalized, e := uuid.Parse(id)
	recaller, ok := p.sender.(diagnosticRecaller)
	if e != nil || normalized.String() != id || !ok || p.store == nil || ctx.Err() != nil ||
		!filepath.IsAbs(p.stateDir) {
		return empty, bad
	}
	own := recaller.AccountID()
	if !canonicalDiagnosticID(own) {
		return empty, bad
	}
	op, e := p.store.SendStatus(ctx, id)
	if e != nil || op.Status != "sent" || op.MessageID == nil {
		return empty, bad
	}
	parent, e := os.OpenRoot(p.stateDir)
	if e != nil {
		return empty, bad
	}
	// G104: Cleanup preserves the original setup/operation error.
	defer parent.Close()
	if e = parent.Mkdir("diagnostic-recalls", privateDiagnosticDirectoryMode); e != nil && !errors.Is(e, os.ErrExist) {
		return empty, bad
	}
	info, e := parent.Lstat("diagnostic-recalls")
	if e != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return empty, bad
	}
	root, e := parent.OpenRoot("diagnostic-recalls")
	if e != nil {
		return empty, bad
	}
	// G104: Cleanup preserves the original setup/operation error.
	defer root.Close()
	digest := sha256.Sum256([]byte(own + "\x00" + id))
	name := hex.EncodeToString(digest[:]) + ".receipt"
	prior, exists, receiptErr := readRecallReceipt(root, name, bad)
	if receiptErr != nil || exists {
		return prior, receiptErr
	}
	quote, quoteErr := p.recallQuote(ctx, op, own, bad)
	if quoteErr != nil {
		return empty, quoteErr
	}
	dir, e := root.Open(".")
	if e != nil {
		return empty, bad
	}
	entries, e := dir.ReadDir(recallReceiptDirectoryReadLimit)
	//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
	dir.Close()
	if e != nil && !errors.Is(e, io.EOF) || len(entries) >= 20 {
		return empty, bad
	}
	f, e := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, privateDiagnosticFileMode)
	if e != nil {
		return empty, bad
	}
	// G104: Cleanup preserves the original setup/operation error.
	defer f.Close()
	write := func(out recallReceipt) error { return writeRecallReceipt(f, out) }
	//nolint:exhaustruct_v5 // recallReceipt initializes only this owner/phase; optional collaborators, buffers and counters keep their zero defaults.
	out := recallReceipt{State: "reserved"}
	if write(out) != nil {
		return empty, bad
	}
	dir, e = root.Open(".")
	if e != nil {
		return empty, bad
	}
	e = dir.Sync()
	//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
	dir.Close()
	if e != nil {
		return empty, bad
	}
	if ctx.Err() != nil {
		return out, nil
	}
	status, e := recaller.UndoDiagnosticDirect(ctx, op.RecipientID, *op.MessageID, quote.Metadata.ClientMessageID)
	out.State = stateUnknownText
	if e == nil {
		out.State = "upstream_response"
		out.Status = &status
	}
	if write(out) != nil {
		return empty, bad
	}
	return out, nil
}

func canonicalDiagnosticID(id string) bool {
	n, e := strconv.ParseUint(id, 10, 64)
	return e == nil && n > 0 && strconv.FormatUint(n, 10) == id
}

func readRecallReceipt(root *os.Root, name string, bad error) (recallReceipt, bool, error) {
	if info, lstatErr := root.Lstat(name); lstatErr == nil {
		out, readErr := decodeRecallReceipt(root, name, info, bad)
		return out, readErr == nil, readErr
	} else if !errors.Is(lstatErr, os.ErrNotExist) {
		return recallReceipt{}, false, bad
	}
	return recallReceipt{}, false, nil
}

func (p *membershipPort) recallQuote(
	ctx context.Context,
	op domain.SendOperation,
	own string,
	bad error,
) (*domain.SendQuote, error) {
	if time.Since(op.UpdatedAt) > 30*time.Minute || time.Until(op.UpdatedAt) > time.Minute ||
		!canonicalDiagnosticID(op.RecipientID) ||
		!canonicalDiagnosticID(*op.MessageID) {
		return nil, bad
	}
	quote, e := p.store.SendQuote(ctx, op.RecipientID, *op.MessageID)
	if e != nil || quote.SenderID != own || quote.Text != recallDiagnosticText ||
		!canonicalDiagnosticID(quote.Metadata.ClientMessageID) {
		return nil, bad
	}
	return quote, nil
}

func writeRecallReceipt(f *os.File, out recallReceipt) error {
	b, marshalErr := json.Marshal(out)
	if marshalErr != nil {
		return marshalErr
	}
	if marshalErr = f.Truncate(0); marshalErr != nil {
		return marshalErr
	}
	if _, marshalErr = f.WriteAt(b, 0); marshalErr != nil {
		return marshalErr
	}
	return f.Sync()
}

func decodeRecallReceipt(root *os.Root, name string, info os.FileInfo, bad error) (recallReceipt, error) {
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 512 {
		return recallReceipt{}, bad
	}
	f, openErr := root.Open(name)
	if openErr != nil {
		return recallReceipt{}, bad
	}
	// G104: Cleanup preserves the original setup/operation error.
	defer f.Close()
	data, openErr := io.ReadAll(io.LimitReader(f, recallReceiptReadLimit))
	if openErr != nil {
		return recallReceipt{}, bad
	}
	var out recallReceipt
	schema, openErr := contracts.Compile(operationProbeRecall, "output")
	var raw any
	if openErr != nil || json.Unmarshal(data, &raw) != nil || schema.Validate(raw) != nil ||
		json.Unmarshal(data, &out) != nil {
		return recallReceipt{}, bad
	}
	return out, nil
}

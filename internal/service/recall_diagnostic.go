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
		_ = json.NewEncoder(w).Encode(&domain.Error{Code: "RECALL_DIAGNOSTIC_UNAVAILABLE", Message: "Local recall diagnostic is unavailable.", NextAction: domain.NextAction{Instruction: "Read the diagnostic receipt or collector status; never redispatch an ambiguous recall."}, Details: map[string]any{}})
	}
	schema, err := contracts.Compile("cli_probe_recall", "input")
	var raw any
	if err != nil || r.Method != "POST" || r.URL.Path != "/rpc" || !uniqueLedgerJSON(body) || json.Unmarshal(body, &raw) != nil || schema.Validate(raw) != nil {
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
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
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

func (p *membershipPort) probeRecall(ctx context.Context, id string) (recallReceipt, error) {
	bad := errors.New("recall diagnostic unavailable")
	empty := recallReceipt{}
	normalized, e := uuid.Parse(id)
	recaller, ok := p.sender.(diagnosticRecaller)
	if e != nil || normalized.String() != id || !ok || p.store == nil || ctx.Err() != nil || !filepath.IsAbs(p.stateDir) {
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
	defer parent.Close()
	if e = parent.Mkdir("diagnostic-recalls", 0700); e != nil && !errors.Is(e, os.ErrExist) {
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
	defer root.Close()
	digest := sha256.Sum256([]byte(own + "\x00" + id))
	name := hex.EncodeToString(digest[:]) + ".receipt"
	if info, e := root.Lstat(name); e == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 512 {
			return empty, bad
		}
		f, e := root.Open(name)
		if e != nil {
			return empty, bad
		}
		defer f.Close()
		data, e := io.ReadAll(io.LimitReader(f, 513))
		if e != nil {
			return empty, bad
		}
		var out recallReceipt
		schema, e := contracts.Compile("cli_probe_recall", "output")
		var raw any
		if e != nil || json.Unmarshal(data, &raw) != nil || schema.Validate(raw) != nil || json.Unmarshal(data, &out) != nil {
			return empty, bad
		}
		return out, nil
	} else if !errors.Is(e, os.ErrNotExist) {
		return empty, bad
	}
	if time.Since(op.UpdatedAt) > 30*time.Minute || time.Until(op.UpdatedAt) > time.Minute || !canonicalDiagnosticID(op.RecipientID) || !canonicalDiagnosticID(*op.MessageID) {
		return empty, bad
	}
	quote, e := p.store.SendQuote(ctx, op.RecipientID, *op.MessageID)
	if e != nil || quote.SenderID != own || quote.Text != recallDiagnosticText || !canonicalDiagnosticID(quote.Metadata.ClientMessageID) {
		return empty, bad
	}
	dir, e := root.Open(".")
	if e != nil {
		return empty, bad
	}
	entries, e := dir.ReadDir(21)
	dir.Close()
	if e != nil && e != io.EOF || len(entries) >= 20 {
		return empty, bad
	}
	f, e := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return empty, bad
	}
	defer f.Close()
	write := func(out recallReceipt) error {
		b, e := json.Marshal(out)
		if e != nil {
			return e
		}
		if e = f.Truncate(0); e != nil {
			return e
		}
		if _, e = f.WriteAt(b, 0); e != nil {
			return e
		}
		return f.Sync()
	}
	out := recallReceipt{State: "reserved"}
	if write(out) != nil {
		return empty, bad
	}
	dir, e = root.Open(".")
	if e != nil {
		return empty, bad
	}
	e = dir.Sync()
	dir.Close()
	if e != nil {
		return empty, bad
	}
	if ctx.Err() != nil {
		return out, nil
	}
	status, e := recaller.UndoDiagnosticDirect(ctx, op.RecipientID, *op.MessageID, quote.Metadata.ClientMessageID)
	out.State = "unknown"
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

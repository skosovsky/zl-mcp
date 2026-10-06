package collector

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type ListenerUpstream interface {
	Upstream
	Listen(context.Context, func(domain.Message) error, func(string, string) error, func() error) error
}

const listenerUnavailableMessage = "Listener unavailable; reconnecting."

// storageFailure preserves the cause across the listener boundary without exposing SQL.
type storageFailure struct{ cause error }

func (e *storageFailure) Error() string { return "collector storage failed" }
func (e *storageFailure) Unwrap() error { return e.cause }

// RunInternal runs the collector inside a service which owns storage and account lock.
// It opens no control socket; ready supplies the in-process membership port.
func RunInternal(ctx context.Context, c config.Config, store *storage.Store, client ListenerUpstream, ready func(*JoinManager) error) error {
	if ready == nil {
		return errors.New("collector readiness callback is required")
	}
	if err := store.BindAccount(ctx, client.AccountID()); err != nil {
		return err
	}
	guard := newSessionGuard(ctx, client)
	defer guard.cancel()
	return runSession(ctx, c, store, guard, ready)
}

func runSession(ctx context.Context, c config.Config, store *storage.Store, client ListenerUpstream, ready func(*JoinManager) error) (resultErr error) {
	var e error
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if source, ok := client.(domain.HistorySource); ok {
		if err := store.RecoverInterruptedHistory(runCtx); err != nil {
			if runCtx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				return nil
			}
			return &storageFailure{cause: err}
		}
		done := make(chan error, 1)
		go func() {
			err := historyimport.RunRecovered(runCtx, store, source)
			done <- err
			if err != nil {
				cancel()
			}
		}()
		defer func() {
			cancel()
			err := <-done
			if resultErr == nil && err != nil {
				if errors.Is(err, domain.ErrAuthenticationRequired) {
					resultErr = err
				} else {
					resultErr = &storageFailure{cause: err}
				}
			}
		}()
	}
	joins := NewJoin(runCtx, store, client, c.Permissions.AllowJoin)
	if e = joins.Recover(ctx); e != nil {
		return e
	}
	defer func() { cancel(); joins.Wait() }()
	if ready != nil {
		if e = ready(joins); e != nil {
			return e
		}
	}
	state := map[string]any{"authenticated": true, "collector_state": "connecting", "last_connected_at": nil, "last_event_at": nil, "last_persisted_at": nil, "last_error": nil}
	var mu sync.Mutex
	set := func(key string, value any) error {
		mu.Lock()
		defer mu.Unlock()
		state[key] = value
		return store.SetState(runCtx, state)
	}
	refresh := func() error {
		q, cancel := context.WithTimeout(runCtx, 30*time.Second)
		defer cancel()
		if e := joins.enter(q); e != nil {
			return e
		}
		defer joins.leave()
		groups, e := client.Groups(q)
		if e != nil {
			return e
		}
		if err := store.ReplaceCatalog(q, groups); err != nil {
			return err
		}
		return joins.Reconcile(q, groups)
	}
	if e = store.BeginGap(runCtx, "collector_start_or_restart"); e != nil {
		return e
	}
	if e = refresh(); e != nil {
		_ = set("last_error", map[string]any{"code": "UPSTREAM_UNAVAILABLE", "message": "Group catalog refresh failed."})
	}
	if source, ok := client.(domain.ConversationPreloadSource); ok {
		done := make(chan struct{})
		go func() { defer close(done); preloadCatalogLoop(runCtx, store, source) }()
		defer func() { cancel(); <-done }()
	}
	if source, ok := client.(domain.ContactSource); ok {
		done := make(chan struct{})
		go func() { defer close(done); contactsLoop(runCtx, store, source) }()
		defer func() { cancel(); <-done }()
	}
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		refreshTicker := time.NewTicker(5 * time.Minute)
		defer refreshTicker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
				slog.Info("zalo_persistence_diagnostics", "counts", store.IngestionDiagnostics())
				mu.Lock()
				_ = store.SetState(runCtx, state)
				mu.Unlock()
			case <-refreshTicker.C:
				_ = refresh()
				_ = store.Retain(runCtx)
			}
		}
	}()
	defer func() {
		cancel()
		<-heartbeatDone
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = store.BeginGap(shutdown, "collector_stopped")
		mu.Lock()
		if state["collector_state"] != "auth_required" {
			state["collector_state"] = "stopped"
		}
		_ = store.SetState(shutdown, state)
		mu.Unlock()
	}()
	delay := time.Second
	for {
		_ = set("collector_state", "connecting")
		e = listenConversations(client, runCtx, func(m domain.Message) error {
			_ = set("last_event_at", time.Now().UTC().Format(time.RFC3339Nano))
			err := store.Put(runCtx, m)
			if err != nil {
				_ = set("last_error", map[string]any{"code": "STORAGE_ERROR", "message": "A message could not be persisted."})
				_ = store.BeginGap(runCtx, "storage_error")
				return &storageFailure{cause: err}
			}
			if store.AllowsConversation(m.Ref()) {
				return set("last_persisted_at", time.Now().UTC().Format(time.RFC3339Nano))
			}
			return nil
		}, func(ref domain.ConversationRef, id string) error {
			if !store.AllowsConversation(ref) {
				return nil
			}
			if err := store.DeleteConversation(runCtx, ref, id); err != nil {
				_ = set("last_error", map[string]any{"code": "STORAGE_ERROR", "message": "A deletion could not be persisted."})
				_ = store.BeginGap(runCtx, "storage_error")
				return &storageFailure{cause: err}
			}
			return nil
		}, func() error {
			delay = time.Second
			// Connection readiness cannot establish replay/history completeness.
			mu.Lock()
			defer mu.Unlock()
			state["last_connected_at"] = time.Now().UTC().Format(time.RFC3339Nano)
			state["collector_state"] = "connected"
			if last, ok := state["last_error"].(map[string]any); ok && last["code"] == "UPSTREAM_UNAVAILABLE" && last["message"] == listenerUnavailableMessage {
				state["last_error"] = nil
			}
			return store.SetState(runCtx, state)
		})
		if runCtx.Err() != nil {
			return nil
		}
		if errors.Is(e, domain.ErrAuthenticationRequired) {
			_ = store.BeginGap(runCtx, "auth_required")
			mu.Lock()
			state["authenticated"] = false
			state["collector_state"] = "auth_required"
			state["last_error"] = map[string]any{"code": "NOT_AUTHENTICATED", "message": "Run local login."}
			saveErr := store.SetState(runCtx, state)
			mu.Unlock()
			if saveErr != nil {
				return saveErr
			}
			return domain.ErrAuthenticationRequired
		}
		_ = store.BeginGap(runCtx, "listener_disconnected")
		_ = set("collector_state", "reconnecting")
		var storageErr *storageFailure
		if !errors.As(e, &storageErr) {
			_ = set("last_error", map[string]any{"code": "UPSTREAM_UNAVAILABLE", "message": listenerUnavailableMessage})
		}
		jitter, _ := rand.Int(rand.Reader, big.NewInt(int64(delay/2)+1))
		wait := delay
		if jitter != nil {
			wait += time.Duration(jitter.Int64())
		}
		timer := time.NewTimer(wait)
		select {
		case <-runCtx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		delay = min(delay*2, 40*time.Second)
	}
}

// ControlHandler serves the contract-validated local collector protocol.
// It is shared by the live collector and the offline model evaluation fixture.
func ControlHandler(j *JoinManager) http.Handler {
	_, compileErr := contracts.Compile("control", "input")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fail := func(err error) {
			de := &domain.Error{Code: "UPSTREAM_UNAVAILABLE", Message: "Collector request failed.", NextAction: domain.NextAction{Instruction: "Check collector status and retry reads later."}, Details: map[string]any{}}
			var typed *domain.Error
			if errors.As(err, &typed) {
				de = typed
			}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(de)
		}
		if r.Method != "POST" || r.URL.Path != "/rpc" {
			fail(domain.Invalid("Use POST /rpc."))
			return
		}
		if compileErr != nil {
			fail(compileErr)
			return
		}
		b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
		if e != nil {
			fail(domain.Invalid("Control request is too large."))
			return
		}
		var v struct {
			Method    string         `json:"method"`
			Arguments map[string]any `json:"arguments"`
		}
		if json.Unmarshal(b, &v) != nil {
			fail(domain.Invalid("Invalid control request."))
			return
		}
		// CLI-only operations are never registered as MCP tools.
		if v.Method == "cli_probe_preload" {
			input, schemaErr := contracts.Compile(v.Method, "input")
			var raw any
			if schemaErr != nil || json.Unmarshal(b, &raw) != nil || input.Validate(raw) != nil {
				fail(domain.Invalid("Probe request does not match its contract."))
				return
			}
			source, _ := j.API.(domain.ConversationPreloadSource)
			result := probePreload(r.Context(), source)
			output, schemaErr := contracts.Compile(v.Method, "output")
			if schemaErr != nil || output.Validate(result) != nil {
				fail(domain.Invalid("Probe result does not match its contract."))
				return
			}
			json.NewEncoder(w).Encode(result)
			return
		}
		if v.Method == "cli_preview" || v.Method == "cli_approve" {
			cliSchema, schemaErr := contracts.Compile(v.Method, "input")
			var rawCLI any
			if schemaErr != nil || json.Unmarshal(b, &rawCLI) != nil || cliSchema.Validate(rawCLI) != nil {
				fail(domain.Invalid("CLI request does not match its contract."))
				return
			}
			if len(v.Arguments) != 1 {
				fail(domain.Invalid("preview_id is required"))
				return
			}
			id, ok := v.Arguments["preview_id"].(string)
			if !ok || id == "" {
				fail(domain.Invalid("preview_id is required"))
				return
			}
			if v.Method == "cli_preview" {
				p, e := j.Preview(r.Context(), id)
				if e != nil {
					fail(e)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"group_id": p.GroupID, "name": p.Name, "approval_required": p.Approval})
				return
			}
			token, e := j.Approve(r.Context(), id)
			if e != nil {
				fail(e)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"plan_token": token})
			return
		}
		result, e := j.Call(r.Context(), v.Method, v.Arguments)
		if e != nil {
			fail(e)
			return
		}
		json.NewEncoder(w).Encode(result)
	})
}

// Call exposes only contract-validated membership operations to the MCP service.
// Trusted CLI approval routes remain confined to ControlHandler.
func (j *JoinManager) Call(parent context.Context, method string, arguments any) (map[string]any, error) {
	schema, err := contracts.Compile("control", "input")
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(map[string]any{"method": method, "arguments": arguments})
	if err != nil {
		return nil, domain.Invalid("Invalid membership arguments.")
	}
	var request map[string]any
	if json.Unmarshal(b, &request) != nil || schema.Validate(request) != nil {
		return nil, domain.Invalid("Arguments do not match control schema.")
	}
	if status, ok := j.API.(interface{ AuthenticationRequired() bool }); ok && status.AuthenticationRequired() && method != "zalo_get_join_status" {
		return nil, safeError("NOT_AUTHENTICATED", "Zalo authentication is required.", "Stop service and run local login.")
	}
	args := request["arguments"].(map[string]any)
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var result map[string]any
	var e error
	switch method {
	case "zalo_get_group":
		if e = j.enter(ctx); e != nil {
			return nil, e
		}
		g, d, err := j.API.Group(ctx, args["group_id"].(string))
		j.leave()
		e = err
		if e == nil {
			g.CollectionEnabled = j.Store.Allowed(g.ID)
			e = j.Store.UpsertGroup(ctx, g, d)
			if e == nil {
				result, e = j.Store.Group(ctx, g.ID)
			}
		}
	case "zalo_inspect_invite":
		if e = j.enter(ctx); e != nil {
			return nil, e
		}
		result, e = j.Inspect(ctx, args["invite_url"].(string))
		j.leave()
	case "zalo_join_group":
		result, e = j.Start(ctx, args["plan_token"].(string), args["request_id"].(string))
	case "zalo_get_join_status":
		result, e = j.Operation(ctx, args["operation_id"].(string))
	}
	if errors.Is(e, domain.ErrAuthenticationRequired) {
		return nil, safeError("NOT_AUTHENTICATED", "Zalo authentication is required.", "Stop service and run local login.")
	}
	return result, e
}

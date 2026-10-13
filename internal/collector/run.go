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

// storageError preserves the cause across the listener boundary without exposing SQL.
type storageError struct{ cause error }

func (e *storageError) Error() string { return "collector storage failed" }
func (e *storageError) Unwrap() error { return e.cause }

// RunInternal runs the collector inside a service which owns storage and account lock.
// It opens no control socket; ready supplies the in-process membership port.
func RunInternal(
	ctx context.Context,
	c config.Config,
	store *storage.Store,
	client ListenerUpstream,
	ready func(*JoinManager) error,
) error {
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

//nolint:funlen // The session owner registers cancel-and-join cleanup for history, joins, preload, contacts and heartbeat in their required reverse order; work loops are separate helpers.
func runSession(
	ctx context.Context,
	c config.Config,
	store *storage.Store,
	client ListenerUpstream,
	ready func(*JoinManager) error,
) (resultErr error) {
	var e error
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if source, ok := client.(domain.HistorySource); ok {
		recovered, recoveryErr := recoverCollectorHistory(runCtx, store)
		if recoveryErr != nil || !recovered {
			return recoveryErr
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
			workerErr := historyWorkerResult(<-done)
			if resultErr == nil {
				resultErr = workerErr
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
	values := map[string]any{
		"authenticated":     true,
		"collector_state":   "connecting",
		"last_connected_at": nil,
		"last_event_at":     nil,
		"last_persisted_at": nil,
		"last_error":        nil,
	}
	state := &collectorState{ctx: runCtx, store: store, values: values, mu: sync.Mutex{}}
	set := state.set
	refresh := func() error { return refreshCollectorGroups(runCtx, joins, store, client) }
	if e = store.BeginGap(runCtx, "collector_start_or_restart"); e != nil {
		return e
	}
	if e = refresh(); e != nil {
		_ = set(
			"last_error",
			map[string]any{"code": "UPSTREAM_UNAVAILABLE", "message": "Group catalog refresh failed."},
		)
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
	go collectorHeartbeat(ctx, runCtx, store, state, refresh, heartbeatDone)
	defer func() {
		cancel()
		<-heartbeatDone
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = store.BeginGap(shutdown, "collector_stopped")
		_ = state.stopped(shutdown)
	}()
	return listenCollector(runCtx, client, store, state)
}

// ControlHandler serves the contract-validated local collector protocol.
// It is shared by the live collector and the offline model evaluation fixture.
func ControlHandler(j *JoinManager) http.Handler {
	_, compileErr := contracts.Compile("control", "input")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fail := func(err error) {
			writeControlFailure(w, err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/rpc" {
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
		var v controlRequest
		if json.Unmarshal(b, &v) != nil {
			fail(domain.Invalid("Invalid control request."))
			return
		}
		// CLI-only operations are never registered as MCP tools.
		var result map[string]any
		switch v.Method {
		case "cli_probe_preload":
			result, e = controlPreloadResult(r.Context(), j, b)
		case "cli_preview", "cli_approve":
			result, e = controlApprovalResult(r.Context(), j, v, b)
		default:
			result, e = j.Call(r.Context(), v.Method, v.Arguments)
		}
		if e != nil {
			fail(e)
			return
		}
		if json.NewEncoder(w).Encode(result) != nil {
			return // A partial HTTP response must not be retried.
		}
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
	if status, ok := j.API.(interface{ AuthenticationRequired() bool }); ok && status.AuthenticationRequired() &&
		method != "zalo_get_join_status" {
		return nil, safeError(
			"NOT_AUTHENTICATED",
			"Zalo authentication is required.",
			"Stop service and run local login.",
		)
	}
	var decoded struct {
		Arguments struct {
			GroupID     string `json:"group_id"`
			InviteURL   string `json:"invite_url"`
			PlanToken   string `json:"plan_token"`
			RequestID   string `json:"request_id"`
			OperationID string `json:"operation_id"`
		} `json:"arguments"`
	}
	if json.Unmarshal(b, &decoded) != nil {
		return nil, domain.Invalid("Invalid membership arguments.")
	}
	args := decoded.Arguments
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	var result map[string]any
	var e error
	switch method {
	case "zalo_get_group":
		if e = j.enter(ctx); e != nil {
			return nil, e
		}
		g, d, err := j.API.Group(ctx, args.GroupID)
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
		result, e = j.Inspect(ctx, args.InviteURL)
		j.leave()
	case "zalo_join_group":
		result, e = j.Start(ctx, args.PlanToken, args.RequestID)
	case "zalo_get_join_status":
		result, e = j.Operation(ctx, args.OperationID)
	}
	if errors.Is(e, domain.ErrAuthenticationRequired) {
		return nil, safeError(
			"NOT_AUTHENTICATED",
			"Zalo authentication is required.",
			"Stop service and run local login.",
		)
	}
	return result, e
}

type controlRequest struct {
	Method    string         `json:"method"`
	Arguments map[string]any `json:"arguments"`
}

func controlPreloadResult(ctx context.Context, j *JoinManager, body []byte) (map[string]any, error) {
	input, err := contracts.Compile("cli_probe_preload", "input")
	var raw any
	if err != nil || json.Unmarshal(body, &raw) != nil || input.Validate(raw) != nil {
		return nil, domain.Invalid("Probe request does not match its contract.")
	}
	source, _ := j.API.(domain.ConversationPreloadSource)
	result := probePreload(ctx, source)
	output, err := contracts.Compile("cli_probe_preload", "output")
	if err != nil || output.Validate(result) != nil {
		return nil, domain.Invalid("Probe result does not match its contract.")
	}
	return result, nil
}
func controlApprovalResult(ctx context.Context, j *JoinManager, request controlRequest, body []byte) (map[string]any, error) {
	schema, err := contracts.Compile(request.Method, "input")
	var raw any
	if err != nil || json.Unmarshal(body, &raw) != nil || schema.Validate(raw) != nil {
		return nil, domain.Invalid("CLI request does not match its contract.")
	}
	if len(request.Arguments) != 1 {
		return nil, domain.Invalid("preview_id is required")
	}
	id, ok := request.Arguments["preview_id"].(string)
	if !ok || id == "" {
		return nil, domain.Invalid("preview_id is required")
	}
	if request.Method == "cli_preview" {
		previewValue, previewErr := j.preview(ctx, id)
		if previewErr != nil {
			return nil, previewErr
		}
		return map[string]any{"group_id": previewValue.GroupID, "name": previewValue.Name, "approval_required": previewValue.Approval}, nil
	}
	token, err := j.Approve(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"plan_token": token}, nil
}

func (state *collectorState) message(m domain.Message) error {

	_ = state.set("last_event_at", time.Now().UTC().Format(time.RFC3339Nano))
	err := state.store.Put(state.ctx, m)
	if err != nil {
		_ = state.set(
			"last_error",
			map[string]any{"code": "STORAGE_ERROR", "message": "A message could not be persisted."},
		)
		_ = state.store.BeginGap(state.ctx, "storage_error")
		return &storageError{cause: err}
	}
	if state.store.AllowsConversation(m.Ref()) {
		return state.set("last_persisted_at", time.Now().UTC().Format(time.RFC3339Nano))
	}
	return nil

}

func (state *collectorState) deleted(ref domain.ConversationRef, id string) error {

	if !state.store.AllowsConversation(ref) {
		return nil
	}
	if err := state.store.DeleteConversation(state.ctx, ref, id); err != nil {
		_ = state.set(
			"last_error",
			map[string]any{"code": "STORAGE_ERROR", "message": "A deletion could not be persisted."},
		)
		_ = state.store.BeginGap(state.ctx, "storage_error")
		return &storageError{cause: err}
	}
	return nil

}

func (state *collectorState) connected() error {

	// Connection readiness cannot establish replay/history completeness.
	state.mu.Lock()
	defer state.mu.Unlock()
	state.values["last_connected_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	state.values["collector_state"] = "connected"
	if last, ok := state.values["last_error"].(map[string]any); ok && last["code"] == "UPSTREAM_UNAVAILABLE" &&
		last["message"] == listenerUnavailableMessage {
		state.values["last_error"] = nil
	}
	return state.store.SetState(state.ctx, state.values)
}

type collectorState struct {
	ctx    context.Context
	store  *storage.Store
	mu     sync.Mutex
	values map[string]any
}

func (state *collectorState) set(key string, value any) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.values[key] = value
	return state.store.SetState(state.ctx, state.values)
}
func (state *collectorState) save(ctx context.Context) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.store.SetState(ctx, state.values)
}
func (state *collectorState) stopped(ctx context.Context) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.values["collector_state"] != "auth_required" {
		state.values["collector_state"] = "stopped"
	}
	return state.store.SetState(ctx, state.values)
}
func (state *collectorState) authenticationRequired() error {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.values["authenticated"] = false
	state.values["collector_state"] = "auth_required"
	state.values["last_error"] = map[string]any{"code": "NOT_AUTHENTICATED", "message": "Run local login."}
	return state.store.SetState(state.ctx, state.values)
}
func collectorHeartbeat(ctx, runCtx context.Context, store *storage.Store, state *collectorState, refresh func() error, heartbeatDone chan struct{}) {
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
			slog.InfoContext(ctx, "zalo_persistence_diagnostics", "counts", store.IngestionDiagnostics())
			_ = state.save(runCtx)
		case <-refreshTicker.C:
			_ = refresh()
			_ = store.Retain(runCtx)
		}
	}
}
func listenCollector(runCtx context.Context, client ListenerUpstream, store *storage.Store, state *collectorState) error {
	var e error
	set := state.set
	delay := time.Second
	for {
		_ = set("collector_state", "connecting")
		e = listenConversations(client, runCtx, state.message, state.deleted, func() error {
			delay = time.Second
			return state.connected()
		})
		if runCtx.Err() != nil {
			//nolint:nilerr // Owner cancellation is normal collector shutdown; do not retry the listener.
			return nil
		}
		if errors.Is(e, domain.ErrAuthenticationRequired) {
			_ = store.BeginGap(runCtx, "auth_required")
			saveErr := state.authenticationRequired()
			if saveErr != nil {
				return saveErr
			}
			return domain.ErrAuthenticationRequired
		}
		_ = store.BeginGap(runCtx, "listener_disconnected")
		_ = set("collector_state", "reconnecting")
		if _, ok := errors.AsType[*storageError](e); !ok {
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

func writeControlFailure(w http.ResponseWriter, err error) {
	de := &domain.Error{
		Code:       "UPSTREAM_UNAVAILABLE",
		Message:    "Collector request failed.",
		NextAction: domain.NextAction{Instruction: "Check collector status and retry reads later."},
		Details:    map[string]any{},
	}
	if typed, ok := errors.AsType[*domain.Error](err); ok {
		de = typed
	}
	w.WriteHeader(http.StatusBadRequest)
	if json.NewEncoder(w).Encode(de) != nil {
		return // A partial HTTP response must not be retried.
	}

}

func recoverCollectorHistory(ctx context.Context, store *storage.Store) (bool, error) {
	if err := store.RecoverInterruptedHistory(ctx); err != nil {
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			return false, nil
		}
		return false, &storageError{cause: err}
	}
	return true, nil
}

func historyWorkerResult(err error) error {
	if err == nil || errors.Is(err, domain.ErrAuthenticationRequired) {
		return err
	}
	return &storageError{cause: err}
}

func refreshCollectorGroups(runCtx context.Context, joins *JoinManager, store *storage.Store, client ListenerUpstream) error {
	q, withTimeoutCancel := context.WithTimeout(runCtx, 30*time.Second)
	defer withTimeoutCancel()
	if enterErr := joins.enter(q); enterErr != nil {
		return enterErr
	}
	defer joins.leave()
	groups, groupsErr := client.Groups(q)
	if groupsErr != nil {
		return groupsErr
	}
	if err := store.ReplaceCatalog(q, groups); err != nil {
		return err
	}
	return joins.Reconcile(q, groups)

}

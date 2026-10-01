package collector

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/zalo"
)

type listenerUpstream interface {
	Upstream
	Listen(context.Context, func(domain.Message) error, func(string, string) error, func() error) error
}

// storageFailure preserves the cause across the listener boundary without exposing SQL.
type storageFailure struct{ cause error }

func (e *storageFailure) Error() string { return "collector storage failed" }
func (e *storageFailure) Unwrap() error { return e.cause }

func Run(ctx context.Context, c config.Config) error {
	return runWithRestore(ctx, c, zalo.Restore)
}

func runWithRestore(ctx context.Context, c config.Config, restore func(context.Context, string) (*zalo.Client, error)) error {
	if _, err := local.SocketPath(c.StateDir); err != nil {
		return err
	}
	unlock, e := local.Lock(c.StateDir)
	if e != nil {
		return e
	}
	defer unlock()
	store, e := storage.Open(ctx, filepath.Join(c.StateDir, "messages.sqlite"), c.Collection.GroupIDs, c.Storage.RetentionDays)
	if e != nil {
		return e
	}
	defer store.Close()
	client, e := restore(ctx, c.StateDir)
	if e != nil {
		state, code, message := "stopped", "UPSTREAM_UNAVAILABLE", "Saved session could not be verified; check connectivity and retry collection."
		if errors.Is(e, domain.ErrAuthenticationRequired) {
			state, code, message = "auth_required", "NOT_AUTHENTICATED", "Run local login."
		} else {
			var typed *domain.Error
			if errors.As(e, &typed) && typed.Code == "STORAGE_ERROR" {
				code, message = "STORAGE_ERROR", "Cannot read private saved session."
			}
		}
		if err := store.SetState(ctx, map[string]any{"authenticated": false, "collector_state": state, "last_connected_at": nil, "last_event_at": nil, "last_persisted_at": nil, "last_error": map[string]any{"code": code, "message": message}}); err != nil {
			return err
		}
		return e
	}
	if e = store.BindAccount(ctx, client.AccountID()); e != nil {
		return e
	}
	if e = client.Save(c.StateDir); e != nil {
		return e
	}
	return runSession(ctx, c, store, client)
}

// runSession owns the actual listener/control lifecycle; ports keep it testable offline.
func runSession(ctx context.Context, c config.Config, store *storage.Store, client listenerUpstream) error {
	var e error
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	joins := NewJoin(runCtx, store, client, c.Permissions.AllowJoin)
	if e = joins.Recover(ctx); e != nil {
		return e
	}
	defer joins.Wait()
	sock, e := local.SocketPath(c.StateDir)
	if e != nil {
		return e
	}
	_ = os.Remove(sock)
	ln, e := net.Listen("unix", sock)
	if e != nil {
		return e
	}
	defer os.Remove(sock)
	if e = os.Chmod(sock, 0600); e != nil {
		ln.Close()
		return e
	}
	// Connection operations are serialized with membership mutations.
	server := &http.Server{Handler: ControlHandler(joins), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second}
	errs := make(chan error, 1)
	go func() {
		e := server.Serve(ln)
		if e != nil && !errors.Is(e, http.ErrServerClosed) {
			errs <- e
			cancel()
		}
	}()
	defer func() {
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		server.Shutdown(shutdown)
	}()
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
		e = client.Listen(runCtx, func(m domain.Message) error {
			_ = set("last_event_at", time.Now().UTC().Format(time.RFC3339Nano))
			err := store.Put(runCtx, m)
			if err != nil {
				_ = set("last_error", map[string]any{"code": "STORAGE_ERROR", "message": "A message could not be persisted."})
				_ = store.BeginGap(runCtx, "storage_error")
				return &storageFailure{cause: err}
			}
			if store.Allowed(m.GroupID) {
				return set("last_persisted_at", time.Now().UTC().Format(time.RFC3339Nano))
			}
			return nil
		}, func(g, id string) error {
			if !store.Allowed(g) {
				return nil
			}
			if err := store.Delete(runCtx, g, id); err != nil {
				_ = set("last_error", map[string]any{"code": "STORAGE_ERROR", "message": "A deletion could not be persisted."})
				_ = store.BeginGap(runCtx, "storage_error")
				return &storageFailure{cause: err}
			}
			return nil
		}, func() error {
			delay = time.Second
			if e := store.EndGaps(runCtx); e != nil {
				return e
			}
			if e := set("last_connected_at", time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
				return e
			}
			return set("collector_state", "connected")
		})
		select {
		case err := <-errs:
			return err
		default:
		}
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
			_ = set("last_error", map[string]any{"code": "UPSTREAM_UNAVAILABLE", "message": "Listener unavailable; reconnecting."})
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
	schema, compileErr := contracts.Compile("control", "input")
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
		var data any
		if json.Unmarshal(b, &data) != nil || schema.Validate(data) != nil {
			fail(domain.Invalid("Arguments do not match control schema."))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var result map[string]any
		switch v.Method {
		case "zalo_get_group":
			if e = j.enter(ctx); e != nil {
				fail(e)
				return
			}
			g, d, err := j.API.Group(ctx, v.Arguments["group_id"].(string))
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
				fail(e)
				return
			}
			result, e = j.Inspect(ctx, v.Arguments["invite_url"].(string))
			j.leave()
		case "zalo_join_group":
			result, e = j.Start(ctx, v.Arguments["plan_token"].(string), v.Arguments["request_id"].(string))
		case "zalo_get_join_status":
			result, e = j.Operation(ctx, v.Arguments["operation_id"].(string))
		}
		if e != nil {
			fail(e)
			return
		}
		json.NewEncoder(w).Encode(result)
	})
}

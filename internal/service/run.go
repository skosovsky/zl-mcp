// Package service owns the single persistent process and its component lifetimes.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/skosovsky/zl-mcp/internal/logging"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/messaging"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/zalo"
)

type restoreFunc func(context.Context, string) (collector.ListenerUpstream, error)

func Run(ctx context.Context, c config.Config) error {
	return run(ctx, c, func(ctx context.Context, dir string) (collector.ListenerUpstream, error) {
		client, err := zalo.Restore(ctx, dir)
		if err != nil {
			return nil, err
		}
		return client, nil
	}, func(client collector.ListenerUpstream) error { return client.(*zalo.Client).Save(c.StateDir) }, nil)
}

// membershipPort blocks network operations while the collector lacks a session.
// Reading a saved operation needs no upstream client.
type membershipPort struct {
	stateDir   string
	mu         sync.RWMutex
	current    *collector.JoinManager
	store      *storage.Store
	sender     messaging.Sender
	allowSend  bool
	recipients map[string]bool
	lifecycle  context.Context
	snapshots  *mobilebackup.SnapshotStore
	archives   *mobilebackup.RetainedArchiveStore
	library    *mobilebackup.RetainedArchiveStore
}

func (p *membershipPort) set(j *collector.JoinManager) { p.mu.Lock(); p.current = j; p.mu.Unlock() }
func (p *membershipPort) setSender(s messaging.Sender) { p.mu.Lock(); p.sender = s; p.mu.Unlock() }
func (p *membershipPort) Call(ctx context.Context, method string, args any) (map[string]any, error) {
	// SDK request contexts can outlive the originating HTTP request. The domain
	// port must also follow service shutdown before it acquires session ownership.
	if p.lifecycle != nil {
		operation, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(p.lifecycle, cancel)
		defer stop()
		defer cancel()
		if p.lifecycle.Err() != nil {
			cancel()
		}
		ctx = operation
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if method == "zalo_import_conversation_history" || method == "zalo_get_history_import_status" || method == "zalo_cancel_history_import" {
		return (&historyimport.Manager{Store: p.store}).Call(ctx, method, args)
	}
	if method == "zalo_send_direct_message" || method == "zalo_get_send_status" {
		return (&messaging.Manager{Store: p.store, Sender: p.sender, Enabled: p.allowSend, Recipients: p.recipients}).Call(ctx, method, args)
	}
	if p.current != nil {
		return p.current.Call(ctx, method, args)
	}
	if method == "zalo_get_join_status" {
		return collector.NewJoin(ctx, p.store, nil, false).Call(ctx, method, args)
	}
	return nil, &domain.Error{Code: "NOT_AUTHENTICATED", Message: "Zalo session is unavailable.", NextAction: domain.NextAction{Instruction: "Check status; when auth_required, stop service and run local login."}, Details: map[string]any{}}
}
func (p *membershipPort) cliHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
		var request struct {
			Method string `json:"method"`
		}
		if err != nil || json.Unmarshal(b, &request) != nil || (request.Method != "cli_preview" && request.Method != "cli_approve" && request.Method != "cli_probe_preload" && !mobileLedgerMethod(request.Method) && request.Method != "cli_probe_recall") {
			http.Error(w, "Only trusted CLI routes are available.", http.StatusBadRequest)
			return
		}
		if request.Method == "cli_probe_recall" {
			p.recallControl(w, r, b)
			return
		}
		if mobileLedgerMethod(request.Method) {
			p.mobileLedgerControl(w, r, b, request.Method)
			return
		}
		p.mu.RLock()
		defer p.mu.RUnlock()
		if p.current == nil {
			http.Error(w, "Session unavailable.", http.StatusServiceUnavailable)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(b))
		collector.ControlHandler(p.current).ServeHTTP(w, r)
	})
}

// ready exposes the bound endpoint only to integration tests, never credentials.
func run(parent context.Context, c config.Config, restore restoreFunc, save func(collector.ListenerUpstream) error, ready func(net.Addr)) error {
	return runConfigured(parent, c, restore, save, ready, nil)
}

// configure permits offline integration tests to inject a TLS receiver transport
// before goroutines start. Production Run always uses the validated HTTP client.
func runConfigured(parent context.Context, c config.Config, restore restoreFunc, save func(collector.ListenerUpstream) error, ready func(net.Addr), configure func(*mcpserver.HTTPService, *events.Worker)) (resultErr error) {
	unlock, err := local.Lock(c.StateDir)
	if err != nil {
		return err
	}
	defer unlock()
	var logFile *logging.File
	if c.Logging.File != "" {
		logFile, err = logging.Open(c.Logging.File, c.Logging.MaxSizeMB, c.Logging.MaxBackups, os.Stderr)
		if err != nil {
			return err
		}
		previous := slog.Default()
		level := slog.LevelInfo
		switch c.Logging.Level {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
		slog.SetDefault(slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: level})))
		defer func() {
			slog.SetDefault(previous)
			if err := logFile.Close(); resultErr == nil && err != nil {
				resultErr = err
			}
		}()
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if c.Collection.Mode == "" {
		slog.Warn("legacy_collection_config", "format", "group_ids", "next_action", "Use explicit selected mode with typed conversations to preserve the group-only scope; all mode widens collection.")
	}
	store, err := storage.OpenWithPolicy(ctx, filepath.Join(c.StateDir, "messages.sqlite"), c.Policy(), c.Storage.RetentionDays)
	if err != nil {
		return err
	}
	defer store.Close()
	if err = store.RecoverInterruptedSends(ctx); err != nil {
		return err
	}
	if err = store.RecoverMobileBackupAttempts(ctx); err != nil {
		return err
	}
	if err = store.Retain(ctx); err != nil {
		return err
	}
	token, err := tokenFile(c.MCP.TokenFile)
	if err != nil {
		return err
	}
	recipients := map[string]bool{}
	for _, id := range c.Permissions.SendRecipientIDs {
		recipients[id] = true
	}
	snapshots, err := mobilebackup.NewSnapshotStore(filepath.Join(c.StateDir, "mobile-snapshots"), 512<<20)
	if err != nil {
		return errors.New("private mobile snapshot store unavailable")
	}
	defer snapshots.Close()
	archives, err := mobilebackup.NewRetainedArchiveStore(filepath.Join(c.StateDir, "account-archives"), 1<<30)
	if err != nil {
		slog.Warn("account_archive_store_unavailable")
	}
	if archives != nil {
		defer archives.Close()
	}
	library, libraryErr := mobilebackup.NewArchiveLibraryStore(filepath.Join(c.StateDir, "account-archive-library", "sources"), 1<<30)
	if libraryErr != nil {
		slog.Warn("account_archive_library_unavailable")
	}
	if library != nil {
		defer library.Close()
	}
	port := &membershipPort{stateDir: c.StateDir, store: store, allowSend: c.Permissions.AllowSend, recipients: recipients, lifecycle: ctx, snapshots: snapshots, archives: archives, library: library}
	if err = port.CleanupHistorySnapshots(ctx); err != nil {
		return errors.New("private mobile snapshot cleanup unavailable")
	}
	handler, err := mcpserver.NewHTTPWithControl(store, c.StateDir, token, port)
	if err != nil {
		return err
	}
	worker, err := events.NewWorker(store)
	if err != nil {
		return err
	}
	var eventTrace *logging.File
	if c.Logging.EventTraceFile != "" {
		eventTrace, err = logging.Open(c.Logging.EventTraceFile, c.Logging.MaxSizeMB, c.Logging.MaxBackups, os.Stderr)
		if err != nil {
			return err
		}
		defer eventTrace.Close()
		worker.TraceLogger = slog.New(slog.NewJSONHandler(eventTrace, nil))
	}
	if configure != nil {
		configure(handler, worker)
	}
	listener, err := net.Listen("tcp", c.MCP.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	socket, err := local.SocketPath(c.StateDir)
	if err != nil {
		return err
	}
	if err = os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ipc, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer ipc.Close()
	defer os.Remove(socket)
	if err = os.Chmod(socket, 0600); err != nil {
		return err
	}
	// Allow the 30s upstream send plus independent result persistence to finish.
	// Shutdown cancels requests before waiting for session ports and closing SQLite.
	httpServer := &http.Server{Handler: handler, BaseContext: func(net.Listener) context.Context { return ctx }, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 30 * time.Second}
	// The owner-only mobile probe may wait 185s and finish its ledger independently.
	// Keep the Unix response deadline above the 190s CLI request budget.
	cliServer := &http.Server{Handler: port.cliHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 450 * time.Second}
	errorsCh := make(chan error, 7)
	var wg sync.WaitGroup
	launch := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := fn()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			errorsCh <- err
			cancel()
		}()
	}
	if logFile != nil {
		launch(func() error {
			select {
			case <-ctx.Done():
				return nil
			case <-logFile.Failed():
				return logFile.Err()
			}
		})
	}
	if eventTrace != nil {
		launch(func() error {
			select {
			case <-ctx.Done():
				return nil
			case <-eventTrace.Failed():
				return eventTrace.Err()
			}
		})
	}
	slog.Info("service started", "mcp_listen", listener.Addr().String())
	launch(func() error { return httpServer.Serve(listener) })
	launch(func() error { return cliServer.Serve(ipc) })
	launch(func() error { return worker.Run(ctx) })
	launch(func() error {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if err := store.Retain(ctx); err != nil {
					return err
				}
			}
		}
	})
	launch(func() error { return collect(ctx, c, store, port, restore, save) })
	if ready != nil {
		ready(listener.Addr())
	}
	<-ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	// Stop accepting requests before waiting for ports or closing shared SQLite.
	_ = httpServer.Shutdown(shutdown)
	_ = cliServer.Shutdown(shutdown)
	_ = httpServer.Close()
	_ = cliServer.Close()
	wg.Wait()
	slog.Info("service stopped")
	if logFile != nil && logFile.Err() != nil {
		return logFile.Err()
	}
	close(errorsCh)
	for err := range errorsCh {
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	return nil
}

func collect(ctx context.Context, c config.Config, store *storage.Store, port *membershipPort, restore restoreFunc, save func(collector.ListenerUpstream) error) error {
	return collectWithHeartbeat(ctx, c, store, port, restore, save, 10*time.Second)
}

func collectWithHeartbeat(parent context.Context, c config.Config, store *storage.Store, port *membershipPort, restore restoreFunc, save func(collector.ListenerUpstream) error, interval time.Duration) error {
	// Capture the previous process's liveness before writing a fresh heartbeat.
	if err := store.BeginGap(parent, "collector_start_or_restart"); err != nil {
		return err
	}
	if err := store.SetState(parent, map[string]any{"authenticated": false, "collector_state": "reconnecting", "last_connected_at": nil, "last_event_at": nil, "last_persisted_at": nil, "last_error": nil}); err != nil {
		return err
	}
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.TouchHeartbeat(ctx); err != nil {
					cancel(err)
					return
				}
			}
		}
	}()
	err := collectSession(ctx, c, store, port, restore, save)
	cancel(nil)
	<-done
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	return err
}

func collectSession(ctx context.Context, c config.Config, store *storage.Store, port *membershipPort, restore restoreFunc, save func(collector.ListenerUpstream) error) error {
	delay := time.Second
	for {
		restoreCtx, stop := context.WithTimeout(ctx, 30*time.Second)
		client, err := restore(restoreCtx, c.StateDir)
		stop()
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			if err = store.BindAccount(ctx, client.AccountID()); err != nil {
				return err
			}
			if err = save(client); err != nil {
				return err
			}
			sessionCtx, stopSession := context.WithCancel(ctx)
			var mobileDone chan error
			err = collector.RunInternal(sessionCtx, c, store, client, func(j *collector.JoinManager) error {
				port.set(j)
				if sender, ok := client.(messaging.Sender); ok {
					port.setSender(sender)
				}
				if port.snapshots != nil {
					mobileDone = make(chan error, 1)
					go func() {
						e := historyimport.RunMobile(sessionCtx, store, port)
						mobileDone <- e
						if e != nil {
							stopSession()
						}
					}()
				}
				return nil
			})
			stopSession()
			if mobileDone != nil {
				if mobileErr := <-mobileDone; mobileErr != nil && err == nil {
					if errors.Is(mobileErr, domain.ErrAuthenticationRequired) {
						err = mobileErr
					} else {
						err = &domain.Error{Code: "STORAGE_ERROR", Message: "Mobile history worker storage failed."}
					}
				}
			}
			port.setSender(nil)
			port.set(nil)
			if ctx.Err() != nil {
				return nil
			}
			if err == nil {
				return errors.New("collector stopped unexpectedly")
			}
			if !errors.Is(err, domain.ErrAuthenticationRequired) {
				return err
			}
		}
		if errors.Is(err, domain.ErrAuthenticationRequired) {
			if err = store.BeginGap(ctx, "auth_required"); err != nil {
				return err
			}
			if err = store.SetState(ctx, map[string]any{"authenticated": false, "collector_state": "auth_required", "last_connected_at": nil, "last_event_at": nil, "last_persisted_at": nil, "last_error": map[string]any{"code": "NOT_AUTHENTICATED", "message": "Stop service and run local login."}}); err != nil {
				return err
			}
			slog.Warn("Zalo authentication required; waiting for local login")
			<-ctx.Done()
			return nil
		}
		var typed *domain.Error
		if errors.As(err, &typed) && typed.Code == "STORAGE_ERROR" {
			if saveErr := store.SetState(ctx, map[string]any{"authenticated": false, "collector_state": "stopped", "last_connected_at": nil, "last_event_at": nil, "last_persisted_at": nil, "last_error": map[string]any{"code": "STORAGE_ERROR", "message": "Cannot read private saved session."}}); saveErr != nil {
				return saveErr
			}
			return err
		}
		if err = store.BeginGap(ctx, "session_restore_failed"); err != nil {
			return err
		}
		if err = store.SetState(ctx, map[string]any{"authenticated": false, "collector_state": "reconnecting", "last_connected_at": nil, "last_event_at": nil, "last_persisted_at": nil, "last_error": map[string]any{"code": "UPSTREAM_UNAVAILABLE", "message": "Saved session verification failed; retrying."}}); err != nil {
			return err
		}
		slog.Warn("Zalo session verification unavailable; retrying", "retry_seconds", delay.Seconds())
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		delay = min(delay*2, 40*time.Second)
	}
}

// Package service owns the single persistent process and its component lifetimes.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/skosovsky/zl-mcp/internal/logging"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/messaging"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/observability"
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
	}, func(client collector.ListenerUpstream) error {
		native, ok := client.(*zalo.Client)
		if !ok {
			return errors.New("unexpected restored client type")
		}
		return native.Save(c.StateDir)
	}, nil)
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
	if method == "zalo_list_archive_sources" || method == "zalo_list_conversations" ||
		method == "zalo_list_conversation_messages" ||
		method == "read_archive_resource" {
		return p.archiveRead(ctx, method, args)
	}
	if method == "zalo_import_conversation_history" || method == "zalo_get_history_import_status" ||
		method == "zalo_cancel_history_import" {
		return (&historyimport.Manager{Store: p.store}).Call(ctx, method, args)
	}
	if method == "zalo_send_direct_message" || method == "zalo_get_send_status" {
		return (&messaging.Manager{Store: p.store, Sender: p.sender, Enabled: p.allowSend, Recipients: p.recipients}).Call(
			ctx,
			method,
			args,
		)
	}
	if p.current != nil {
		return p.current.Call(ctx, method, args)
	}
	if method == "zalo_get_join_status" {
		return collector.NewJoin(ctx, p.store, nil, false).Call(ctx, method, args)
	}
	return nil, &domain.Error{
		Code:    "NOT_AUTHENTICATED",
		Message: "Zalo session is unavailable.",
		NextAction: domain.NextAction{
			Instruction: "Check status; when auth_required, stop service and run local login.",
		},
		Details: map[string]any{},
	}
}
func (p *membershipPort) cliHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
		var request struct {
			Method string `json:"method"`
		}
		if err != nil || json.Unmarshal(b, &request) != nil ||
			(request.Method != "cli_preview" && request.Method != "cli_approve" && request.Method != "cli_probe_preload" && !mobileLedgerMethod(request.Method) && request.Method != "cli_probe_recall") {
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
func run(
	parent context.Context,
	c config.Config,
	restore restoreFunc,
	save func(collector.ListenerUpstream) error,
	ready func(net.Addr),
) error {
	return runConfigured(parent, c, restore, save, ready, nil)
}

// configure permits offline integration tests to inject a TLS receiver transport
// before goroutines start. Production Run always uses the validated HTTP client.
//
//nolint:funlen,gocognit // The service owner registers logger restoration, store/exporter/archive/listener cleanup and keeps shared resources alive until all service loops join; endpoint and worker logic are separate helpers.
func runConfigured(
	parent context.Context,
	c config.Config,
	restore restoreFunc,
	save func(collector.ListenerUpstream) error,
	ready func(net.Addr),
	configure func(*mcpserver.HTTPService, *events.Worker),
) (resultErr error) {
	unlock, err := local.Lock(c.StateDir)
	if err != nil {
		return err
	}
	defer unlock()
	logFile, restoreLog, err := openConfiguredLog(c)
	if err != nil {
		return err
	}
	if restoreLog != nil {
		defer func() {
			if closeErr := restoreLog(); resultErr == nil && closeErr != nil {
				resultErr = closeErr
			}
		}()
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	if c.Collection.Mode == "" {
		slog.WarnContext(
			parent,
			"legacy_collection_config",
			"format",
			"group_ids",
			"next_action",
			"Use explicit selected mode with typed conversations to preserve the group-only scope; all mode widens collection.",
		)
	}
	store, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(c.StateDir, "messages.sqlite"),
		c.Policy(),
		c.Storage.RetentionDays,
	)
	if err != nil {
		return err
	}
	defer store.Close()
	telemetryConfig := c.Telemetry.WithDefaults()
	if err = telemetryConfig.Validate(); err != nil {
		return err
	}
	telemetryOwner, observer, err := observability.NewOwned(ctx, observability.Options{
		Enabled: telemetryConfig.Enabled, Exporter: telemetryConfig.Exporter,
		Endpoint: telemetryConfig.Endpoint, Headers: telemetryConfig.Headers,
		ExportTimeout:   time.Duration(telemetryConfig.ExportTimeoutMS) * time.Millisecond,
		ShutdownTimeout: time.Duration(telemetryConfig.ShutdownTimeoutMS) * time.Millisecond,
	})
	if err != nil {
		return err
	}
	defer func() {
		if shutdownErr := telemetryOwner.Shutdown(ctx); shutdownErr != nil {
			slog.WarnContext(parent, "telemetry_shutdown_failed")
		}
	}()
	if observer != nil {
		ctx = shared.WithTelemetry(ctx, observer)
	}
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
		slog.WarnContext(parent, "account_archive_store_unavailable")
	}
	if archives != nil {
		defer archives.Close()
	}
	library, libraryErr := mobilebackup.NewArchiveLibraryStore(
		filepath.Join(c.StateDir, "account-archive-library", "sources"),
		1<<30,
	)
	if libraryErr != nil {
		slog.WarnContext(parent, "account_archive_library_unavailable")
	}
	if library != nil {
		defer library.Close()
	}
	port := &membershipPort{
		stateDir:   c.StateDir,
		store:      store,
		allowSend:  c.Permissions.AllowSend,
		recipients: recipients,
		lifecycle:  ctx,
		snapshots:  snapshots,
		archives:   archives,
		library:    library,
	}
	if err = port.CleanupHistorySnapshots(ctx); err != nil {
		return errors.New("private mobile snapshot cleanup unavailable")
	}
	handler, worker, eventTrace, endpointErr := configuredEndpoints(c, store, port, token)
	if endpointErr != nil {
		return endpointErr
	}
	if eventTrace != nil {
		defer eventTrace.Close()
	}
	if configure != nil {
		configure(handler, worker)
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", c.MCP.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	ipc, socket, ipcErr := openConfiguredIPC(ctx, c.StateDir)
	if ipcErr != nil {
		return ipcErr
	}
	defer ipc.Close()
	defer os.Remove(socket)
	if err = os.Chmod(socket, 0600); err != nil {
		return err
	}
	// Allow the 30s upstream send plus independent result persistence to finish.
	// Shutdown cancels requests before waiting for session ports and closing SQLite.
	httpServer := &http.Server{
		Handler:           handler,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	// The owner-only mobile probe may wait 185s and finish its ledger independently.
	// Keep the Unix response deadline above the 190s CLI request budget.
	cliServer := &http.Server{
		Handler:           port.cliHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      450 * time.Second,
	}
	loop := serviceLoop{
		parent: parent, ctx: ctx, cancel: cancel, config: c, store: store, port: port,
		restore: restore, save: save, ready: ready, worker: worker,
		httpServer: httpServer, cliServer: cliServer, listener: listener, ipc: ipc,
		logFile: logFile, eventTrace: eventTrace,
	}
	return loop.run()
}

func collect(
	ctx context.Context,
	c config.Config,
	store *storage.Store,
	port *membershipPort,
	restore restoreFunc,
	save func(collector.ListenerUpstream) error,
) error {
	return collectWithHeartbeat(ctx, c, store, port, restore, save, 10*time.Second)
}

func collectWithHeartbeat(
	parent context.Context,
	c config.Config,
	store *storage.Store,
	port *membershipPort,
	restore restoreFunc,
	save func(collector.ListenerUpstream) error,
	interval time.Duration,
) error {
	// Capture the previous process's liveness before writing a fresh heartbeat.
	if err := store.BeginGap(parent, "collector_start_or_restart"); err != nil {
		return err
	}
	if err := store.SetState(
		parent,
		map[string]any{
			"authenticated":     false,
			"collector_state":   "reconnecting",
			"last_connected_at": nil,
			"last_event_at":     nil,
			"last_persisted_at": nil,
			"last_error":        nil,
		},
	); err != nil {
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
	if cause := context.Cause(
		ctx,
	); cause != nil && !errors.Is(cause, context.Canceled) &&
		!errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	return err
}

func collectSession(
	ctx context.Context,
	c config.Config,
	store *storage.Store,
	port *membershipPort,
	restore restoreFunc,
	save func(collector.ListenerUpstream) error,
) error {
	delay := time.Second
	for {
		restoreCtx, stop := context.WithTimeout(ctx, 30*time.Second)
		client, err := restore(restoreCtx, c.StateDir)
		stop()
		if ctx.Err() != nil {
			//nolint:nilerr // Owner context cancellation is normal service shutdown, not a session failure requiring retry.
			return nil
		}
		if err == nil {
			err = runVerifiedCollector(ctx, c, store, port, client, save)
			terminal, outcomeErr := activeCollectorOutcome(ctx, err)
			if terminal {
				return outcomeErr
			}
		}
		if errors.Is(err, domain.ErrAuthenticationRequired) {
			return awaitLocalLogin(ctx, store)
		}
		var typed *domain.Error
		if errors.As(err, &typed) && typed.Code == "STORAGE_ERROR" {
			return persistSessionStorageFailure(ctx, store, err)
		}
		retry, retryErr := awaitSessionRetry(ctx, store, delay)
		if retryErr != nil || !retry {
			return retryErr
		}
		delay = min(delay*2, 40*time.Second)
	}
}

func runActiveCollector(ctx context.Context, c config.Config, store *storage.Store, port *membershipPort, client collector.ListenerUpstream) error {
	sessionCtx, stopSession := context.WithCancel(ctx)
	var mobileDone chan error
	err := collector.RunInternal(sessionCtx, c, store, client, func(j *collector.JoinManager) error {
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
	return err
}

type serviceLoop struct {
	parent, ctx           context.Context
	cancel                context.CancelFunc
	config                config.Config
	store                 *storage.Store
	port                  *membershipPort
	restore               restoreFunc
	save                  func(collector.ListenerUpstream) error
	ready                 func(net.Addr)
	worker                *events.Worker
	httpServer, cliServer *http.Server
	listener, ipc         net.Listener
	logFile, eventTrace   *logging.File
}

func (loop serviceLoop) run() error {
	errorsCh := make(chan error, 8)
	var wg sync.WaitGroup
	launch := func(fn func() error) {
		wg.Go(func() {
			err := fn()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			errorsCh <- err
			loop.cancel()
		})
	}
	if loop.logFile != nil {
		launch(func() error {
			select {
			case <-loop.ctx.Done():
				return nil
			case <-loop.logFile.Failed():
				return loop.logFile.Err()
			}
		})
	}
	if loop.eventTrace != nil {
		launch(func() error {
			select {
			case <-loop.ctx.Done():
				return nil
			case <-loop.eventTrace.Failed():
				return loop.eventTrace.Err()
			}
		})
	}
	slog.InfoContext(loop.parent, "service started", "mcp_listen", loop.listener.Addr().String())
	launch(func() error { return loop.httpServer.Serve(loop.listener) })
	launch(func() error { return loop.cliServer.Serve(loop.ipc) })
	launch(func() error { return loop.worker.Run(loop.ctx) })
	if shared.Recording(loop.ctx) {
		launch(func() error { return observeNativeQueues(loop.ctx, loop.store) })
	}

	launch(loop.retain)
	launch(func() error { return collect(loop.ctx, loop.config, loop.store, loop.port, loop.restore, loop.save) })
	if loop.ready != nil {
		loop.ready(loop.listener.Addr())
	}
	<-loop.ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	// Stop accepting requests before waiting for ports or closing shared SQLite.
	_ = loop.httpServer.Shutdown(shutdown)
	_ = loop.cliServer.Shutdown(shutdown)
	_ = loop.httpServer.Close()
	_ = loop.cliServer.Close()
	wg.Wait()
	if shared.Recording(loop.ctx) {
		sample, cancelSample := context.WithTimeout(context.WithoutCancel(loop.ctx), 500*time.Millisecond)
		sampleNativeQueues(sample, loop.store)
		cancelSample()
	}
	slog.InfoContext(loop.parent, "service stopped")
	if loop.logFile != nil && loop.logFile.Err() != nil {
		return loop.logFile.Err()
	}
	close(errorsCh)
	for err := range errorsCh {
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	return nil
}

func (loop serviceLoop) retain() error {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-loop.ctx.Done():
			return nil
		case <-ticker.C:
			if err := loop.store.Retain(loop.ctx); err != nil {
				return err
			}
		}
	}
}

func openConfiguredLog(c config.Config) (*logging.File, func() error, error) {
	if c.Logging.File == "" {
		return nil, nil, nil
	}
	logFile, err := logging.Open(c.Logging.File, c.Logging.MaxSizeMB, c.Logging.MaxBackups, os.Stderr)
	if err != nil {
		return nil, nil, err
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
	restore := func() error {
		slog.SetDefault(previous)
		return logFile.Close()
	}
	return logFile, restore, nil
}

func runVerifiedCollector(ctx context.Context, c config.Config, store *storage.Store, port *membershipPort, client collector.ListenerUpstream, save func(collector.ListenerUpstream) error) error {
	var err error
	if err = store.BindAccount(ctx, client.AccountID()); err != nil {
		return err
	}
	if err = save(client); err != nil {
		return err
	}
	return runActiveCollector(ctx, c, store, port, client)
}

func awaitLocalLogin(ctx context.Context, store *storage.Store) error {
	var err error
	if err = store.BeginGap(ctx, "auth_required"); err != nil {
		return err
	}
	if err = store.SetState(
		ctx,
		map[string]any{
			"authenticated":     false,
			"collector_state":   "auth_required",
			"last_connected_at": nil,
			"last_event_at":     nil,
			"last_persisted_at": nil,
			"last_error": map[string]any{
				"code":    "NOT_AUTHENTICATED",
				"message": "Stop service and run local login.",
			},
		},
	); err != nil {
		return err
	}
	slog.WarnContext(ctx, "Zalo authentication required; waiting for local login")
	<-ctx.Done()
	return nil

}

func persistSessionStorageFailure(ctx context.Context, store *storage.Store, err error) error {
	if saveErr := store.SetState(
		ctx,
		map[string]any{
			"authenticated":     false,
			"collector_state":   "stopped",
			"last_connected_at": nil,
			"last_event_at":     nil,
			"last_persisted_at": nil,
			"last_error": map[string]any{
				"code":    "STORAGE_ERROR",
				"message": "Cannot read private saved session.",
			},
		},
	); saveErr != nil {
		return saveErr
	}
	return err
}

func configuredEndpoints(c config.Config, store *storage.Store, port *membershipPort, token string) (*mcpserver.HTTPService, *events.Worker, *logging.File, error) {
	protocol, err := events.NewSubscriptionManager(store, c.StateDir)
	if err != nil {
		return nil, nil, nil, err
	}
	handler, err := mcpserver.NewHTTP(token, mcpReadPorts(store, port), protocol)
	if err != nil {
		return nil, nil, nil, err
	}
	worker, err := events.NewWorker(store)
	if err != nil {
		return nil, nil, nil, err
	}
	var eventTrace *logging.File
	if c.Logging.EventTraceFile != "" {
		eventTrace, err = logging.Open(c.Logging.EventTraceFile, c.Logging.MaxSizeMB, c.Logging.MaxBackups, os.Stderr)
		if err != nil {
			return nil, nil, nil, err
		}
		worker.TraceLogger = slog.New(slog.NewJSONHandler(eventTrace, nil))
	}
	return handler, worker, eventTrace, nil
}

func openConfiguredIPC(ctx context.Context, stateDir string) (net.Listener, string, error) {
	socket, err := local.SocketPath(stateDir)
	if err != nil {
		return nil, "", err
	}
	if err = os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	ipc, err := (&net.ListenConfig{}).Listen(ctx, "unix", socket)
	if err != nil {
		return nil, "", err
	}
	return ipc, socket, nil
}

func awaitSessionRetry(ctx context.Context, store *storage.Store, delay time.Duration) (bool, error) {
	if err := store.BeginGap(ctx, "session_restore_failed"); err != nil {
		return false, err
	}
	if err := store.SetState(
		ctx,
		map[string]any{
			"authenticated":     false,
			"collector_state":   "reconnecting",
			"last_connected_at": nil,
			"last_event_at":     nil,
			"last_persisted_at": nil,
			"last_error": map[string]any{
				"code":    "UPSTREAM_UNAVAILABLE",
				"message": "Saved session verification failed; retrying.",
			},
		},
	); err != nil {
		return false, err
	}
	slog.WarnContext(ctx, "Zalo session verification unavailable; retrying", "retry_seconds", delay.Seconds())
	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		return false, nil
	case <-timer.C:
	}
	return true, nil
}

func activeCollectorOutcome(ctx context.Context, err error) (bool, error) {
	if ctx.Err() != nil {
		//nolint:nilerr // Owner cancellation is normal session shutdown; the worker error must not trigger a retry.
		return true, nil
	}
	if err == nil {
		return true, errors.New("collector stopped unexpectedly")
	}
	if !errors.Is(err, domain.ErrAuthenticationRequired) {
		return true, err
	}
	return false, nil
}

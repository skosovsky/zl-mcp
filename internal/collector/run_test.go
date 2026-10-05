package collector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type scriptedListener struct {
	*fakeZalo
	listen func(context.Context, func(domain.Message) error, func(string, string) error, func() error) error
}

func (s *scriptedListener) Listen(ctx context.Context, message func(domain.Message) error, deletion func(string, string) error, connected func() error) error {
	return s.listen(ctx, message, deletion, connected)
}

func sessionStore(t *testing.T) (config.Config, *storage.Store) {
	t.Helper()
	// Short socket path: macOS limits AF_UNIX addresses to 104 bytes.
	dir, err := os.MkdirTemp("/tmp", "zl-session-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s, err := storage.Open(context.Background(), filepath.Join(dir, "messages.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return config.Config{StateDir: dir}, s
}

func TestSessionCollectsReplayAndStopsCleanly(t *testing.T) {
	// Arrange: the real collector lifecycle receives fake listener deliveries.
	c, s := sessionStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &scriptedListener{fakeZalo: &fakeZalo{name: "Test group", joined: true}}
	client.listen = func(ctx context.Context, onMessage func(domain.Message) error, _ func(string, string) error, onConnected func() error) error {
		if err := onConnected(); err != nil {
			return err
		}
		m := domain.Message{GroupID: "g", ID: "m", SenderID: "sender", SentAt: time.Now().UTC(), Text: "Ремонт кондиционера", Source: "live"}
		if err := onMessage(m); err != nil {
			return err
		}
		m.Source = "replay"
		if err := onMessage(m); err != nil {
			return err
		}
		m.GroupID = "outside"
		if err := onMessage(m); err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	}
	// Act
	err := RunInternal(ctx, c, s, client, func(*JoinManager) error { return nil })
	hits, _, _, searchErr := s.Search(context.Background(), storage.Search{Query: "ремонт"})
	state, stateErr := s.State(context.Background())
	coverage, coverageErr := s.Coverage(context.Background(), "g")
	// Assert: cancellation drains lifecycle; one permitted message remains searchable.
	// A connected listener does not establish history completeness or close its gap.
	if err != nil || searchErr != nil || stateErr != nil || coverageErr != nil {
		t.Fatalf("session=%v search=%v state=%v coverage=%v", err, searchErr, stateErr, coverageErr)
	}
	if len(hits) != 1 || state["collector_state"] != "stopped" || len(coverage.KnownGaps) != 1 {
		t.Fatalf("hits=%+v state=%+v coverage=%+v", hits, state, coverage)
	}
	if coverage.KnownGaps[0].To != nil || coverage.HistoryComplete {
		t.Fatalf("incorrect coverage: %+v", coverage)
	}
	if _, err = s.Message(context.Background(), "outside", "m"); err == nil {
		t.Fatal("outside allowlist persisted")
	}
	if _, err = os.Stat(filepath.Join(c.StateDir, "collector.sock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket not removed: %v", err)
	}
}

func TestStorageFailureRemainsVisibleDuringReconnect(t *testing.T) {
	// Arrange: fail only message INSERTs, leaving status and gap storage available.
	c, s := sessionStore(t)
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_message BEFORE INSERT ON messages BEGIN SELECT RAISE(FAIL,'synthetic write failure'); END`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var attempts atomic.Int32
	reconnecting := make(chan struct{})
	client := &scriptedListener{fakeZalo: &fakeZalo{name: "Test group", joined: true}}
	client.listen = func(ctx context.Context, onMessage func(domain.Message) error, _ func(string, string) error, onConnected func() error) error {
		if attempts.Add(1) == 1 {
			if err := onConnected(); err != nil {
				return err
			}
			return onMessage(domain.Message{GroupID: "g", ID: "m", SenderID: "sender", SentAt: time.Now().UTC(), Text: "test", Source: "live"})
		}
		close(reconnecting)
		<-ctx.Done()
		return ctx.Err()
	}
	finished := make(chan error, 1)
	// Act
	go func() { finished <- RunInternal(ctx, c, s, client, func(*JoinManager) error { return nil }) }()
	select {
	case <-reconnecting:
	case <-time.After(5 * time.Second):
		cancel()
		<-finished
		t.Fatal("listener did not reconnect")
	}
	state, err := s.State(context.Background())
	coverage, coverageErr := s.Coverage(context.Background(), "g")
	cancel()
	runErr := <-finished
	// Assert: the loop retains the storage cause, rather than overwriting it with upstream failure.
	if err != nil || coverageErr != nil || runErr != nil {
		t.Fatalf("state=%v coverage=%v run=%v", err, coverageErr, runErr)
	}
	last := state["last_error"].(map[string]any)
	if last["code"] != "STORAGE_ERROR" || len(coverage.KnownGaps) != 1 || coverage.KnownGaps[0].Reason != "collector_start_or_restart" || coverage.KnownGaps[0].To != nil {
		t.Fatalf("state=%+v coverage=%+v", state, coverage)
	}
}

func TestConnectedListenerResolvesOnlyListenerFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		storage bool
	}{
		{name: "listener failure clears"},
		{name: "storage failure remains", storage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: interrupt the first connected session, then reconnect.
			c, s := sessionStore(t)
			if tc.storage {
				if _, err := s.DB.Exec(`CREATE TRIGGER fail_message BEFORE INSERT ON messages BEGIN SELECT RAISE(FAIL,'synthetic write failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			finished := make(chan error, 1)
			observed := make(chan [2]map[string]any, 1)
			var attempts int
			client := &scriptedListener{fakeZalo: &fakeZalo{name: "Test group", joined: true}}
			client.listen = func(ctx context.Context, message func(domain.Message) error, _ func(string, string) error, connected func() error) error {
				attempts++
				if attempts == 1 {
					if err := connected(); err != nil {
						return err
					}
					if tc.storage {
						return message(domain.Message{GroupID: "g", ID: "m", SenderID: "sender", SentAt: time.Now().UTC(), Text: "test", Source: "live"})
					}
					return errors.New("synthetic listener interruption")
				}
				before, err := s.State(ctx)
				if err != nil {
					return err
				}
				if err = connected(); err != nil {
					return err
				}
				after, err := s.State(ctx)
				if err != nil {
					return err
				}
				observed <- [2]map[string]any{before, after}
				<-ctx.Done()
				return ctx.Err()
			}
			// Act: run the production reconnect loop and read its durable state.
			go func() { finished <- RunInternal(ctx, c, s, client, func(*JoinManager) error { return nil }) }()
			var states [2]map[string]any
			select {
			case states = <-observed:
				cancel()
				if err := <-finished; err != nil {
					t.Fatal(err)
				}
			case err := <-finished:
				cancel()
				t.Fatalf("collector stopped before reconnect: %v", err)
			case <-time.After(10 * time.Second):
				cancel()
				<-finished
				t.Fatal("listener did not reconnect")
			}
			// Assert: readiness resolves transport failure, not persistence failure.
			if states[0]["collector_state"] != "connecting" || states[1]["collector_state"] != "connected" || states[1]["last_connected_at"] == nil {
				t.Fatalf("unexpected reconnect states: %+v", states)
			}
			before, ok := states[0]["last_error"].(map[string]any)
			if !ok {
				t.Fatal("reconnect cause missing")
			}
			if tc.storage {
				after, ok := states[1]["last_error"].(map[string]any)
				if !ok || before["code"] != "STORAGE_ERROR" || after["code"] != "STORAGE_ERROR" {
					t.Fatalf("storage cause was cleared: %+v", states)
				}
			} else if before["code"] != "UPSTREAM_UNAVAILABLE" || states[1]["last_error"] != nil {
				t.Fatalf("recovered listener error remains: %+v", states)
			}
		})
	}
}

func TestAuthenticationFailureStopsReconnectAndPreservesState(t *testing.T) {
	// Arrange: the listener reports an explicit authentication rejection after connecting.
	c, s := sessionStore(t)
	var attempts atomic.Int32
	client := &scriptedListener{fakeZalo: &fakeZalo{name: "Test group", joined: true}}
	client.listen = func(_ context.Context, _ func(domain.Message) error, _ func(string, string) error, connected func() error) error {
		attempts.Add(1)
		if err := connected(); err != nil {
			return err
		}
		return fmt.Errorf("listener: %w", domain.ErrAuthenticationRequired)
	}
	// Act
	err := RunInternal(context.Background(), c, s, client, func(*JoinManager) error { return nil })
	state, stateErr := s.State(context.Background())
	coverage, coverageErr := s.Coverage(context.Background(), "g")
	// Assert: cleanup must not overwrite the terminal auth_required state with stopped.
	if !errors.Is(err, domain.ErrAuthenticationRequired) || stateErr != nil || coverageErr != nil || attempts.Load() != 1 {
		t.Fatalf("run=%v state=%v coverage=%v attempts=%d", err, stateErr, coverageErr, attempts.Load())
	}
	if state["authenticated"] != false || state["collector_state"] != "auth_required" || state["last_error"].(map[string]any)["code"] != "NOT_AUTHENTICATED" {
		t.Fatalf("incorrect auth state: %+v", state)
	}
	if len(coverage.KnownGaps) != 1 || coverage.KnownGaps[0].Reason != "collector_start_or_restart" || coverage.KnownGaps[0].To != nil {
		t.Fatalf("missing authentication gap: %+v", coverage)
	}
	if _, err := os.Stat(filepath.Join(c.StateDir, "collector.sock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("control socket not cleaned: %v", err)
	}
}

func TestInternalCollectorHasNoControlListener(t *testing.T) {
	// Arrange: unified service owns the store and injects a synthetic upstream.
	c, store := sessionStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &scriptedListener{fakeZalo: &fakeZalo{name: "Test group", joined: true}}
	readyCalls := 0
	client.listen = func(ctx context.Context, message func(domain.Message) error, _ func(string, string) error, connected func() error) error {
		// Assert while the collector is active: it has no IPC listener of its own.
		if _, err := os.Stat(filepath.Join(c.StateDir, "collector.sock")); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("internal collector opened a socket: %v", err)
		}
		if err := connected(); err != nil {
			return err
		}
		if err := message(domain.Message{GroupID: "g", ID: "internal", SenderID: "sender", SentAt: time.Now().UTC(), Text: "internal collector", Source: "live"}); err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	}
	// Act: receive a direct membership port without opening collector IPC.
	err := RunInternal(ctx, c, store, client, func(port *JoinManager) error {
		readyCalls++
		_, err := port.Call(ctx, "zalo_get_group", map[string]any{"group_id": "g"})
		return err
	})
	_, readErr := store.Message(context.Background(), "g", "internal")
	// Assert: collector ran once, persisted data, and left service-owned storage usable.
	if err != nil || readErr != nil || readyCalls != 1 {
		t.Fatalf("run=%v read=%v ready=%d", err, readErr, readyCalls)
	}
}

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
	"github.com/skosovsky/zl-mcp/internal/zalo"
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
	err := runSession(ctx, c, s, client)
	hits, _, _, searchErr := s.Search(context.Background(), storage.Search{Query: "ремонт"})
	state, stateErr := s.State(context.Background())
	coverage, coverageErr := s.Coverage(context.Background(), "g")
	// Assert: cancellation drains lifecycle; one permitted message remains searchable.
	if err != nil || searchErr != nil || stateErr != nil || coverageErr != nil {
		t.Fatalf("session=%v search=%v state=%v coverage=%v", err, searchErr, stateErr, coverageErr)
	}
	if len(hits) != 1 || state["collector_state"] != "stopped" || len(coverage.KnownGaps) != 2 {
		t.Fatalf("hits=%+v state=%+v coverage=%+v", hits, state, coverage)
	}
	if coverage.KnownGaps[0].To == nil || coverage.KnownGaps[1].To != nil || coverage.HistoryComplete {
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
	go func() { finished <- runSession(ctx, c, s, client) }()
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
	if last["code"] != "STORAGE_ERROR" || len(coverage.KnownGaps) != 2 || coverage.KnownGaps[1].Reason != "storage_error" || coverage.KnownGaps[1].To != nil {
		t.Fatalf("state=%+v coverage=%+v", state, coverage)
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
	err := runSession(context.Background(), c, s, client)
	state, stateErr := s.State(context.Background())
	coverage, coverageErr := s.Coverage(context.Background(), "g")
	// Assert: cleanup must not overwrite the terminal auth_required state with stopped.
	if !errors.Is(err, domain.ErrAuthenticationRequired) || stateErr != nil || coverageErr != nil || attempts.Load() != 1 {
		t.Fatalf("run=%v state=%v coverage=%v attempts=%d", err, stateErr, coverageErr, attempts.Load())
	}
	if state["authenticated"] != false || state["collector_state"] != "auth_required" || state["last_error"].(map[string]any)["code"] != "NOT_AUTHENTICATED" {
		t.Fatalf("incorrect auth state: %+v", state)
	}
	if len(coverage.KnownGaps) != 2 || coverage.KnownGaps[1].Reason != "auth_required" || coverage.KnownGaps[1].To != nil {
		t.Fatalf("missing authentication gap: %+v", coverage)
	}
	if _, err := os.Stat(filepath.Join(c.StateDir, "collector.sock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("control socket not cleaned: %v", err)
	}
}

func TestStartupRestoreFailurePersistsCorrectRecoveryState(t *testing.T) {
	for _, tc := range []struct {
		name        string
		failure     error
		state, code string
	}{
		{"authentication", domain.ErrAuthenticationRequired, "auth_required", "NOT_AUTHENTICATED"},
		{"network", &domain.Error{Code: "UPSTREAM_UNAVAILABLE", Message: "synthetic"}, "stopped", "UPSTREAM_UNAVAILABLE"},
		{"storage", &domain.Error{Code: "STORAGE_ERROR", Message: "synthetic"}, "stopped", "STORAGE_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: execute the real lock/database/startup lifecycle with a failed restore port.
			c, s := sessionStore(t)
			var calls int
			restore := func(context.Context, string) (*zalo.Client, error) { calls++; return nil, tc.failure }
			// Act
			err := runWithRestore(context.Background(), c, restore)
			state, stateErr := s.State(context.Background())
			// Assert: only explicit authentication failure asks for a fresh local login.
			if !errors.Is(err, tc.failure) || calls != 1 || stateErr != nil || state["collector_state"] != tc.state || state["last_error"].(map[string]any)["code"] != tc.code {
				t.Fatalf("error=%v state=%+v read=%v calls=%d", err, state, stateErr, calls)
			}
		})
	}
}

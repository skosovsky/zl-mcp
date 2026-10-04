package collector

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type profileFunction func(context.Context, []string) ([]domain.Contact, error)

func (f profileFunction) ContactProfiles(ctx context.Context, ids []string) ([]domain.Contact, error) {
	return f(ctx, ids)
}

type profileListener struct {
	*scriptedListener
	profiles profileFunction
}

func (*profileListener) ContactsPage(context.Context, int, int) ([]domain.Contact, error) {
	return nil, nil
}

func (p *profileListener) ContactProfiles(ctx context.Context, ids []string) ([]domain.Contact, error) {
	return p.profiles(ctx, ids)
}

func TestDirectoryWorkerProfileAuthenticationBoundary(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO conversations VALUES('direct','synthetic-peer',NULL,'stored_message','observed','2026-10-03T00:00:00Z','2026-10-03T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var calls atomic.Int32
	source := &profileListener{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}}
	source.profiles = func(request context.Context, ids []string) ([]domain.Contact, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-request.Done()
		return nil, request.Err()
	}
	guard := newSessionGuard(ctx, source)
	defer guard.cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		contactsLoop(ctx, s, guard)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("directory worker did not stop")
		}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("directory worker did not start profile request")
	}

	// Act
	guard.observe(domain.ErrAuthenticationRequired)
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, statusErr := s.ProfileStatus(ctx)
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if status["stop_reason"] == "auth_required" {
			if status["status"] != "partial" {
				t.Fatalf("auth loss status: %#v", status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("profile refresh did not observe auth loss: %#v", status)
		}
		time.Sleep(time.Millisecond)
	}
	_, err = guard.ContactProfiles(ctx, []string{"synthetic-peer"})

	// Assert
	if !errors.Is(err, domain.ErrAuthenticationRequired) || calls.Load() != 1 {
		t.Fatalf("new profile request crossed auth boundary: err=%v calls=%d", err, calls.Load())
	}
	var n int
	if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM directory_contacts`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("auth-cancelled worker persisted profiles: count=%d error=%v", n, err)
	}
}

func TestProfileRefreshCancellationBeforePersistence(t *testing.T) {
	// Arrange
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := storage.OpenWithPolicy(parent, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.DB.ExecContext(parent, `INSERT INTO conversations VALUES('direct','synthetic-peer',NULL,'stored_message','observed','2026-10-03T00:00:00Z','2026-10-03T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	source := profileFunction(func(context.Context, []string) ([]domain.Contact, error) {
		cancel()
		return []domain.Contact{{ID: "synthetic-peer", Name: "Synthetic", Friendship: "unknown"}}, nil
	})

	// Act
	err = refreshContactProfiles(parent, s, source)
	status, statusErr := s.ProfileStatus(context.Background())

	// Assert
	if !errors.Is(err, context.Canceled) || statusErr != nil || status["status"] != "partial" || status["stop_reason"] != "cancelled_or_deadline" {
		t.Fatalf("cancellation result: %v, status error: %v, status: %#v", err, statusErr, status)
	}
	var n int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM directory_contacts`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("cancelled refresh persisted profiles: count=%d error=%v", n, err)
	}
}

func TestProfileRefreshMissingForeignAndBatchLimits(t *testing.T) {
	for _, test := range []struct {
		name, mode                  string
		n, calls, returned, missing int
		state, reason               string
		wantErr                     bool
	}{
		{"complete", "complete", 101, 2, 101, 0, "completed", "known_ids_processed", false},
		{"missing", "missing", 101, 2, 0, 101, "partial", "missing_profiles", false},
		{"foreign", "foreign", 1, 1, 0, 0, "partial", "storage_or_metadata_error", true},
		{"bounded", "complete", 2001, 20, 2000, 0, "partial", "page_limit", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			s, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			tx, err := s.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < test.n; i++ {
				if _, err = tx.ExecContext(ctx, `INSERT INTO conversations VALUES('direct',?,NULL,'stored_message','observed','2026-10-03T00:00:00Z','2026-10-03T00:00:00Z')`, fmt.Sprintf("peer-%04d", i)); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			calls := 0
			source := profileFunction(func(ctx context.Context, ids []string) ([]domain.Contact, error) {
				calls++
				if len(ids) > 100 {
					t.Fatal("unbounded batch")
				}
				if test.mode == "missing" {
					return nil, nil
				}
				if test.mode == "foreign" {
					return []domain.Contact{{ID: "foreign", Friendship: "unknown"}}, nil
				}
				result := []domain.Contact{}
				for _, id := range ids {
					result = append(result, domain.Contact{ID: id, Name: "Synthetic", Friendship: "unknown"})
				}
				return result, nil
			})
			// Act
			err = refreshContactProfiles(ctx, s, source)
			status, e := s.ProfileStatus(ctx)
			if e != nil {
				t.Fatal(e)
			}
			// Assert
			if (err != nil) != test.wantErr || calls != test.calls || status["status"] != test.state || status["stop_reason"] != test.reason || status["returned_count"] != float64(test.returned) || status["missing_count"] != float64(test.missing) {
				t.Fatalf("wrong source result: %v %#v calls=%d", err, status, calls)
			}
			for _, table := range []string{"messages", "message_events", "peer_first_incoming"} {
				var n int
				if e = s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); e != nil {
					t.Fatal(e)
				}
				if n != 0 {
					t.Fatalf("profile refresh created %s records", table)
				}
			}
		})
	}
}

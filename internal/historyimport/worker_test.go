package historyimport_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type pageSource func(context.Context, domain.ConversationRef, string, int) (domain.HistoryPage, error)

func (f pageSource) HistoryPage(ctx context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
	return f(ctx, ref, cursor, limit)
}

func prepare(t *testing.T) (*storage.Store, storage.HistoryOperation) {
	t.Helper()
	s, err := storage.OpenWithPolicy(context.Background(), filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err = s.BindAccount(context.Background(), "synthetic-account"); err != nil {
		t.Fatal(err)
	}
	r := domain.HistoryImportRequest{ConversationType: "group", ConversationID: "synthetic-group", RequestID: "00000000-0000-4000-8000-000000000001", Since: "2026-09-01T00:00:00Z", Until: "2026-11-01T00:00:00Z", PageSize: 2, MaxMessages: 3}
	op, err := s.PrepareHistoryOperation(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return s, op
}

func message(ref domain.ConversationRef, id string) domain.Message {
	return domain.Message{Conversation: ref, ID: id, SenderID: "synthetic-author", SentAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Text: "Synthetic history"}
}

func awaitState(t *testing.T, s *storage.Store, id, state string) storage.HistoryOperation {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		op, err := s.HistoryOperation(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status.State == state {
			return op
		}
		if time.Now().After(deadline) {
			t.Fatalf("wanted %s, got %s", state, op.Status.State)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWorkerUsesExactCheckpointAndRemainingRecordBudget(t *testing.T) {
	// Arrange
	s, op := prepare(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	source := pageSource(func(ctx context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
		m := message(ref, "first")
		more := calls.Add(1) == 1
		if more {
			if cursor != "0" || limit != 2 {
				return domain.HistoryPage{}, domain.Invalid("Wrong initial source request.")
			}
			outside := message(ref, "outside")
			outside.SentAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			next := "9007199254740993123"
			return domain.HistoryPage{Messages: []domain.Message{m, outside}, HasMore: &more, Cursor: &next}, nil
		}
		if cursor != "9007199254740993123" || limit != 1 {
			return domain.HistoryPage{}, domain.Invalid("Cursor precision or remaining bound lost.")
		}
		return domain.HistoryPage{Messages: []domain.Message{m}, HasMore: &more}, nil
	})
	done := make(chan error, 1)
	go func() { done <- historyimport.Run(ctx, s, source) }()
	// Act
	finished := awaitState(t, s, op.Status.OperationID, "completed")
	cancel()
	// Assert
	if err := <-done; err != nil || calls.Load() != 2 || finished.Status.InsertedCount != 1 || finished.Status.DuplicateCount != 1 || finished.Status.OutOfIntervalCount != 1 || finished.Status.RecordsObserved != 3 || finished.Status.HistoryComplete {
		t.Fatalf("worker result: %+v calls=%d error=%v", finished.Status, calls.Load(), err)
	}
	for _, table := range []string{"message_events", "event_deliveries"} {
		var n int
		if err := s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("history notified: table=%s count=%d error=%v", table, n, err)
		}
	}
}

func TestWorkerAuthPauseResumesOnlyInNextSession(t *testing.T) {
	// Arrange
	s, op := prepare(t)
	source := pageSource(func(context.Context, domain.ConversationRef, string, int) (domain.HistoryPage, error) {
		return domain.HistoryPage{}, domain.ErrAuthenticationRequired
	})
	// Act
	err := historyimport.Run(context.Background(), s, source)
	// Assert
	if !errors.Is(err, domain.ErrAuthenticationRequired) {
		t.Fatal("auth loss did not end worker session", err)
	}
	paused := awaitState(t, s, op.Status.OperationID, "paused")
	if paused.Status.StopReason == nil || *paused.Status.StopReason != "auth_required" {
		t.Fatal("auth pause not recorded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- historyimport.Run(ctx, s, pageSource(func(ctx context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
			more := false
			return domain.HistoryPage{Messages: []domain.Message{message(ref, "after-auth")}, HasMore: &more}, nil
		}))
	}()
	finished := awaitState(t, s, op.Status.OperationID, "completed")
	cancel()
	if err = <-done; err != nil || finished.Status.OperationID != paused.Status.OperationID || finished.WorkDuration < paused.WorkDuration {
		t.Fatal("new session lost operation/checkpoint", err)
	}
}

func TestWorkerCannotPersistPageAfterCancellation(t *testing.T) {
	// Arrange
	s, op := prepare(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- historyimport.Run(ctx, s, pageSource(func(ctx context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
			if ref.ID != op.Status.ConversationID {
				more := false
				return domain.HistoryPage{HasMore: &more}, nil
			}
			close(started)
			select {
			case <-release:
				more := false
				return domain.HistoryPage{Messages: []domain.Message{message(ref, "late")}, HasMore: &more}, nil
			case <-ctx.Done():
				return domain.HistoryPage{}, ctx.Err()
			}
		}))
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("source request not started")
	}
	// Act
	if _, err := s.CancelHistoryOperation(ctx, op.Status.OperationID); err != nil {
		t.Fatal(err)
	}
	secondRequest := op.Status.HistoryImportRequest
	secondRequest.ConversationID, secondRequest.RequestID = "second-synthetic-group", "00000000-0000-4000-8000-000000000002"
	second, err := s.PrepareHistoryOperation(ctx, secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	// Completion of the next sequential operation proves the late page was
	// handled while the worker/session remained alive, not discarded by shutdown.
	awaitState(t, s, second.Status.OperationID, "completed")
	cancel()
	// Assert
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB.QueryRow("SELECT count(*) FROM messages").Scan(&n); err != nil || n != 0 {
		t.Fatal("cancelled page was persisted", err)
	}
	awaitState(t, s, op.Status.OperationID, "cancelled")
}

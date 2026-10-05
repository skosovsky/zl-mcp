package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func historyRequest() domain.HistoryImportRequest {
	return domain.HistoryImportRequest{ConversationType: "group", ConversationID: "synthetic-group", RequestID: "00000000-0000-4000-8000-000000000001", Since: "2026-09-01T00:00:00Z", Until: "2026-11-01T00:00:00Z"}
}

func historyOperationStore(t *testing.T, path string, policy domain.CollectionPolicy) *Store {
	t.Helper()
	s, err := OpenWithPolicy(context.Background(), path, policy, 90)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindAccount(context.Background(), "synthetic-account"); err != nil {
		s.Close()
		t.Fatal(err)
	}
	return s
}

func assertHistoryStatusSchema(t *testing.T, op HistoryOperation) {
	t.Helper()
	b, err := json.Marshal(op.Status)
	if err != nil {
		t.Fatal(err)
	}
	var body any
	if err = json.Unmarshal(b, &body); err != nil {
		t.Fatal(err)
	}
	schema, err := contracts.Compile("history_import_operation", "output")
	if err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(body); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryOperationDurableCheckpointAndRequestIdentity(t *testing.T) {
	// Arrange
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s := historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	defer func() { s.Close() }()
	r := historyRequest()
	op, err := s.PrepareHistoryOperation(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	equivalent := r
	equivalent.Since, equivalent.PageSize, equivalent.MaxPages, equivalent.MaxMessages = "2026-09-01T07:00:00+07:00", 50, 20, 1000
	again, err := s.PrepareHistoryOperation(ctx, equivalent)
	if err != nil || again.Status.OperationID != op.Status.OperationID {
		t.Fatal("equivalent retry created another operation", err)
	}
	conflict := r
	conflict.MaxPages = 1
	if _, err = s.PrepareHistoryOperation(ctx, conflict); !errors.Is(err, ErrHistoryConflict) {
		t.Fatal("changed request accepted", err)
	}
	busy := r
	busy.RequestID = "00000000-0000-4000-8000-000000000002"
	if _, err = s.PrepareHistoryOperation(ctx, busy); !errors.Is(err, ErrHistoryBusy) {
		t.Fatal("same conversation ran concurrently", err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	msg := historicalFixture(r.Ref(), "first")
	msg.SentAt = time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	out := historicalFixture(r.Ref(), "outside")
	out.SentAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	more, cursor := true, "9007199254740993123"
	page := domain.HistoryPage{Messages: []domain.Message{msg, msg, out}, HasMore: &more, Cursor: &cursor}

	// Act
	op, err = s.CommitHistoryOperationPage(ctx, op.Status.OperationID, op.Revision, page, 2*time.Second)

	// Assert
	if err != nil || op.Cursor != cursor || op.Status.State != "running" || op.Status.InsertedCount != 1 || op.Status.DuplicateCount != 1 || op.Status.OutOfIntervalCount != 1 || op.Status.RecordsObserved != 3 || op.Status.PagesObserved != 1 || op.WorkDuration < 2*time.Second || op.WorkDuration > 3*time.Second {
		t.Fatalf("wrong checkpoint: op=%+v error=%v", op, err)
	}
	assertHistoryStatusSchema(t, op)
	oldRevision := op.Revision
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	loaded, err := s.HistoryOperation(ctx, op.Status.OperationID)
	if err != nil || loaded.Status.State != "running" || loaded.Revision != oldRevision {
		t.Fatal("opening diagnostics recovered a live operation", err)
	}
	if err = s.RecoverInterruptedHistory(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err = s.HistoryOperation(ctx, op.Status.OperationID)
	if err != nil || loaded.Status.State != "queued" || loaded.Cursor != cursor || loaded.Status.InsertedCount != 1 || loaded.WorkDuration != op.WorkDuration {
		t.Fatal("restart lost checkpoint", err)
	}
	if _, err = s.CommitHistoryOperationPage(ctx, op.Status.OperationID, oldRevision, page, time.Second); !errors.Is(err, ErrHistoryState) {
		t.Fatal("stale worker advanced checkpoint", err)
	}
	loaded, err = s.ClaimHistoryOperation(ctx, loaded.Status.OperationID, loaded.Revision)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = s.CommitHistoryOperationPage(ctx, loaded.Status.OperationID, loaded.Revision, page, time.Second)
	if err != nil || loaded.Status.State != "partial" || loaded.Status.StopReason == nil || *loaded.Status.StopReason != "repeated_cursor" || loaded.Status.InsertedCount != 1 || loaded.Status.DuplicateCount != 3 {
		t.Fatalf("repeated cursor was not bounded: op=%+v error=%v", loaded, err)
	}
	assertHistoryStatusSchema(t, loaded)
}

func TestHistoryOperationPageAndCheckpointRollbackTogether(t *testing.T) {
	// Arrange
	ctx := context.Background()
	s := historyOperationStore(t, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true})
	defer s.Close()
	r := historyRequest()
	op, err := s.PrepareHistoryOperation(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`CREATE TRIGGER reject_checkpoint BEFORE UPDATE ON history_operations BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	more, cursor := true, "123"
	m := historicalFixture(r.Ref(), "first")
	m.SentAt = time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	// Act
	_, err = s.CommitHistoryOperationPage(ctx, op.Status.OperationID, op.Revision, domain.HistoryPage{Messages: []domain.Message{m}, HasMore: &more, Cursor: &cursor}, time.Second)
	// Assert
	if err == nil {
		t.Fatal("failed checkpoint was reported committed")
	}
	for _, table := range []string{"messages", "message_identities", "conversations", "message_events"} {
		var n int
		if err = s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("page escaped failed checkpoint: table=%s count=%d error=%v", table, n, err)
		}
	}
	loaded, err := s.HistoryOperation(ctx, op.Status.OperationID)
	if err != nil || loaded.Cursor != "0" || loaded.Status.RecordsObserved != 0 || loaded.Revision != op.Revision {
		t.Fatal("failed commit advanced journal", err)
	}
	var n int
	if err = s.DB.QueryRow("SELECT COUNT(*) FROM history_cursors").Scan(&n); err != nil || n != 1 {
		t.Fatal("failed checkpoint retained continuation", err)
	}
}

func TestHistoryOperationCancellationUnsupportedAndAccountBoundary(t *testing.T) {
	// Arrange
	ctx := context.Background()
	s := historyOperationStore(t, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true})
	defer s.Close()
	r := historyRequest()
	op, err := s.PrepareHistoryOperation(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	// Act
	cancelled, err := s.CancelHistoryOperation(ctx, op.Status.OperationID)
	// Assert
	if err != nil || cancelled.Status.State != "cancelled" {
		t.Fatal(err)
	}
	if _, err = s.CommitHistoryOperationPage(ctx, op.Status.OperationID, op.Revision, domain.HistoryPage{}, 0); !errors.Is(err, ErrHistoryState) {
		t.Fatal("cancelled worker persisted", err)
	}
	again, err := s.PrepareHistoryOperation(ctx, r)
	if err != nil || again.Status.State != "cancelled" || again.Status.OperationID != cancelled.Status.OperationID {
		t.Fatal("retry reactivated cancellation", err)
	}
	assertHistoryStatusSchema(t, again)
	r.RequestID, r.ConversationType = "00000000-0000-4000-8000-000000000003", "direct"
	unsupported, err := s.PrepareHistoryOperation(ctx, r)
	if err != nil || unsupported.Status.State != "unsupported" || unsupported.Status.SourceKind != "unsupported" {
		t.Fatal("direct history was claimed supported", err)
	}
	assertHistoryStatusSchema(t, unsupported)
	if _, err = s.DB.Exec("UPDATE metadata SET value='different-synthetic-account' WHERE key='account'"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.HistoryOperation(ctx, cancelled.Status.OperationID); err == nil {
		t.Fatal("operation crossed account binding")
	}
}

func TestHistoryOperationContinuationBoundsAndAuthPause(t *testing.T) {
	for _, mode := range []string{"missing", "filtered", "page_limit", "message_limit", "auth_pause"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			s := historyOperationStore(t, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true})
			defer s.Close()
			r := historyRequest()
			if mode == "page_limit" {
				r.MaxPages = 1
			}
			if mode == "message_limit" {
				r.MaxMessages = 1
			}
			op, err := s.PrepareHistoryOperation(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
			if err != nil {
				t.Fatal(err)
			}
			more, cursor := true, "123"
			m := historicalFixture(r.Ref(), "first")
			m.SentAt = time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
			page := domain.HistoryPage{Messages: []domain.Message{m}, HasMore: &more, Cursor: &cursor}
			reason := mode
			if mode == "missing" {
				page.HasMore = nil
				reason = "missing_continuation"
			}
			if mode == "filtered" {
				page.IsFilteredByTimeJoin = &more
				reason = "source_filtered"
			}
			// Act
			if mode == "auth_pause" {
				op, err = s.StopHistoryOperation(ctx, op.Status.OperationID, op.Revision, "paused", "auth_required", 3*time.Second)
			} else {
				op, err = s.CommitHistoryOperationPage(ctx, op.Status.OperationID, op.Revision, page, time.Second)
			}
			// Assert
			if err != nil {
				t.Fatal(err)
			}
			assertHistoryStatusSchema(t, op)
			if mode == "auth_pause" {
				pending, err := s.PendingHistoryOperations(ctx, 10)
				if err != nil || len(pending) != 1 || pending[0].Status.State != "paused" || pending[0].WorkDuration != 3*time.Second {
					t.Fatal("auth checkpoint missing", err)
				}
				op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
				if err != nil || op.Status.State != "running" || op.WorkDuration != 3*time.Second || op.Status.PagesObserved != 0 {
					t.Fatal("auth resume lost work budget", err)
				}
			} else if op.Status.State != "partial" || op.Status.StopReason == nil || *op.Status.StopReason != reason || op.Status.HistoryComplete {
				t.Fatalf("wrong bounded outcome: %+v", op.Status)
			}
		})
	}
}

func TestHistoryOperationRevocationAndMigrationRollback(t *testing.T) {
	// Arrange
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s := historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	r := historyRequest()
	op, err := s.PrepareHistoryOperation(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = historyOperationStore(t, path, domain.CollectionPolicy{})
	defer s.Close()
	// Act
	err = s.RecoverInterruptedHistory(ctx)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	var state string
	if err = s.DB.QueryRow("SELECT state FROM history_operations WHERE operation_id=?", op.Status.OperationID).Scan(&state); err != nil || state != "cancelled" {
		t.Fatal("revoked operation remained active", err)
	}
	if _, err = s.HistoryOperation(ctx, op.Status.OperationID); err == nil {
		t.Fatal("revoked scope still readable")
	}

	// Arrange: force version 8's final marker to fail after its DDL.
	if _, err = s.DB.Exec(`DROP TABLE history_cursors; DROP TABLE history_operations; DELETE FROM schema_migrations WHERE version=8;
CREATE TRIGGER reject_history_migration BEFORE INSERT ON schema_migrations WHEN new.version=8 BEGIN SELECT RAISE(ABORT,'synthetic migration failure'); END`); err != nil {
		t.Fatal(err)
	}
	// Act
	err = s.migrateHistoryOperations(ctx)
	// Assert
	if err == nil {
		t.Fatal("failed migration succeeded")
	}
	var n int
	if err = s.DB.QueryRow("SELECT count(*) FROM sqlite_master WHERE name IN ('history_operations','history_cursors','history_active_conversation')").Scan(&n); err != nil || n != 0 {
		t.Fatal("partial history schema escaped rollback", err)
	}
	if err = s.DB.QueryRow("SELECT count(*) FROM schema_migrations WHERE version=8").Scan(&n); err != nil || n != 0 {
		t.Fatal("failed migration retained version marker", err)
	}
}

func TestHistoryOperationCrashChargesDurableSourceReservation(t *testing.T) {
	// Arrange: one persisted operation, no source response/checkpoint committed.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s := historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	op, err := s.PrepareHistoryOperation(ctx, historyRequest())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	// Act: simulate four crashes after the durable pre-request reservation.
	for attempt := 0; attempt < 4; attempt++ {
		op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
		if err != nil {
			t.Fatal(err)
		}
		op, err = s.ReserveHistoryPage(ctx, op.Status.OperationID, op.Revision, 30*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ReserveHistoryPage(ctx, op.Status.OperationID, op.Revision, time.Second); !errors.Is(err, ErrHistoryState) {
			t.Fatalf("double reservation allowed: %v", err)
		}
		s.Close()
		s = historyOperationStore(t, path, domain.CollectionPolicy{All: true})
		if err := s.RecoverInterruptedHistory(ctx); err != nil {
			t.Fatal(err)
		}
		op, err = s.HistoryOperation(ctx, op.Status.OperationID)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Assert: no records/events were invented and restart cannot reset the budget.
	if op.WorkDuration != HistoryWorkBudget || op.ReservedDuration != 0 || op.Status.State != "partial" || op.Status.StopReason == nil || *op.Status.StopReason != "time_limit" || op.Status.PagesObserved != 0 {
		t.Fatalf("crash budget not enforced: %#v", op)
	}
}

func TestHistoryOperationReservationReconcilesActualSourceWork(t *testing.T) {
	// Arrange
	ctx := context.Background()
	s := historyOperationStore(t, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true})
	defer s.Close()
	op, err := s.PrepareHistoryOperation(ctx, historyRequest())
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ReserveHistoryPage(ctx, op.Status.OperationID, op.Revision, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Act: auth loss after a short request, then an arbitrary waiting period.
	op, err = s.StopHistoryOperation(ctx, op.Status.OperationID, op.Revision, "paused", "auth_required", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverInterruptedHistory(ctx); err != nil {
		t.Fatal(err)
	}
	op, err = s.HistoryOperation(ctx, op.Status.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: unused reservation/auth wait does not become active source work.
	if op.WorkDuration != time.Millisecond || op.ReservedDuration != 0 || op.Status.State != "paused" {
		t.Fatalf("normal stop charged unused reservation: %#v", op)
	}
}

func TestHistorySourceSelectionPreservesLegacyRequestIdentity(t *testing.T) {
	// Arrange
	ctx := context.Background()
	store := historyOperationStore(t, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true})
	defer store.Close()
	request := historyRequest()
	original, err := store.PrepareHistoryOperation(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	// Act: explicit default is the same effective legacy request.
	request.Source = "group_cloud"
	retry, err := store.PrepareHistoryOperation(ctx, request)
	// Assert
	if err != nil || retry.Status.OperationID != original.Status.OperationID {
		t.Fatal("legacy fingerprint changed")
	}
	request.Source = "conversation_preload"
	if _, err = store.PrepareHistoryOperation(ctx, request); !errors.Is(err, ErrHistoryConflict) {
		t.Fatal("source change reused request identity")
	}
}

func TestHistoryPhaseIdentitySurvivesReopen(t *testing.T) {
	// Arrange
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s := historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	defer func() { s.Close() }()
	op, err := s.PrepareHistoryOperation(ctx, historyRequest())
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	more, old, cursor := true, true, "0"
	page := domain.HistoryPage{PhaseRequired: true, Messages: []domain.Message{historicalFixture(historyRequest().Ref(), "phase")}, HasMore: &more, Cursor: &cursor, IsOld: &old}
	// Act: initial recent/0 and next old/0 are different continuations.
	op, err = s.CommitHistoryOperationPage(ctx, op.Status.OperationID, op.Revision, page, time.Millisecond)
	// Assert
	if err != nil || op.Status.State != "running" || op.Status.IsOld == nil || !*op.Status.IsOld {
		t.Fatal("phase transition rejected", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	op, err = s.HistoryOperation(ctx, op.Status.OperationID)
	if err != nil || op.Cursor != "0" || op.Status.IsOld == nil || !*op.Status.IsOld {
		t.Fatal("phase lost on reopen", err)
	}
	// Act: repeating old/0 must stop without creating an event.
	op, err = s.CommitHistoryOperationPage(ctx, op.Status.OperationID, op.Revision, page, time.Millisecond)
	// Assert
	if err != nil || op.Status.State != "partial" || op.Status.StopReason == nil || *op.Status.StopReason != "repeated_cursor" {
		t.Fatal("same-phase repetition accepted", err)
	}
	assertHistoryStatusSchema(t, op)
	var events int
	if err = s.DB.QueryRow("SELECT count(*) FROM message_events").Scan(&events); err != nil || events != 0 {
		t.Fatal("history generated events", err)
	}
}

func TestHistoryContinuingProductionPageRequiresPhase(t *testing.T) {
	// Arrange
	ctx := context.Background()
	s := historyOperationStore(t, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true})
	defer s.Close()
	op, err := s.PrepareHistoryOperation(ctx, historyRequest())
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	more, cursor := true, "1"
	// Act
	op, err = s.CommitHistoryOperationPage(ctx, op.Status.OperationID, op.Revision, domain.HistoryPage{PhaseRequired: true, Messages: []domain.Message{historicalFixture(historyRequest().Ref(), "phase")}, HasMore: &more, Cursor: &cursor}, time.Millisecond)
	// Assert
	if err != nil || op.Status.State != "partial" || op.Status.StopReason == nil || *op.Status.StopReason != "missing_continuation" {
		t.Fatal("missing phase guessed", err)
	}
}

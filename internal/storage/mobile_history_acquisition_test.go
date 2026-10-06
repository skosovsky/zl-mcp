package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func acquisitionOperation(t *testing.T) (*Store, HistoryOperation, string) {
	t.Helper()
	s, op, path, _ := mobileOperation(t)
	next, err := s.ReserveMobileHistoryWork(context.Background(), op.Status.OperationID, op.Revision, 180*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return s, next, path
}

func countAcquisitionRows(t *testing.T, s *Store, attempts, links int) {
	t.Helper()
	for table, want := range map[string]int{"mobile_backup_attempts": attempts, "history_mobile_attempts": links} {
		var n int
		if err := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != want {
			t.Fatal("unexpected acquisition journal", table, n, err)
		}
	}
}

func TestMobileHistoryAcquisitionBindingIsAtomicAndIdempotent(t *testing.T) {
	// Arrange: claimed/reserved history, with a failure after attempt creation.
	ctx := context.Background()
	s, op, _ := acquisitionOperation(t)
	defer s.Close()
	if _, err := s.DB.Exec(`CREATE TRIGGER reject_acquisition_link BEFORE INSERT ON history_mobile_attempts BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	// Act: the failed link must not leave a dispatchable orphan attempt.
	if _, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0); err == nil {
		t.Fatal("link failure was ignored")
	}
	// Assert: both tables roll back and history revision/reservation are unchanged.
	countAcquisitionRows(t, s, 0, 0)
	got, err := s.HistoryOperation(ctx, op.Status.OperationID)
	if err != nil || got.Revision != op.Revision || got.ReservedDuration != op.ReservedDuration {
		t.Fatal("failed binding changed history", err)
	}
	if _, err = s.DB.Exec("DROP TRIGGER reject_acquisition_link"); err != nil {
		t.Fatal(err)
	}
	attempt, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 64<<20)
	if err != nil || repeat.OperationID != attempt.OperationID || repeat.Revision != attempt.Revision || repeat.State != "prepared" || repeat.Request.RequestID != op.Status.RequestID || repeat.Request.Ref() != op.Status.Ref() || repeat.Request.Since != op.Status.Since || repeat.Request.Until != op.Status.Until || repeat.Request.MaxMessages != op.Status.MaxMessages {
		t.Fatal("retry changed acquisition identity", err)
	}
	if _, err = s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 32<<20); !errors.Is(err, ErrMobileBackupConflict) {
		t.Fatal("archive budget changed", err)
	}
	countAcquisitionRows(t, s, 1, 1)
	attempt, err = s.DispatchMobileBackup(ctx, attempt.OperationID, attempt.Revision, mobileJournalPublic(t))
	if err != nil || attempt.State != "dispatching" {
		t.Fatal("valid binding cannot dispatch", err)
	}
	if _, err = s.DispatchMobileBackup(ctx, attempt.OperationID, 0, mobileJournalPublic(t)); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("same attempt dispatched twice", err)
	}
}

func TestMobileHistoryAcquisitionRejectsStaleCancelledAndUnreservedDispatch(t *testing.T) {
	for _, mode := range []string{"unreserved", "stale-prepare", "cancel-before-bind", "cancel-before-dispatch", "recover-before-dispatch", "terminal-before-dispatch"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: a mobile operation; reserve only for cases that can bind.
			ctx := context.Background()
			s, op, _, _ := mobileOperation(t)
			defer s.Close()
			if mode != "unreserved" {
				var err error
				op, err = s.ReserveMobileHistoryWork(ctx, op.Status.OperationID, op.Revision, 180*time.Second)
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "unreserved" || mode == "stale-prepare" || mode == "cancel-before-bind" {
				revision := op.Revision
				if mode == "stale-prepare" {
					revision--
				}
				if mode == "cancel-before-bind" {
					if _, err := s.CancelHistoryOperation(ctx, op.Status.OperationID); err != nil {
						t.Fatal(err)
					}
				}
				// Act / Assert: invalid authority creates no attempt or link.
				if _, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, revision, 0); !errors.Is(err, ErrHistoryState) {
					t.Fatal("invalid history authority bound an attempt", err)
				}
				countAcquisitionRows(t, s, 0, 0)
				return
			}
			attempt, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "cancel-before-dispatch":
				_, err = s.CancelHistoryOperation(ctx, op.Status.OperationID)
			case "recover-before-dispatch":
				err = s.RecoverInterruptedHistory(ctx)
				if err == nil {
					op, err = s.HistoryOperation(ctx, op.Status.OperationID)
				}
				if err == nil {
					op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
				}
				if err == nil {
					// The existing prepared attempt remains bound to its original revision.
					var same MobileBackupAttempt
					same, err = s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
					if err == nil && same.OperationID != attempt.OperationID {
						t.Fatal("recovery created replacement acquisition")
					}
				}
			case "terminal-before-dispatch":
				_, err = s.StopHistoryOperation(ctx, op.Status.OperationID, op.Revision, "partial", "source_unavailable", 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			// Act: the attempt's own revision is still current, but history authority changed.
			_, err = s.DispatchMobileBackup(ctx, attempt.OperationID, attempt.Revision, mobileJournalPublic(t))
			// Assert: dispatch/public-key storage is denied; terminal cleanup remains possible.
			if !errors.Is(err, ErrHistoryState) {
				t.Fatal("stale linked history dispatched", err)
			}
			got, err := s.MobileBackupAttempt(ctx, attempt.OperationID)
			if err != nil || got.State != "prepared" || got.Revision != attempt.Revision || got.PublicKey != "" {
				t.Fatal("rejected dispatch mutated acquisition", err)
			}
			if _, err = s.ProgressMobileBackup(ctx, attempt.OperationID, got.Revision, "cancelled"); err != nil {
				t.Fatal("cleanup lost its ability to cancel acquisition", err)
			}
		})
	}
}

func TestMobileHistoryAcquisitionRestartCannotRedispatchOrAdoptUnlinkedAttempt(t *testing.T) {
	// Arrange: both journals and their permanent link are committed before dispatch.
	ctx := context.Background()
	s, op, path := acquisitionOperation(t)
	defer func() { s.Close() }()
	attempt, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Act: process restart recovers both ledgers, then retries the same operation.
	s.Close()
	s = historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	if err = s.RecoverMobileBackupAttempts(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverInterruptedHistory(ctx); err != nil {
		t.Fatal(err)
	}
	op, err = s.HistoryOperation(ctx, op.Status.OperationID)
	if err == nil {
		op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
	}
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
	// Assert: return the original interrupted attempt, without requiring a new reservation.
	if err != nil || got.OperationID != attempt.OperationID || got.State != "interrupted" || op.WorkDuration != 180*time.Second {
		t.Fatal("restart replaced or redispatched attempt", err)
	}
	countAcquisitionRows(t, s, 1, 1)
	if _, err = s.DispatchMobileBackup(ctx, got.OperationID, got.Revision, mobileJournalPublic(t)); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("interrupted acquisition dispatched", err)
	}

	// Arrange: a different store already has an unlinked owner-diagnostic UUID.
	other, pending, _ := acquisitionOperation(t)
	defer other.Close()
	r := domain.MobileBackupRequest{RequestID: pending.Status.RequestID, ConversationType: pending.Status.ConversationType, ConversationID: pending.Status.ConversationID, Since: pending.Status.Since, Until: pending.Status.Until, MaxMessages: pending.Status.MaxMessages}
	standalone, err := other.PrepareMobileBackup(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	// Act / Assert: ownership must not be adopted implicitly, even with matching arguments.
	if _, err = other.PrepareMobileHistoryAcquisition(ctx, pending.Status.OperationID, pending.Revision, 0); !errors.Is(err, ErrMobileBackupConflict) {
		t.Fatal("unlinked attempt adopted", err)
	}
	countAcquisitionRows(t, other, 1, 0)
	if _, err = other.DispatchMobileBackup(ctx, standalone.OperationID, standalone.Revision, mobileJournalPublic(t)); err != nil {
		t.Fatal("unlinked diagnostic behavior changed", err)
	}
}

func TestMobileHistoryAcquisitionMigrationIsAtomic(t *testing.T) {
	// Arrange: schema 13 exists, and recording schema 14 fails.
	s, _, _, _ := mobileOperation(t)
	defer s.Close()
	if _, err := s.DB.Exec(`DROP TABLE history_mobile_attempts;DELETE FROM schema_migrations WHERE version=14;CREATE TRIGGER reject_acquisition_migration BEFORE INSERT ON schema_migrations WHEN new.version=14 BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	// Act: execute the isolated acquisition migration.
	if err := s.migrateMobileHistoryAcquisition(context.Background()); err == nil {
		t.Fatal("migration failure ignored")
	}
	// Assert: neither table nor version was partially published.
	var tables, versions int
	if err := s.DB.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='history_mobile_attempts'").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow("SELECT count(*) FROM schema_migrations WHERE version=14").Scan(&versions); err != nil || tables != 0 || versions != 0 {
		t.Fatal("partial acquisition migration", err)
	}
}

func TestMobileHistoryAcquisitionScopeAndInputFailuresHaveNoSideEffects(t *testing.T) {
	for _, mode := range []string{"foreign-account", "revoked", "invalid-budget", "cancelled-context", "legacy-source"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: change one binding/input before preparation.
			ctx := context.Background()
			s, op, path := acquisitionOperation(t)
			defer func() { s.Close() }()
			budget := int64(0)
			switch mode {
			case "foreign-account":
				if _, err := s.DB.Exec("UPDATE metadata SET value='foreign-account-binding' WHERE key='account'"); err != nil {
					t.Fatal(err)
				}
			case "revoked":
				s.Close()
				s = historyOperationStore(t, path, domain.CollectionPolicy{})
			case "invalid-budget":
				budget = -1
			case "cancelled-context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "legacy-source":
				request := historyRequest()
				request.RequestID = "00000000-0000-4000-8000-000000000002"
				var err error
				op, err = s.PrepareHistoryOperation(ctx, request)
				if err == nil {
					op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
				}
				if err == nil {
					op, err = s.ReserveHistoryPage(ctx, op.Status.OperationID, op.Revision, 30*time.Second)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			// Act: every rejected preparation must leave no dispatchable prefix.
			_, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, budget)
			// Assert: both journals remain empty.
			if err == nil {
				t.Fatal("invalid acquisition scope accepted")
			}
			countAcquisitionRows(t, s, 0, 0)
		})
	}
	for _, mode := range []string{"foreign-account", "revoked"} {
		t.Run("dispatch-"+mode, func(t *testing.T) {
			// Arrange: access changes after the attempt is linked, before dispatch.
			ctx := context.Background()
			s, op, path := acquisitionOperation(t)
			defer func() { s.Close() }()
			attempt, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "foreign-account" {
				if _, err = s.DB.Exec("UPDATE metadata SET value='foreign-account-binding' WHERE key='account'"); err != nil {
					t.Fatal(err)
				}
			} else {
				s.Close()
				s = historyOperationStore(t, path, domain.CollectionPolicy{})
			}
			// Act: dispatch authority must be rechecked rather than borrowed from binding time.
			_, err = s.DispatchMobileBackup(ctx, attempt.OperationID, attempt.Revision, mobileJournalPublic(t))
			// Assert: denied dispatch does not save even its public key/state.
			if err == nil {
				t.Fatal("revoked acquisition dispatched")
			}
			var state, public string
			var revision int64
			if err = s.DB.QueryRow("SELECT state,public_key,revision FROM mobile_backup_attempts WHERE operation_id=?", attempt.OperationID).Scan(&state, &public, &revision); err != nil || state != "prepared" || public != "" || revision != attempt.Revision {
				t.Fatal("denied dispatch changed acquisition", err)
			}
			countAcquisitionRows(t, s, 1, 1)
		})
	}
}

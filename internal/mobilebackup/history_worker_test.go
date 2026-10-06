package mobilebackup

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type historyWorkerSource struct {
	session  *historyWorkerSession
	leaseErr error
}

func (s *historyWorkerSource) CleanupHistorySnapshots(ctx context.Context) error {
	return s.session.snapshots.CleanupTerminal(ctx, s.session.store.MobileHistorySnapshotTerminal)
}

func (s *historyWorkerSource) WithHistorySession(ctx context.Context, visit func(context.Context, historyimport.MobileHistorySession) error) error {
	if s.leaseErr != nil {
		return s.leaseErr
	}
	return visit(ctx, s.session)
}

type historyWorkerSession struct {
	store                  *storage.Store
	snapshots              *SnapshotStore
	selected               SelectedArchive
	scratch, public        string
	receives, saves, reads int
	saveErr                error
	receiveErr             error
	beforeRead             func(int) error
}

func stageDeadline(ctx context.Context, maximum time.Duration) bool {
	deadline, ok := ctx.Deadline()
	return ok && time.Until(deadline) > 0 && time.Until(deadline) <= maximum
}

func (s *historyWorkerSession) ReceiveOffer(ctx context.Context, id string) (domain.MobileBackupOffer, error) {
	if !stageDeadline(ctx, 180*time.Second) {
		return domain.MobileBackupOffer{}, errors.New("unbounded offer stage")
	}
	if s.receiveErr != nil {
		return domain.MobileBackupOffer{}, s.receiveErr
	}
	return RunPreparedOffer(ctx, s.store, offerSourceFunc(func(ctx context.Context, observer *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
		if err := observer.BeforeDispatch(s.public); err != nil {
			return domain.MobileBackupOffer{}, err
		}
		s.receives++
		if err := observer.Progress("waiting_for_confirmation"); err != nil {
			return domain.MobileBackupOffer{}, err
		}
		if err := observer.Progress("offer_ready"); err != nil {
			return domain.MobileBackupOffer{}, err
		}
		return domain.MobileBackupOffer{SessionAccountID: "10", URL: "https://example.invalid/synthetic", KeyText: "synthetic-key", FileSize: 16}, nil
	}), id)
}

func (s *historyWorkerSession) SaveOffer(ctx context.Context, r domain.MobileBackupRequest, offer domain.MobileBackupOffer) error {
	if !stageDeadline(ctx, 120*time.Second) || offer.SessionAccountID != "10" {
		return errors.New("invalid download stage")
	}
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.snapshots.Save(ctx, s.selected, r, "10")
}

func (s *historyWorkerSession) ReadHistoryPage(ctx context.Context, r domain.MobileBackupRequest, size, examined int, previous *domain.MobileHistorySnapshot, after *domain.MobileHistoryPosition) (domain.MobileHistoryPage, error) {
	if !stageDeadline(ctx, 30*time.Second) {
		return domain.MobileHistoryPage{}, errors.New("unbounded read stage")
	}
	s.reads++
	if s.beforeRead != nil {
		if err := s.beforeRead(s.reads); err != nil {
			return domain.MobileHistoryPage{}, err
		}
	}
	page, err := s.snapshots.ReadHistoryPage(ctx, r, "10", s.scratch, size, examined, previous, after, historyPageMapper())
	if errors.Is(err, ErrSnapshot) || errors.Is(err, ErrSnapshotConflict) {
		return domain.MobileHistoryPage{}, historyimport.ErrMobileHistorySourceUnavailable
	}
	if errors.Is(err, ErrSQLite) {
		return domain.MobileHistoryPage{}, domain.ErrHistoryInvalidPage
	}
	if errors.Is(err, ErrSnapshotSourceUnsupported) {
		return domain.MobileHistoryPage{}, domain.ErrHistoryUnsupported
	}
	return page, err
}

func historyWorkerFixture(t *testing.T) (*historyWorkerSource, storage.HistoryOperation, string, string, domain.MobileBackupRequest) {
	t.Helper()
	ctx := context.Background()
	selected, request := historyPageFixture(t)
	t.Cleanup(func() { selected.Clear() })
	path, dir := filepath.Join(t.TempDir(), "messages.sqlite"), filepath.Join(t.TempDir(), "snapshots")
	s, err := storage.OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err = s.BindAccount(ctx, "10"); err != nil {
		t.Fatal(err)
	}
	op, err := s.PrepareMobileHistoryOperation(ctx, domain.HistoryImportRequest{RequestID: request.RequestID, ConversationType: request.ConversationType, ConversationID: request.ConversationID, Since: request.Since, Until: request.Until, MaxMessages: request.MaxMessages, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, err := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	if err != nil {
		t.Fatal(err)
	}
	source := &historyWorkerSource{session: &historyWorkerSession{store: s, snapshots: openSnapshotStore(t, dir, 1<<20), selected: selected, scratch: t.TempDir(), public: base64.StdEncoding.EncodeToString(der)}}
	return source, op, path, dir, request
}

func startHistoryWorker(ctx context.Context, s *storage.Store, source historyimport.MobileHistorySource) <-chan error {
	done := make(chan error, 1)
	go func() { done <- historyimport.RunMobile(ctx, s, source) }()
	return done
}

func stopHistoryWorker(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("worker failed", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop")
	}
}

func waitMobileState(t *testing.T, s *storage.Store, id, state string) storage.HistoryOperation {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		op, err := s.HistoryOperation(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status.State == state {
			return op
		}
		if time.Now().After(deadline) {
			t.Fatal("mobile worker state deadline", op.Status.State)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func assertSilentWorker(t *testing.T, s *storage.Store, messages int) {
	t.Helper()
	for table, want := range map[string]int{"messages": messages, "message_events": 0, "event_deliveries": 0} {
		var n int
		if err := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != want {
			t.Fatal("worker message/notification side effect", table, n, err)
		}
	}
}

func TestMobileWorkerFullAcquisitionSnapshotAndPagingCycle(t *testing.T) {
	// Arrange: a real encrypted snapshot/reader/converter/journal, with a synthetic receiver.
	source, pending, _, _, _ := historyWorkerFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Act: worker owns receive, save and all pages under the same session callback.
	done := startHistoryWorker(ctx, source.session.store, source)
	op := waitMobileState(t, source.session.store, pending.Status.OperationID, "partial")
	stopHistoryWorker(t, cancel, done)
	// Assert: one attempt, one download, three bounded pages and silent original-TTL records.
	if source.session.receives != 1 || source.session.saves != 1 || source.session.reads != 3 || op.Status.StopReason == nil || *op.Status.StopReason != "source_gaps" || op.Status.PagesObserved != 3 || op.Status.RecordsObserved != 5 || op.Status.InsertedCount != 2 || op.ReservedDuration != 0 || op.WorkDuration <= 0 || op.Status.HistoryComplete {
		t.Fatal("mobile worker did not preserve bounded source coverage")
	}
	attempt, err := source.session.store.MobileHistoryAcquisition(context.Background(), op.Status.OperationID)
	if err != nil || attempt.State != "offer_ready" {
		t.Fatal("worker lost its only acquisition", err)
	}
	assertSilentWorker(t, source.session.store, 2)
}

func TestMobileWorkerRestartCleansTerminalSourceWithoutSession(t *testing.T) {
	// Arrange: crash after a terminal journal commit, before removing its image.
	source, pending, _, dir, request := historyWorkerFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := source.session.store
	op, err := store.ClaimHistoryOperation(ctx, pending.Status.OperationID, pending.Revision)
	if err == nil {
		op, err = store.ReserveMobileHistoryWork(ctx, op.Status.OperationID, op.Revision, 180*time.Second)
	}
	if err == nil {
		_, err = store.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = source.session.snapshots.Save(ctx, source.session.selected, request, "10"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.StopHistoryOperation(ctx, op.Status.OperationID, op.Revision, "partial", "source_unavailable", 0); err != nil {
		t.Fatal(err)
	}
	if err = source.session.snapshots.Close(); err != nil {
		t.Fatal(err)
	}
	source.session.snapshots = openSnapshotStore(t, dir, 1<<20)
	if err = store.RecoverMobileBackupAttempts(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.RecoverInterruptedHistory(ctx); err != nil {
		t.Fatal(err)
	}
	source.leaseErr = domain.ErrAuthenticationRequired
	// Act: cleanup runs before queue processing without borrowing upstream authority.
	done := startHistoryWorker(ctx, store, source)
	name, _ := snapshotName(request.RequestID)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err = os.Stat(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil || time.Now().After(deadline) {
			cancel()
			stopHistoryWorker(t, cancel, done)
			t.Fatal("terminal image survived restart", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopHistoryWorker(t, cancel, done)
	// Assert: no new session/offer/read; cleanup retains the spent snapshot identity.
	if source.session.receives != 0 || source.session.saves != 0 || source.session.reads != 0 {
		t.Fatal("local cleanup borrowed network authority")
	}
	if err = source.session.snapshots.Save(context.Background(), source.session.selected, request, "10"); !errors.Is(err, ErrSnapshot) {
		t.Fatal("restart cleanup renewed source", err)
	}
	assertSilentWorker(t, store, 0)
}

func TestMobileWorkerCompletedImportRemovesSource(t *testing.T) {
	// Arrange: one valid text row and a real encrypted snapshot/reader/journal cycle.
	source, pending, _, dir, request := historyWorkerFixture(t)
	source.session.selected.Clear()
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		metadata := attachmentField(6, append(attachmentField(45, []byte("rtf")), attachmentField(47, []byte("synthetic complete title"))...))
		_, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", "13", "14", "synthetic complete import", time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC).UnixMilli(), 0, 0, 1, metadata)
		if err != nil {
			t.Fatal(err)
		}
	})
	source.session.selected = SelectedArchive{requestID: request.RequestID, requestFingerprint: request.Fingerprint(), ref: request.Ref(), File: ArchiveFile{Name: "902.db", Data: data}}
	defer source.session.selected.Clear()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Act: let the operation and its local terminal cleanup finish before shutdown.
	done := startHistoryWorker(ctx, source.session.store, source)
	op := waitMobileState(t, source.session.store, pending.Status.OperationID, "completed")
	name, _ := snapshotName(request.RequestID)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil || time.Now().After(deadline) {
			stopHistoryWorker(t, cancel, done)
			t.Fatal("completed source retained", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	stopHistoryWorker(t, cancel, done)
	// Assert: source exhaustion is limited, silent and does not retain its staging file.
	if op.Status.InsertedCount != 1 || op.Status.PagesObserved != 1 || op.Status.HistoryComplete || source.session.receives != 1 || source.session.saves != 1 || source.session.reads != 1 {
		t.Fatal("completed cleanup/import mismatch", op.Status)
	}
	var text, attachments, provenance string
	if err := source.session.store.DB.QueryRow("SELECT text,attachments,source FROM messages WHERE message_id='13'").Scan(&text, &attachments, &provenance); err != nil || text != "synthetic complete title" || attachments != `["rtf"]` || provenance != "history" {
		t.Fatal("visible rich-text projection was not silently preserved", err)
	}
	assertSilentWorker(t, source.session.store, 1)
}

func TestMobileWorkerRestartsFromExactSnapshotWithoutAnotherPhone(t *testing.T) {
	// Arrange: stop after a committed gap-only first page, with the next read in flight.
	source, pending, path, dir, _ := historyWorkerFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source.session.beforeRead = func(page int) error {
		if page == 2 {
			cancel()
			return ctx.Err()
		}
		return nil
	}
	done := startHistoryWorker(ctx, source.session.store, source)
	stopHistoryWorkerAfterDone := func() {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("worker did not reach interrupted read")
		}
	}
	stopHistoryWorkerAfterDone()
	op, err := source.session.store.HistoryOperation(context.Background(), pending.Status.OperationID)
	if err != nil || op.Status.PagesObserved != 1 || op.Status.RecordsObserved != 2 || op.ReservedDuration != 30*time.Second {
		t.Fatal("shutdown lost the committed gap page", err)
	}
	before, err := source.session.store.MobileHistoryCheckpoint(context.Background(), op.Status.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	// Act: recover both journals and reopen encrypted bytes, then run again.
	source.session.store.Close()
	source.session.snapshots.Close()
	s, err := storage.OpenWithPolicy(context.Background(), path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err = s.BindAccount(context.Background(), "10"); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverMobileBackupAttempts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.RecoverInterruptedHistory(context.Background()); err != nil {
		t.Fatal(err)
	}
	source.session.store, source.session.snapshots = s, openSnapshotStore(t, dir, 1<<20)
	source.session.beforeRead = nil
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	done = startHistoryWorker(ctx, s, source)
	op = waitMobileState(t, s, op.Status.OperationID, "partial")
	stopHistoryWorker(t, cancel, done)
	// Assert: exact source lifetime/identity survives, and only the remaining pages are read.
	after, err := s.MobileHistoryCheckpoint(context.Background(), op.Status.OperationID)
	if err != nil || after.Snapshot != before.Snapshot || source.session.receives != 1 || source.session.saves != 1 || source.session.reads != 4 || op.Status.RecordsObserved != 5 || op.Status.PagesObserved != 3 || op.Status.InsertedCount != 2 || op.WorkDuration < 30*time.Second {
		t.Fatal("restart renewed or redispatched source", err)
	}
	assertSilentWorker(t, s, 2)
}

func TestMobileWorkerLossAndCancellationDoNotRedispatchOrImportPrefixes(t *testing.T) {
	for _, mode := range []string{"prepared-without-snapshot", "receive-failure", "save-failure", "lost-after-page", "cancel-before-commit", "invalid-page", "wal-source"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: one known source/failure boundary.
			source, pending, _, _, request := historyWorkerFixture(t)
			s := source.session.store
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "prepared-without-snapshot" {
				op, err := s.ClaimHistoryOperation(ctx, pending.Status.OperationID, pending.Revision)
				if err == nil {
					op, err = s.ReserveMobileHistoryWork(ctx, op.Status.OperationID, op.Revision, 180*time.Second)
				}
				if err == nil {
					_, err = s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
				}
				if err == nil {
					err = s.RecoverInterruptedHistory(ctx)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode == "save-failure" {
				source.session.saveErr = historyimport.ErrMobileHistorySourceUnavailable
			}
			if mode == "receive-failure" {
				source.session.receiveErr = domain.ErrMobileBackupRejected
			}
			if mode == "wal-source" {
				source.session.selected.File.Data[18], source.session.selected.File.Data[19] = 2, 2
			}
			source.session.beforeRead = func(page int) error {
				if mode == "lost-after-page" && page == 2 {
					return source.session.snapshots.Remove(ctx, request, "10")
				}
				if mode == "cancel-before-commit" {
					_, err := s.CancelHistoryOperation(ctx, pending.Status.OperationID)
					return err
				}
				if mode == "invalid-page" {
					return domain.ErrHistoryInvalidPage
				}
				return nil
			}
			// Act: process once; source loss/cancellation must not retry the receiver.
			done := startHistoryWorker(ctx, s, source)
			state := "partial"
			if mode == "cancel-before-commit" {
				state = "cancelled"
			}
			if mode == "invalid-page" {
				state = "failed"
			}
			if mode == "wal-source" {
				state = "unsupported"
			}
			op := waitMobileState(t, s, pending.Status.OperationID, state)
			stopHistoryWorker(t, cancel, done)
			// Assert: only the first gap page can survive; no prefix or historical Event is stored.
			wantPages, wantReceives := 0, 1
			if mode == "lost-after-page" {
				wantPages = 1
			}
			if mode == "prepared-without-snapshot" || mode == "receive-failure" {
				wantReceives = 0
			}
			if op.Status.PagesObserved != wantPages || source.session.receives != wantReceives || op.Status.InsertedCount != 0 {
				t.Fatal("failed worker guessed a source prefix or repeated receiver")
			}
			if state == "partial" && (op.Status.StopReason == nil || *op.Status.StopReason != "source_unavailable") {
				t.Fatal("source loss not reported")
			}
			assertSilentWorker(t, s, 0)
			if mode == "receive-failure" || mode == "prepared-without-snapshot" {
				attempt, err := s.MobileHistoryAcquisition(context.Background(), op.Status.OperationID)
				if err != nil || attempt.State != "interrupted" {
					t.Fatal("inactive attempt kept the account transfer locked", err)
				}
			}
		})
	}
}

func TestMobileWorkerUnauthenticatedLeasePausesBeforePhone(t *testing.T) {
	// Arrange: there is no permitted authenticated session lease.
	source, pending, _, _, _ := historyWorkerFixture(t)
	source.leaseErr = domain.ErrAuthenticationRequired
	// Act: authentication loss ends the worker instead of polling credentials.
	err := historyimport.RunMobile(context.Background(), source.session.store, source)
	// Assert: no acquisition/snapshot or active work occurred.
	op, readErr := source.session.store.HistoryOperation(context.Background(), pending.Status.OperationID)
	if !errors.Is(err, domain.ErrAuthenticationRequired) || readErr != nil || op.Status.State != "paused" || op.WorkDuration != 0 || op.ReservedDuration != 0 || source.session.receives != 0 || source.session.saves != 0 || source.session.reads != 0 {
		t.Fatal("unauthenticated worker performed source work", err, readErr)
	}
	if _, err = source.session.store.MobileHistoryAcquisition(context.Background(), op.Status.OperationID); err == nil {
		t.Fatal("authentication waiting created an acquisition")
	}
}

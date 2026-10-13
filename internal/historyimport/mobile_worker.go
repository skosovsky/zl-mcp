package historyimport

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

var ErrMobileHistorySourceUnavailable = errors.New("mobile history source unavailable")

// MobileHistorySession lends the existing authenticated session across all stages.
// SaveOffer publishes the exact selected encrypted snapshot before returning.
type MobileHistorySession interface {
	ReceiveOffer(context.Context, string) (domain.MobileBackupOffer, error)
	SaveOffer(context.Context, domain.MobileBackupRequest, domain.MobileBackupOffer) error
	ReadHistoryPage(
		context.Context,
		domain.MobileBackupRequest,
		int,
		int,
		*domain.MobileHistorySnapshot,
		*domain.MobileHistoryPosition,
	) (domain.MobileHistoryPage, error)
}

type MobileHistorySource interface {
	WithHistorySession(context.Context, func(context.Context, MobileHistorySession) error) error
	CleanupHistorySnapshots(context.Context) error
}

// RunMobile runs after the single owner has recovered both journals. It performs
// no recovery itself, so it cannot reset concurrently running legacy imports.
func RunMobile(ctx context.Context, store *storage.Store, source MobileHistorySource) error {
	if ctx == nil || store == nil {
		return errors.New("mobile history worker requires context and store")
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		continueWorker, cleanupErr := cleanupMobileSource(ctx, source)
		if cleanupErr != nil || !continueWorker {
			return cleanupErr
		}
		pending, err := store.PendingMobileHistoryOperations(ctx, 20)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		for _, op := range pending {
			continueWorker, operationErr := runMobilePending(ctx, store, source, op)
			if operationErr != nil || !continueWorker {
				return operationErr
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func runMobileOperation(
	ctx context.Context,
	store *storage.Store,
	source MobileHistorySource,
	pending storage.HistoryOperation,
) error {
	return shared.ObserveWorker(
		ctx,
		domain.WorkHistory,
		pending.Status.OperationID,
		func(ctx context.Context) error { return runMobileOperationObserved(ctx, store, source, pending) },
	)
}

func runMobileOperationObserved(
	ctx context.Context,
	store *storage.Store,
	source MobileHistorySource,
	pending storage.HistoryOperation,
) error {
	op, err := store.ClaimHistoryOperation(ctx, pending.Status.OperationID, pending.Revision)
	if ctx.Err() != nil || errors.Is(err, storage.ErrHistoryState) {
		//nolint:nilerr // Claim races and cancellation stop this worker successfully; storage failures are returned separately.
		return nil
	}
	if err != nil || op.Status.State != "running" {
		return err
	}
	created, _ := time.Parse(time.RFC3339Nano, op.Status.CreatedAt)
	shared.ObserveTransition(ctx, domain.WorkHistory, pending.Status.State, op.Status.State, created)
	work := &mobileHistoryWork{ctx: ctx, store: store, op: op, created: created, storageFailed: false}

	if source == nil {
		return work.stop("unsupported", "source_unsupported", 0)
	}

	err = source.WithHistorySession(ctx, work.runSession)
	if ctx.Err() != nil {
		//nolint:nilerr // Claim races and cancellation stop this worker successfully; storage failures are returned separately.
		return nil
	}
	if errors.Is(err, storage.ErrHistoryState) || work.op.Status.State != "running" {
		if e := work.store.CloseMobileHistoryAcquisition(
			ctx,
			work.op.Status.OperationID,
		); e != nil &&
			!errors.Is(e, storage.ErrHistoryState) {
			return e
		}
		return nil
	}
	if work.storageFailed {
		return err
	}
	if err == nil {
		return errors.New("mobile history session ended with unfinished work")
	}
	state, reason := "partial", "source_unavailable"
	switch {
	case errors.Is(err, domain.ErrAuthenticationRequired):
		if e := work.stop("paused", "auth_required", work.op.ReservedDuration); e != nil {
			return e
		}
		return domain.ErrAuthenticationRequired
	case errors.Is(err, domain.ErrHistoryUnsupported):
		state, reason = "unsupported", "source_unsupported"
	case errors.Is(err, domain.ErrHistoryInvalidPage):
		state, reason = "failed", "invalid_source_page"
	}
	return work.stop(state, reason, work.op.ReservedDuration)
}

type mobileHistoryWork struct {
	ctx           context.Context
	store         *storage.Store
	op            storage.HistoryOperation
	created       time.Time
	storageFailed bool
}

func (work *mobileHistoryWork) stop(state, reason string, elapsed time.Duration) error {
	remaining := max(storage.MobileHistoryWorkBudget-work.op.WorkDuration, 0)
	next, e := work.store.StopHistoryOperation(
		work.ctx,
		work.op.Status.OperationID,
		work.op.Revision,
		state,
		reason,
		min(elapsed, remaining),
	)
	if e == nil {
		shared.ObserveTransition(work.ctx, domain.WorkHistory, work.op.Status.State, next.Status.State, work.created)
		work.op = next
	}
	if work.ctx.Err() != nil || errors.Is(e, storage.ErrHistoryState) {
		//nolint:nilerr // Claim races and cancellation stop this worker successfully; storage failures are returned separately.
		return nil
	}
	if e == nil {
		e = work.store.CloseMobileHistoryAcquisition(work.ctx, work.op.Status.OperationID)
	}
	return e
}

func (work *mobileHistoryWork) reserve(stage context.Context, maximum time.Duration) (time.Duration, error) {
	remaining := storage.MobileHistoryWorkBudget - work.op.WorkDuration
	if remaining <= 0 {
		return 0, work.stop("partial", "time_limit", 0)
	}
	budget := min(maximum, remaining)
	next, e := work.store.ReserveMobileHistoryWork(stage, work.op.Status.OperationID, work.op.Revision, budget)
	if e != nil {
		work.storageFailed = !errors.Is(e, storage.ErrHistoryState)
		return 0, e
	}
	work.op = next
	if work.op.Status.State != "running" {
		return 0, nil
	}
	return budget, nil
}

func (work *mobileHistoryWork) complete(stage context.Context, started time.Time) error {
	next, e := work.store.CompleteMobileHistoryWork(
		stage,
		work.op.Status.OperationID,
		work.op.Revision,
		min(time.Since(started), storage.MobileHistoryWorkBudget),
	)
	if e == nil {
		work.op = next
	}
	return e
}

func (work *mobileHistoryWork) runSession(sessionCtx context.Context, session MobileHistorySession) error {
	if sessionCtx == nil || session == nil {
		return domain.ErrHistoryUnsupported
	}
	attempt, ready, err := work.prepareAcquisition(sessionCtx, session)
	if err != nil || !ready {
		return err
	}
	for work.op.Status.State == "running" {
		continueWork, pageErr := work.readPage(sessionCtx, session, attempt)
		if pageErr != nil || !continueWork {
			return pageErr
		}
	}
	return nil
}

func (work *mobileHistoryWork) prepareAcquisition(sessionCtx context.Context, session MobileHistorySession) (storage.MobileBackupAttempt, bool, error) {
	attempt, e := work.store.MobileHistoryAcquisition(sessionCtx, work.op.Status.OperationID)
	if !errors.Is(e, sql.ErrNoRows) {
		if e != nil {
			work.storageFailed = !errors.Is(e, storage.ErrHistoryState)
			return attempt, false, e
		}
		return attempt, true, nil
	}
	budget, reserveErr := work.reserve(sessionCtx, 180*time.Second)
	if reserveErr != nil || budget == 0 {
		return attempt, false, reserveErr
	}
	attempt, reserveErr = work.store.PrepareMobileHistoryAcquisition(sessionCtx, work.op.Status.OperationID, work.op.Revision, 0)
	if reserveErr != nil {
		work.storageFailed = !errors.Is(reserveErr, storage.ErrHistoryState)
		return attempt, false, reserveErr
	}
	request, cancel := context.WithTimeout(sessionCtx, budget)
	started := time.Now()
	offer, receiveErr := shared.ObserveUpstream(
		request,
		domain.WorkHistory,
		func(ctx context.Context) (domain.MobileBackupOffer, error) {
			return session.ReceiveOffer(ctx, attempt.OperationID)
		},
	)
	cancel()
	defer func() { offer = domain.MobileBackupOffer{} }()
	if sessionCtx.Err() != nil {
		return attempt, false, sessionCtx.Err()
	}
	if reserveErr = work.complete(sessionCtx, started); reserveErr != nil {
		work.storageFailed = !errors.Is(reserveErr, storage.ErrHistoryState)
		return attempt, false, reserveErr
	}
	if receiveErr != nil || work.op.Status.State != "running" {
		return attempt, false, receiveErr
	}
	budget, reserveErr = work.reserve(sessionCtx, 120*time.Second)
	if reserveErr != nil || budget == 0 {
		return attempt, false, reserveErr
	}
	request, cancel = context.WithTimeout(sessionCtx, budget)
	started = time.Now()
	saveErr := func() error {
		_, observeUpstreamErr := shared.ObserveUpstream(
			request,
			domain.WorkHistory,
			func(ctx context.Context) (struct{}, error) {
				return struct{}{}, session.SaveOffer(ctx, attempt.Request, offer)
			},
		)
		return observeUpstreamErr
	}()
	cancel()
	offer = domain.MobileBackupOffer{}
	if sessionCtx.Err() != nil {
		return attempt, false, sessionCtx.Err()
	}
	if reserveErr = work.complete(sessionCtx, started); reserveErr != nil {
		work.storageFailed = !errors.Is(reserveErr, storage.ErrHistoryState)
		return attempt, false, reserveErr
	}
	if saveErr != nil || work.op.Status.State != "running" {
		return attempt, false, saveErr
	}
	return attempt, true, nil
}

func (work *mobileHistoryWork) readPage(sessionCtx context.Context, session MobileHistorySession, attempt storage.MobileBackupAttempt) (bool, error) {
	budget, e := work.reserve(sessionCtx, 30*time.Second)
	if e != nil || budget == 0 {
		return false, e
	}
	var previous *domain.MobileHistorySnapshot
	var after *domain.MobileHistoryPosition
	if work.op.Status.PagesObserved > 0 || work.op.Status.MobileCoverage != nil {
		checkpoint, mobileHistoryCheckpointErr := work.store.MobileHistoryCheckpoint(sessionCtx, work.op.Status.OperationID)
		if mobileHistoryCheckpointErr != nil {
			work.storageFailed = !errors.Is(mobileHistoryCheckpointErr, storage.ErrHistoryState)
			return false, mobileHistoryCheckpointErr
		}
		previous, after = &checkpoint.Snapshot, checkpoint.Next
	}
	request, cancel := context.WithTimeout(sessionCtx, budget)
	started := time.Now()
	page, readErr := shared.ObserveUpstream(
		request,
		domain.WorkHistory,
		func(ctx context.Context) (domain.MobileHistoryPage, error) {
			return session.ReadHistoryPage(
				ctx,
				attempt.Request,
				work.op.Status.PageSize,
				work.op.Status.RecordsObserved,
				previous,
				after,
			)
		},
	)
	cancel()
	elapsed := time.Since(started)
	if readErr != nil || sessionCtx.Err() != nil {
		page.Clear()
		return false, work.finishReadFailure(sessionCtx, started, readErr)
	}
	if commitErr := work.commitPage(sessionCtx, &page, elapsed, started); commitErr != nil {
		return false, commitErr
	}
	return true, nil
}

func (work *mobileHistoryWork) commitPage(sessionCtx context.Context, page *domain.MobileHistoryPage, elapsed time.Duration, started time.Time) error {
	next, e := work.store.CommitMobileHistoryPage(
		sessionCtx,
		work.op.Status.OperationID,
		work.op.Revision,
		*page,
		min(elapsed, storage.MobileHistoryWorkBudget),
	)
	page.Clear()
	if e != nil {
		return work.commitPageFailure(sessionCtx, started, e)
	}
	if work.op.Status.State != next.Status.State {
		shared.ObserveTransition(work.ctx, domain.WorkHistory, work.op.Status.State, next.Status.State, work.created)
	}
	work.op = next
	return nil
}

func (work *mobileHistoryWork) finishReadFailure(sessionCtx context.Context, started time.Time, readErr error) error {
	if sessionCtx.Err() != nil {
		return sessionCtx.Err()
	}
	if e := work.complete(sessionCtx, started); e != nil {
		work.storageFailed = !errors.Is(e, storage.ErrHistoryState)
		return e
	}
	return readErr
}

func runMobilePending(ctx context.Context, store *storage.Store, source MobileHistorySource, op storage.HistoryOperation) (bool, error) {
	if err := runMobileOperation(ctx, store, source, op); err != nil {
		// Owner shutdown can race a final operation-journal write after
		// that operation's last context check. As with pending reads and
		// snapshot cleanup, cancellation ends this worker normally.
		if ctx.Err() != nil {
			//nolint:nilerr // Owner shutdown is normal termination; operation errors remain failures while the owner context is active.
			return false, nil
		}
		return false, err
	}
	if source != nil && ctx.Err() == nil {
		if err := source.CleanupHistorySnapshots(ctx); err != nil {
			if ctx.Err() != nil {
				//nolint:nilerr // Owner shutdown is normal termination; operation errors remain failures while the owner context is active.
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
}

func cleanupMobileSource(ctx context.Context, source MobileHistorySource) (bool, error) {
	if source == nil {
		return true, nil
	}
	if err := source.CleanupHistorySnapshots(ctx); err != nil {
		if ctx.Err() != nil {
			//nolint:nilerr // Owner cancellation ends snapshot cleanup normally; active storage errors are returned.
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (work *mobileHistoryWork) commitPageFailure(sessionCtx context.Context, started time.Time, e error) error {
	var invalid *domain.Error
	if errors.As(e, &invalid) && invalid.Code == "INVALID_ARGUMENT" ||
		errors.Is(e, storage.ErrMobileHistorySource) {
		sourceErr := domain.ErrHistoryInvalidPage
		if errors.Is(e, storage.ErrMobileHistorySource) {
			sourceErr = ErrMobileHistorySourceUnavailable
		}
		if e = work.complete(sessionCtx, started); e != nil {
			work.storageFailed = !errors.Is(e, storage.ErrHistoryState)
			return e
		}
		return sourceErr
	}
	work.storageFailed = !errors.Is(e, storage.ErrHistoryState)
	return e
}

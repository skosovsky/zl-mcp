// Package historyimport coordinates explicit, silent historical imports.
package historyimport

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// Run owns one worker under the existing service account lock/session. Auth
// loss ends this session's worker; a new authenticated session resumes pauses.
func Run(ctx context.Context, store *storage.Store, source domain.HistorySource) error {
	if err := store.RecoverInterruptedHistory(ctx); err != nil {
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			return nil
		}
		return err
	}
	return RunRecovered(ctx, store, source)
}

// RunRecovered consumes legacy operations after the single owner has recovered
// history once, before any legacy or mobile worker is started.
func RunRecovered(ctx context.Context, store *storage.Store, source domain.HistorySource) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		pending, err := store.PendingHistoryOperations(ctx, 20)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		for _, op := range pending {
			if err = runOperation(ctx, store, source, op); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func runOperation(
	ctx context.Context,
	store *storage.Store,
	source domain.HistorySource,
	pending storage.HistoryOperation,
) error {
	return shared.ObserveWorker(
		ctx,
		domain.WorkHistory,
		pending.Status.OperationID,
		func(ctx context.Context) error { return runOperationObserved(ctx, store, source, pending) },
	)
}

func runOperationObserved(
	ctx context.Context,
	store *storage.Store,
	source domain.HistorySource,
	pending storage.HistoryOperation,
) error {
	op, err := store.ClaimHistoryOperation(ctx, pending.Status.OperationID, pending.Revision)
	if errors.Is(err, storage.ErrHistoryState) || ctx.Err() != nil {
		//nolint:nilerr // Claim races and cancellation stop this worker successfully; storage failures are returned separately.
		return nil
	}
	if err != nil {
		return err
	}
	created, _ := time.Parse(time.RFC3339Nano, op.Status.CreatedAt)
	shared.ObserveTransition(ctx, domain.WorkHistory, pending.Status.State, op.Status.State, created)
	work := &historyWork{ctx: ctx, store: store, source: source, op: op, created: created}
	for work.op.Status.State == "running" {
		continueWork, stepErr := work.step()
		if stepErr != nil || !continueWork {
			return stepErr
		}
	}
	return nil
}

func logSourceFailure(operationID string, err error) {
	category := "unknown"
	attributes := []any{"operation_id", operationID}
	var source *domain.HistorySourceFailure
	var network net.Error
	switch {
	case errors.Is(err, domain.ErrAuthenticationRequired):
		category = "auth_required"
	case errors.Is(err, domain.ErrHistoryUnsupported):
		category = "source_unsupported"
	case errors.Is(err, domain.ErrHistoryInvalidPage):
		category = "invalid_source_page"
		if malformed, ok := errors.AsType[*domain.HistoryPageFailure](err); ok {
			attributes = append(attributes, "reason", malformed.Reason())
		}
	case errors.Is(err, context.DeadlineExceeded):
		category = "timeout"
	case errors.As(err, &source):
		category = "api_error"
		if source.APICode != nil {
			attributes = append(attributes, "source_code", *source.APICode)
		}
	case errors.As(err, &network):
		category = "network"
	}
	attributes = append(attributes, "category", category)
	// Never log err.Error(): SDK errors may contain URLs or private response text.
	slog.Warn("History source request failed", attributes...)
}

func requestHistoryPage(ctx context.Context, source domain.HistorySource, op storage.HistoryOperation, limit int) (domain.HistoryPage, error, bool) {
	if op.Status.Source == "conversation_preload" {
		snapshot, ok := source.(domain.PreloadHistorySource)
		if !ok {
			return domain.HistoryPage{}, domain.ErrHistoryUnsupported, false
		}
		page, err := shared.ObserveUpstream(ctx, domain.WorkHistory, func(ctx context.Context) (domain.HistoryPage, error) {
			return snapshot.PreloadHistoryPage(ctx, op.Status.Ref(), limit)
		})
		return page, err, false
	}
	if source == nil {
		return domain.HistoryPage{}, domain.ErrHistoryUnsupported, false
	}
	if phased, ok := source.(domain.PhasedHistorySource); ok {
		if phased.HistoryPhaseRequired() && op.Status.PagesObserved > 0 && op.Status.IsOld == nil {
			return domain.HistoryPage{}, nil, true
		}
		page, err := shared.ObserveUpstream(ctx, domain.WorkHistory, func(ctx context.Context) (domain.HistoryPage, error) {
			return phased.HistoryPageWithPhase(ctx, op.Status.Ref(), op.Cursor, op.Status.IsOld, limit)
		})
		return page, err, false
	}
	page, err := shared.ObserveUpstream(ctx, domain.WorkHistory, func(ctx context.Context) (domain.HistoryPage, error) {
		return source.HistoryPage(ctx, op.Status.Ref(), op.Cursor, limit)
	})
	return page, err, false
}

type historyWork struct {
	ctx     context.Context
	store   *storage.Store
	source  domain.HistorySource
	op      storage.HistoryOperation
	created time.Time
}

func (work *historyWork) stop(state, reason string, elapsed time.Duration) error {

	_, stopHistoryOperationErr := work.store.StopHistoryOperation(
		work.ctx,
		work.op.Status.OperationID,
		work.op.Revision,
		state,
		reason,
		min(elapsed, storage.HistoryWorkBudget),
	)
	if stopHistoryOperationErr == nil {
		shared.ObserveTransition(work.ctx, domain.WorkHistory, work.op.Status.State, state, work.created)
	}
	if errors.Is(stopHistoryOperationErr, storage.ErrHistoryState) || work.ctx.Err() != nil {
		//nolint:nilerr // Claim races and cancellation stop this worker successfully; storage failures are returned separately.
		return nil
	}
	return stopHistoryOperationErr

}

func (work *historyWork) step() (bool, error) {
	var err error
	ctx := work.ctx
	if ctx.Err() != nil {
		//nolint:nilerr // Claim races and cancellation stop this worker successfully; storage failures are returned separately.
		return false, nil
	}
	remaining := storage.HistoryWorkBudget - work.op.WorkDuration
	if remaining <= 0 {
		return false, work.stop("partial", "time_limit", 0)
	}
	limit := min(work.op.Status.PageSize, work.op.Status.MaxMessages-work.op.Status.RecordsObserved)
	if limit <= 0 {
		return false, errors.New("running history operation has exhausted its record budget")
	}
	requestBudget := min(remaining, 30*time.Second)
	work.op, err = work.store.ReserveHistoryPage(ctx, work.op.Status.OperationID, work.op.Revision, requestBudget)
	if errors.Is(err, storage.ErrHistoryState) || ctx.Err() != nil {
		//nolint:nilerr // Claim races and cancellation stop this worker successfully; storage failures are returned separately.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if work.op.Status.State != "running" {
		return false, nil
	}
	request, cancel := context.WithTimeout(ctx, requestBudget)
	started := time.Now()
	page, requestErr, missingContinuation := requestHistoryPage(request, work.source, work.op, limit)
	err = requestErr
	if missingContinuation {
		cancel()
		return false, work.stop("partial", "missing_continuation", 0)
	}
	elapsed := time.Since(started)
	cancel()
	if ctx.Err() != nil {
		//nolint:nilerr // Claim races and cancellation stop this worker successfully; storage failures are returned separately.
		return false, nil // running checkpoint is recoverable; never fabricate success.
	}
	if err != nil {
		return false, work.sourceFailure(err, remaining, elapsed)
	}
	commitBudget := remaining - time.Since(started)
	if commitBudget <= 0 {
		return false, work.stop("partial", "time_limit", elapsed)
	}
	commit, commitCancel := context.WithTimeout(ctx, commitBudget)
	updated, commitErr := work.store.CommitHistoryOperationPage(
		commit,
		work.op.Status.OperationID,
		work.op.Revision,
		page,
		min(elapsed, storage.HistoryWorkBudget),
	)
	commitCancel()
	if errors.Is(commitErr, storage.ErrHistoryState) || ctx.Err() != nil {
		//nolint:nilerr // Claim races and cancellation stop this worker successfully; storage failures are returned separately.
		return false, nil
	}
	if commitErr != nil {
		return false, work.commitFailure(commitErr, elapsed, started)
	}
	if work.op.Status.State != updated.Status.State {
		shared.ObserveTransition(ctx, domain.WorkHistory, work.op.Status.State, updated.Status.State, work.created)
	}
	work.op = updated
	return true, nil
}

func (work *historyWork) sourceFailure(err error, remaining, elapsed time.Duration) error {
	logSourceFailure(work.op.Status.OperationID, err)
	state, reason := "failed", "upstream_unavailable"
	var invalid *domain.Error
	switch {
	case errors.Is(err, domain.ErrAuthenticationRequired):
		if e := work.stop("paused", "auth_required", elapsed); e != nil {
			return e
		}
		return domain.ErrAuthenticationRequired
	case errors.Is(err, domain.ErrHistoryUnsupported):
		state, reason = "unsupported", "source_unsupported"
	case errors.Is(err, context.DeadlineExceeded) && remaining <= 30*time.Second:
		state, reason = "partial", "time_limit"
	case errors.As(err, &invalid) && invalid.Code == "INVALID_ARGUMENT":
		reason = "invalid_source_page"
	case errors.Is(err, domain.ErrHistoryInvalidPage):
		reason = "invalid_source_page"
	}
	return work.stop(state, reason, elapsed)

}

func (work *historyWork) commitFailure(commitErr error, elapsed time.Duration, started time.Time) error {
	if errors.Is(commitErr, context.DeadlineExceeded) {
		return work.stop("partial", "time_limit", time.Since(started))
	}
	reason := "storage_error"
	var invalid *domain.Error
	if errors.As(commitErr, &invalid) && invalid.Code == "INVALID_ARGUMENT" {
		reason = "invalid_source_page"
	}
	return work.stop("failed", reason, elapsed)

}

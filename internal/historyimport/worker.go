// Package historyimport coordinates explicit, silent historical imports.
package historyimport

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// Run owns one worker under the existing service account lock/session. Auth
// loss ends this session's worker; a new authenticated session resumes pauses.
func Run(ctx context.Context, store *storage.Store, source domain.HistorySource) error {
	if err := store.RecoverInterruptedHistory(ctx); err != nil {
		return err
	}
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

func runOperation(ctx context.Context, store *storage.Store, source domain.HistorySource, pending storage.HistoryOperation) error {
	op, err := store.ClaimHistoryOperation(ctx, pending.Status.OperationID, pending.Revision)
	if errors.Is(err, storage.ErrHistoryState) || ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	stop := func(state, reason string, elapsed time.Duration) error {
		_, err := store.StopHistoryOperation(ctx, op.Status.OperationID, op.Revision, state, reason, min(elapsed, storage.HistoryWorkBudget))
		if errors.Is(err, storage.ErrHistoryState) || ctx.Err() != nil {
			return nil
		}
		return err
	}
	for op.Status.State == "running" {
		if ctx.Err() != nil {
			return nil
		}
		remaining := storage.HistoryWorkBudget - op.WorkDuration
		if remaining <= 0 {
			return stop("partial", "time_limit", 0)
		}
		limit := min(op.Status.PageSize, op.Status.MaxMessages-op.Status.RecordsObserved)
		if limit <= 0 {
			return errors.New("running history operation has exhausted its record budget")
		}
		requestBudget := min(remaining, 30*time.Second)
		op, err = store.ReserveHistoryPage(ctx, op.Status.OperationID, op.Revision, requestBudget)
		if errors.Is(err, storage.ErrHistoryState) || ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		if op.Status.State != "running" {
			return nil
		}
		request, cancel := context.WithTimeout(ctx, requestBudget)
		started := time.Now()
		var page domain.HistoryPage
		if op.Status.Source == "conversation_preload" {
			if snapshot, ok := source.(domain.PreloadHistorySource); ok {
				page, err = snapshot.PreloadHistoryPage(request, op.Status.Ref(), limit)
			} else {
				err = domain.ErrHistoryUnsupported
			}
		} else if source == nil {
			err = domain.ErrHistoryUnsupported
		} else if phased, ok := source.(domain.PhasedHistorySource); ok {
			if phased.HistoryPhaseRequired() && op.Status.PagesObserved > 0 && op.Status.IsOld == nil {
				cancel()
				return stop("partial", "missing_continuation", 0)
			}
			page, err = phased.HistoryPageWithPhase(request, op.Status.Ref(), op.Cursor, op.Status.IsOld, limit)
		} else {
			page, err = source.HistoryPage(request, op.Status.Ref(), op.Cursor, limit)
		}
		elapsed := time.Since(started)
		cancel()
		if ctx.Err() != nil {
			return nil // running checkpoint is recoverable; never fabricate success.
		}
		if err != nil {
			logSourceFailure(op.Status.OperationID, err)
			state, reason := "failed", "upstream_unavailable"
			var invalid *domain.Error
			switch {
			case errors.Is(err, domain.ErrAuthenticationRequired):
				if e := stop("paused", "auth_required", elapsed); e != nil {
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
			return stop(state, reason, elapsed)
		}
		commitBudget := remaining - time.Since(started)
		if commitBudget <= 0 {
			return stop("partial", "time_limit", elapsed)
		}
		commit, commitCancel := context.WithTimeout(ctx, commitBudget)
		updated, commitErr := store.CommitHistoryOperationPage(commit, op.Status.OperationID, op.Revision, page, min(elapsed, storage.HistoryWorkBudget))
		commitCancel()
		if errors.Is(commitErr, storage.ErrHistoryState) || ctx.Err() != nil {
			return nil
		}
		if commitErr != nil {
			if errors.Is(commitErr, context.DeadlineExceeded) {
				return stop("partial", "time_limit", time.Since(started))
			}
			reason := "storage_error"
			var invalid *domain.Error
			if errors.As(commitErr, &invalid) && invalid.Code == "INVALID_ARGUMENT" {
				reason = "invalid_source_page"
			}
			return stop("failed", reason, elapsed)
		}
		op = updated
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
		var malformed *domain.HistoryPageFailure
		if errors.As(err, &malformed) {
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

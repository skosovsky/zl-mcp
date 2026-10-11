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
	ReadHistoryPage(context.Context, domain.MobileBackupRequest, int, int, *domain.MobileHistorySnapshot, *domain.MobileHistoryPosition) (domain.MobileHistoryPage, error)
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
		if source != nil {
			if err := source.CleanupHistorySnapshots(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
		pending, err := store.PendingMobileHistoryOperations(ctx, 20)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
		for _, op := range pending {
			if err := runMobileOperation(ctx, store, source, op); err != nil {
				// Owner shutdown can race a final operation-journal write after
				// that operation's last context check. As with pending reads and
				// snapshot cleanup, cancellation ends this worker normally.
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			if source != nil && ctx.Err() == nil {
				if err := source.CleanupHistorySnapshots(ctx); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return err
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func runMobileOperation(ctx context.Context, store *storage.Store, source MobileHistorySource, pending storage.HistoryOperation) error {
	return shared.ObserveWorker(ctx, domain.WorkHistory, pending.Status.OperationID, func(ctx context.Context) error { return runMobileOperationObserved(ctx, store, source, pending) })
}
func runMobileOperationObserved(ctx context.Context, store *storage.Store, source MobileHistorySource, pending storage.HistoryOperation) error {
	op, err := store.ClaimHistoryOperation(ctx, pending.Status.OperationID, pending.Revision)
	if ctx.Err() != nil || errors.Is(err, storage.ErrHistoryState) {
		return nil
	}
	if err != nil || op.Status.State != "running" {
		return err
	}
	created, _ := time.Parse(time.RFC3339Nano, op.Status.CreatedAt)
	shared.ObserveTransition(ctx, domain.WorkHistory, pending.Status.State, op.Status.State, created)
	storageFailed := false
	stop := func(state, reason string, elapsed time.Duration) error {
		remaining := max(storage.MobileHistoryWorkBudget-op.WorkDuration, 0)
		next, e := store.StopHistoryOperation(ctx, op.Status.OperationID, op.Revision, state, reason, min(elapsed, remaining))
		if e == nil {
			shared.ObserveTransition(ctx, domain.WorkHistory, op.Status.State, next.Status.State, created)
			op = next
		}
		if ctx.Err() != nil || errors.Is(e, storage.ErrHistoryState) {
			return nil
		}
		if e == nil {
			e = store.CloseMobileHistoryAcquisition(ctx, op.Status.OperationID)
		}
		return e
	}
	if source == nil {
		return stop("unsupported", "source_unsupported", 0)
	}
	reserve := func(stage context.Context, maximum time.Duration) (time.Duration, error) {
		remaining := storage.MobileHistoryWorkBudget - op.WorkDuration
		if remaining <= 0 {
			return 0, stop("partial", "time_limit", 0)
		}
		budget := min(maximum, remaining)
		next, e := store.ReserveMobileHistoryWork(stage, op.Status.OperationID, op.Revision, budget)
		if e != nil {
			storageFailed = !errors.Is(e, storage.ErrHistoryState)
			return 0, e
		}
		op = next
		if op.Status.State != "running" {
			return 0, nil
		}
		return budget, nil
	}
	complete := func(stage context.Context, started time.Time) error {
		next, e := store.CompleteMobileHistoryWork(stage, op.Status.OperationID, op.Revision, min(time.Since(started), storage.MobileHistoryWorkBudget))
		if e == nil {
			op = next
		}
		return e
	}
	err = source.WithHistorySession(ctx, func(sessionCtx context.Context, session MobileHistorySession) error {
		if sessionCtx == nil || session == nil {
			return domain.ErrHistoryUnsupported
		}
		attempt, e := store.MobileHistoryAcquisition(sessionCtx, op.Status.OperationID)
		if errors.Is(e, sql.ErrNoRows) {
			budget, e := reserve(sessionCtx, 180*time.Second)
			if e != nil || budget == 0 {
				return e
			}
			attempt, e = store.PrepareMobileHistoryAcquisition(sessionCtx, op.Status.OperationID, op.Revision, 0)
			if e != nil {
				storageFailed = !errors.Is(e, storage.ErrHistoryState)
				return e
			}
			request, cancel := context.WithTimeout(sessionCtx, budget)
			started := time.Now()
			offer, receiveErr := shared.ObserveUpstream(request, domain.WorkHistory, func(ctx context.Context) (domain.MobileBackupOffer, error) {
				return session.ReceiveOffer(ctx, attempt.OperationID)
			})
			cancel()
			defer func() { offer = domain.MobileBackupOffer{} }()
			if sessionCtx.Err() != nil {
				return sessionCtx.Err()
			}
			if e = complete(sessionCtx, started); e != nil {
				storageFailed = !errors.Is(e, storage.ErrHistoryState)
				return e
			}
			if receiveErr != nil || op.Status.State != "running" {
				return receiveErr
			}
			budget, e = reserve(sessionCtx, 120*time.Second)
			if e != nil || budget == 0 {
				return e
			}
			request, cancel = context.WithTimeout(sessionCtx, budget)
			started = time.Now()
			saveErr := func() error {
				_, err := shared.ObserveUpstream(request, domain.WorkHistory, func(ctx context.Context) (struct{}, error) {
					return struct{}{}, session.SaveOffer(ctx, attempt.Request, offer)
				})
				return err
			}()
			cancel()
			offer = domain.MobileBackupOffer{}
			if sessionCtx.Err() != nil {
				return sessionCtx.Err()
			}
			if e = complete(sessionCtx, started); e != nil {
				storageFailed = !errors.Is(e, storage.ErrHistoryState)
				return e
			}
			if saveErr != nil || op.Status.State != "running" {
				return saveErr
			}
		} else if e != nil {
			storageFailed = !errors.Is(e, storage.ErrHistoryState)
			return e
		}
		for op.Status.State == "running" {
			budget, e := reserve(sessionCtx, 30*time.Second)
			if e != nil || budget == 0 {
				return e
			}
			var previous *domain.MobileHistorySnapshot
			var after *domain.MobileHistoryPosition
			if op.Status.PagesObserved > 0 || op.Status.MobileCoverage != nil {
				checkpoint, e := store.MobileHistoryCheckpoint(sessionCtx, op.Status.OperationID)
				if e != nil {
					storageFailed = !errors.Is(e, storage.ErrHistoryState)
					return e
				}
				previous, after = &checkpoint.Snapshot, checkpoint.Next
			}
			request, cancel := context.WithTimeout(sessionCtx, budget)
			started := time.Now()
			page, readErr := shared.ObserveUpstream(request, domain.WorkHistory, func(ctx context.Context) (domain.MobileHistoryPage, error) {
				return session.ReadHistoryPage(ctx, attempt.Request, op.Status.PageSize, op.Status.RecordsObserved, previous, after)
			})
			cancel()
			elapsed := time.Since(started)
			if readErr != nil || sessionCtx.Err() != nil {
				page.Clear()
				if sessionCtx.Err() != nil {
					return sessionCtx.Err()
				}
				if e = complete(sessionCtx, started); e != nil {
					storageFailed = !errors.Is(e, storage.ErrHistoryState)
					return e
				}
				return readErr
			}
			next, e := store.CommitMobileHistoryPage(sessionCtx, op.Status.OperationID, op.Revision, page, min(elapsed, storage.MobileHistoryWorkBudget))
			page.Clear()
			if e != nil {
				var invalid *domain.Error
				if errors.As(e, &invalid) && invalid.Code == "INVALID_ARGUMENT" || errors.Is(e, storage.ErrMobileHistorySource) {
					sourceErr := domain.ErrHistoryInvalidPage
					if errors.Is(e, storage.ErrMobileHistorySource) {
						sourceErr = ErrMobileHistorySourceUnavailable
					}
					if e = complete(sessionCtx, started); e != nil {
						storageFailed = !errors.Is(e, storage.ErrHistoryState)
						return e
					}
					return sourceErr
				}
				storageFailed = !errors.Is(e, storage.ErrHistoryState)
				return e
			}
			if op.Status.State != next.Status.State {
				shared.ObserveTransition(ctx, domain.WorkHistory, op.Status.State, next.Status.State, created)
			}
			op = next
		}
		return nil
	})
	if ctx.Err() != nil {
		return nil
	}
	if errors.Is(err, storage.ErrHistoryState) || op.Status.State != "running" {
		if e := store.CloseMobileHistoryAcquisition(ctx, op.Status.OperationID); e != nil && !errors.Is(e, storage.ErrHistoryState) {
			return e
		}
		return nil
	}
	if storageFailed {
		return err
	}
	if err == nil {
		return errors.New("mobile history session ended with unfinished work")
	}
	state, reason := "partial", "source_unavailable"
	switch {
	case errors.Is(err, domain.ErrAuthenticationRequired):
		if e := stop("paused", "auth_required", op.ReservedDuration); e != nil {
			return e
		}
		return domain.ErrAuthenticationRequired
	case errors.Is(err, domain.ErrHistoryUnsupported):
		state, reason = "unsupported", "source_unsupported"
	case errors.Is(err, domain.ErrHistoryInvalidPage):
		state, reason = "failed", "invalid_source_page"
	}
	return stop(state, reason, op.ReservedDuration)
}

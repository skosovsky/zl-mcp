package mobilebackup

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

var ErrOfferExecution = errors.New("mobile backup offer execution failed")

// RunPreparedOffer executes only a trusted prepared attempt using the current guarded session.
// No caller retry reconstructs an offer or redispatches a terminal attempt.
func RunPreparedOffer(ctx context.Context, store *storage.Store, source domain.MobileBackupSource, id string) (domain.MobileBackupOffer, error) {
	if ctx == nil || store == nil || source == nil {
		return domain.MobileBackupOffer{}, ErrOfferExecution
	}
	observer, err := store.MobileBackupObserver(ctx, id)
	if err != nil {
		return domain.MobileBackupOffer{}, ErrOfferExecution
	}
	var claimed atomic.Bool
	before := observer.BeforeDispatch
	observer.BeforeDispatch = func(public string) error {
		e := before(public)
		if e == nil {
			claimed.Store(true)
		}
		return e
	}
	var offer domain.MobileBackupOffer
	if ctx.Err() == nil {
		offer, err = source.ReceiveMobileBackupOffer(ctx, observer)
	} else {
		err = ctx.Err()
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil {
		attempt, e := store.MobileBackupAttempt(ctx, id)
		if e == nil && claimed.Load() && attempt.State == "offer_ready" {
			return offer, nil
		}
		err = ErrOfferExecution
	}
	// Only the invocation that committed dispatch owns finalization. A pre-dispatch
	// source failure may race with another observer; leave its journal unchanged.
	if !claimed.Load() {
		return domain.MobileBackupOffer{}, ErrOfferExecution
	}
	// Request lifetime may be over; finish evidence independently before returning.
	finish, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	attempt, e := store.MobileBackupAttempt(finish, id)
	if e != nil {
		return domain.MobileBackupOffer{}, ErrOfferExecution
	}
	switch attempt.State {
	case "dispatching", "waiting_for_confirmation", "request_result_unknown", "mobile_restoring", "waiting_for_backup":
		state := "failed"
		cancelled := ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		if cancelled || errors.Is(err, domain.ErrMobileBackupUnknown) || errors.Is(err, domain.ErrAuthenticationRequired) {
			state = "interrupted"
		}
		if _, e = store.ProgressMobileBackup(finish, id, attempt.Revision, state); e != nil {
			return domain.MobileBackupOffer{}, ErrOfferExecution
		}
	}
	return domain.MobileBackupOffer{}, ErrOfferExecution
}

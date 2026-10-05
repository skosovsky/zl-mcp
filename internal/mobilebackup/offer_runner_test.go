package mobilebackup

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type offerSourceFunc func(context.Context, *domain.MobileBackupObserver) (domain.MobileBackupOffer, error)

func (f offerSourceFunc) ReceiveMobileBackupOffer(c context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
	return f(c, o)
}
func runnerAttempt(t *testing.T) (*storage.Store, storage.MobileBackupAttempt, string) {
	t.Helper()
	s, e := storage.OpenWithPolicy(context.Background(), filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if e = s.BindAccount(context.Background(), "synthetic-account"); e != nil {
		t.Fatal(e)
	}
	a, e := s.PrepareMobileBackup(context.Background(), domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "synthetic-peer", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"})
	if e != nil {
		t.Fatal(e)
	}
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, e := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	if e != nil {
		t.Fatal(e)
	}
	return s, a, base64.StdEncoding.EncodeToString(der)
}
func TestPreparedOfferDurableDispatchAndNoRetry(t *testing.T) {
	// Arrange: source checks dispatch evidence before simulating outbound request.
	s, a, public := runnerAttempt(t)
	requests := 0
	source := offerSourceFunc(func(c context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
		if e := o.BeforeDispatch(public); e != nil {
			return domain.MobileBackupOffer{}, e
		}
		persisted, e := s.MobileBackupAttempt(c, a.OperationID)
		if e != nil || persisted.State != "dispatching" || persisted.PublicKey != public {
			t.Fatal("request precedes journal")
		}
		requests++
		if e = o.Progress("waiting_for_confirmation"); e != nil {
			return domain.MobileBackupOffer{}, e
		}
		if e = o.Progress("offer_ready"); e != nil {
			return domain.MobileBackupOffer{}, e
		}
		return domain.MobileBackupOffer{URL: "https://example.invalid/archive", KeyText: "synthetic-key", FileSize: 16}, nil
	})
	// Act.
	offer, e := RunPreparedOffer(context.Background(), s, source, a.OperationID)
	_, retry := RunPreparedOffer(context.Background(), s, source, a.OperationID)
	// Assert: offer credentials are available only on original successful execution.
	if e != nil || offer.FileSize != 16 || retry == nil || requests != 1 {
		t.Fatal("offer identity/retry lost")
	}
}
func TestPreparedOfferFailuresCloseEvidenceAndDiscardOffer(t *testing.T) {
	for _, mode := range []string{"premature", "cancel-before", "cancel-after", "unknown", "explicit"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			s, a, public := runnerAttempt(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			if mode == "cancel-before" {
				cancel()
			}
			source := offerSourceFunc(func(c context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
				calls++
				if mode == "premature" {
					return domain.MobileBackupOffer{KeyText: "must-discard"}, nil
				}
				if e := o.BeforeDispatch(public); e != nil {
					return domain.MobileBackupOffer{}, e
				}
				if e := o.Progress("waiting_for_confirmation"); e != nil {
					return domain.MobileBackupOffer{}, e
				}
				switch mode {
				case "cancel-after":
					cancel()
					return domain.MobileBackupOffer{KeyText: "must-discard"}, nil
				case "unknown":
					return domain.MobileBackupOffer{}, domain.ErrMobileBackupUnknown
				default:
					return domain.MobileBackupOffer{}, errors.New("private source failure must not escape")
				}
			})
			// Act.
			offer, e := RunPreparedOffer(ctx, s, source, a.OperationID)
			persisted, read := s.MobileBackupAttempt(context.Background(), a.OperationID)
			// Assert: no source error/secret leaks; cancellation finalization survives dead context.
			want := "failed"
			if mode == "cancel-before" || mode == "premature" {
				want = "prepared"
			}
			if mode == "cancel-after" || mode == "unknown" {
				want = "interrupted"
			}
			if !errors.Is(e, ErrOfferExecution) || offer.KeyText != "" || read != nil || persisted.State != want || mode == "cancel-before" && calls != 0 {
				t.Fatal("failure evidence not closed", read)
			}
		})
	}
}

func TestPreparedOfferRetryCannotInterruptCurrentOwner(t *testing.T) {
	// Arrange: first request owns the durable dispatch while a retry arrives.
	s, a, public := runnerAttempt(t)
	entered, release := make(chan struct{}), make(chan struct{})
	original := offerSourceFunc(func(c context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
		if e := o.BeforeDispatch(public); e != nil {
			return domain.MobileBackupOffer{}, e
		}
		if e := o.Progress("waiting_for_confirmation"); e != nil {
			return domain.MobileBackupOffer{}, e
		}
		close(entered)
		<-release
		if e := o.Progress("offer_ready"); e != nil {
			return domain.MobileBackupOffer{}, e
		}
		return domain.MobileBackupOffer{FileSize: 16}, nil
	})
	done := make(chan error, 1)
	go func() { _, e := RunPreparedOffer(context.Background(), s, original, a.OperationID); done <- e }()
	<-entered
	// Act.
	retrySource := offerSourceFunc(func(context.Context, *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
		t.Error("retry reached source")
		return domain.MobileBackupOffer{}, nil
	})
	_, retry := RunPreparedOffer(context.Background(), s, retrySource, a.OperationID)
	state, e := s.MobileBackupAttempt(context.Background(), a.OperationID)
	close(release)
	first := <-done
	// Assert: rejected retry cannot finalize another execution's active state.
	if retry == nil || e != nil || state.State != "waiting_for_confirmation" || first != nil {
		t.Fatal("retry interrupted owner")
	}
}

func TestPreparedOfferStaleObserverCannotCloseWinningDispatch(t *testing.T) {
	// Arrange: both invocations attach before either reserves dispatch.
	s, a, public := runnerAttempt(t)
	firstEntered, secondEntered, dispatched, release := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	winner := offerSourceFunc(func(c context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
		close(firstEntered)
		<-secondEntered
		if e := o.BeforeDispatch(public); e != nil {
			return domain.MobileBackupOffer{}, e
		}
		if e := o.Progress("waiting_for_confirmation"); e != nil {
			return domain.MobileBackupOffer{}, e
		}
		close(dispatched)
		<-release
		if e := o.Progress("offer_ready"); e != nil {
			return domain.MobileBackupOffer{}, e
		}
		return domain.MobileBackupOffer{FileSize: 16}, nil
	})
	loser := offerSourceFunc(func(c context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
		close(secondEntered)
		<-dispatched
		e := o.BeforeDispatch(public)
		if e == nil {
			t.Error("stale observer reserved a second dispatch")
		}
		return domain.MobileBackupOffer{}, e
	})
	done := make(chan error, 1)
	go func() { _, e := RunPreparedOffer(context.Background(), s, winner, a.OperationID); done <- e }()
	<-firstEntered
	// Act.
	_, failed := RunPreparedOffer(context.Background(), s, loser, a.OperationID)
	state, e := s.MobileBackupAttempt(context.Background(), a.OperationID)
	close(release)
	success := <-done
	// Assert: CAS loser cannot finalize an attempt owned by the winner.
	if failed == nil || e != nil || state.State != "waiting_for_confirmation" || success != nil {
		t.Fatal("loser closed winning request")
	}
}

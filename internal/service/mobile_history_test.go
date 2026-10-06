package service

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type historyLeaseSource struct {
	archiveProbeSource
	leaseCancel    context.CancelFunc
	opened, closed int
}

type productionHistorySource struct{ archiveProbeSource }

func (s *productionHistorySource) ReceiveMobileBackupOffer(ctx context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
	offer, err := s.mobilePageSource.ReceiveMobileBackupOffer(ctx, o)
	offer.URL = "https://trans-bin.zaloapp.com/synthetic"
	offer.SessionAccountID = "10"
	return offer, err
}

func TestMobileHistoryWorkerUsesExistingCollector(t *testing.T) {
	// Arrange: encrypted synthetic archive, production session guard and one queued import.
	raw, err := os.ReadFile("../mobilebackup/testdata/format1-sqlite-vector.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct{ Ciphertext string }
	if err = json.Unmarshal(raw, &vector); err != nil {
		t.Fatal(err)
	}
	data, err := hex.DecodeString(vector.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, err := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	if err != nil {
		t.Fatal(err)
	}
	source := &productionHistorySource{archiveProbeSource{mobilePageSource{data: data, mobileServiceSource: mobileServiceSource{public: base64.StdEncoding.EncodeToString(der), historyServiceSource: historyServiceSource{started: make(chan struct{})}}}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.BindAccount(ctx, "10"); err != nil {
		t.Fatal(err)
	}
	op, err := store.PrepareMobileHistoryOperation(ctx, domain.HistoryImportRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "12", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z", PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := mobilebackup.NewSnapshotStore(filepath.Join(t.TempDir(), "snapshots"), 512<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshots.Close()
	port := &membershipPort{store: store, lifecycle: ctx, snapshots: snapshots}
	var c config.Config
	c.Collection.Mode = "all"
	var restores atomic.Int32
	done := make(chan error, 1)
	// Act: service lifecycle starts both workers after recovery on its single listener.
	go func() {
		done <- collectWithHeartbeat(ctx, c, store, port, func(context.Context, string) (collector.ListenerUpstream, error) { restores.Add(1); return source, nil }, func(collector.ListenerUpstream) error { return nil }, time.Hour)
	}()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("collector and mobile worker did not stop")
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		op, err = store.HistoryOperation(ctx, op.Status.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status.State != "queued" && op.Status.State != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mobile operation did not finish", op.Status.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Assert: a snapshot-backed page was committed silently without another restore/offer.
	if restores.Load() != 1 || source.offers.Load() != 1 || source.downloads.Load() != 1 || op.Status.PagesObserved == 0 || op.Status.HistoryComplete {
		t.Fatal("production mobile integration failed", op.Status)
	}
	for _, table := range []string{"message_events", "event_deliveries"} {
		var count int
		if err := store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("history generated events", table, count, err)
		}
	}
}

func (s *historyLeaseSource) MobileBackupContext(ctx context.Context) (context.Context, func(), error) {
	s.opened++
	child, cancel := context.WithCancel(ctx)
	s.leaseCancel = cancel
	return child, func() { s.closed++; cancel() }, nil
}

func TestMobileHistorySessionLifetime(t *testing.T) {
	for _, mode := range []string{"success", "parent-stop", "service-stop", "session-loss", "callback-error"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: one authenticated session and a private snapshot store.
			p := mobileLedgerPort(t)
			snapshots, err := mobilebackup.NewSnapshotStore(filepath.Join(t.TempDir(), "snapshots"), 512<<20)
			if err != nil {
				t.Fatal(err)
			}
			defer snapshots.Close()
			p.snapshots = snapshots
			parent, stopParent := context.WithCancel(context.Background())
			defer stopParent()
			lifecycle, stopService := context.WithCancel(context.Background())
			defer stopService()
			p.lifecycle = lifecycle
			source := &historyLeaseSource{}
			p.current = &collector.JoinManager{API: source}
			downloader, err := mobilebackup.NewDownloader([]string{"archive.example.com"})
			if err != nil {
				t.Fatal(err)
			}
			callbackErr := errors.New("synthetic callback failure")
			var scratch string
			// Act: hold the same session through cancellation or callback completion.
			err = p.withHistorySession(parent, downloader, func(ctx context.Context, borrowed historyimport.MobileHistorySession) error {
				s := borrowed.(*mobileHistorySession)
				scratch = s.scratch
				if info, e := os.Stat(scratch); e != nil || info.Mode().Perm() != 0700 {
					t.Fatal("scratch is not private")
				}
				if strings.Contains(fmt.Sprintf("%#v", s), scratch) {
					t.Fatal("session formatting exposes private state")
				}
				switch mode {
				case "parent-stop":
					stopParent()
				case "service-stop":
					stopService()
				case "session-loss":
					source.leaseCancel()
				case "callback-error":
					return callbackErr
				default:
					return nil
				}
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
					t.Fatal("borrowed session did not stop")
				}
				if _, e := s.ReadHistoryPage(ctx, domain.MobileBackupRequest{}, 1, 0, nil, nil); !errors.Is(e, domain.ErrAuthenticationRequired) {
					t.Fatal("cancelled session remained usable", e)
				}
				return nil
			})
			// Assert: exactly one scope, correct failure boundary and no scratch remains.
			var want error
			switch mode {
			case "parent-stop", "service-stop":
				want = context.Canceled
			case "session-loss":
				want = domain.ErrAuthenticationRequired
			case "callback-error":
				want = callbackErr
			}
			if !errors.Is(err, want) {
				t.Fatal("incorrect session result", err, want)
			}
			if source.opened != 1 || source.closed != 1 || source.offers.Load() != 0 || source.downloads.Load() != 0 {
				t.Fatal("unexpected session or network work")
			}
			if _, e := os.Stat(scratch); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("scratch survived session", e)
			}
		})
	}
}

func TestMobileHistorySessionRequiresAuthenticatedSource(t *testing.T) {
	// Arrange: no collector session; the callback must never be invoked.
	p := mobileLedgerPort(t)
	called := false
	// Act.
	err := p.WithHistorySession(context.Background(), func(context.Context, historyimport.MobileHistorySession) error { called = true; return nil })
	// Assert.
	if !errors.Is(err, domain.ErrAuthenticationRequired) || called {
		t.Fatal("missing session accepted", err)
	}
}

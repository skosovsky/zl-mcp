package service

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"math/big"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type mobileServiceSource struct {
	historyServiceSource
	public string
	offers atomic.Int32
}

func (s *mobileServiceSource) ReceiveMobileBackupOffer(ctx context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
	if e := o.BeforeDispatch(s.public); e != nil {
		return domain.MobileBackupOffer{}, e
	}
	s.offers.Add(1)
	if e := o.Progress("waiting_for_confirmation"); e != nil {
		return domain.MobileBackupOffer{}, e
	}
	if e := o.Progress("offer_ready"); e != nil {
		return domain.MobileBackupOffer{}, e
	}
	return domain.MobileBackupOffer{FileSize: 16, KeyText: "synthetic-private-key"}, nil
}
func TestServiceMobilePortUsesOneCollectorSession(t *testing.T) {
	// Arrange: real collector/sessionGuard with one source, not a separate backup login.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, e := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, _ := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	source := &mobileServiceSource{historyServiceSource: historyServiceSource{started: make(chan struct{})}, public: base64.StdEncoding.EncodeToString(der)}
	p := &membershipPort{store: s, lifecycle: ctx}
	var c config.Config
	c.Collection.Mode = "all"
	restores := atomic.Int32{}
	done := make(chan error, 1)
	go func() {
		done <- collectWithHeartbeat(ctx, c, s, p, func(context.Context, string) (collector.ListenerUpstream, error) { restores.Add(1); return source, nil }, func(collector.ListenerUpstream) error { return nil }, time.Hour)
	}()
	select {
	case <-source.started:
	case <-time.After(5 * time.Second):
		cancel()
		<-done
		t.Fatal("collector not ready")
	}
	r := domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "synthetic-peer", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"}
	a, e := s.PrepareMobileBackup(ctx, r)
	if e != nil {
		cancel()
		<-done
		t.Fatal(e)
	}
	// Act: internal offer execution through the same current guarded source.
	offer, e := p.runPreparedMobileOffer(ctx, a.OperationID)
	_, retry := p.runPreparedMobileOffer(ctx, a.OperationID)
	saved, read := s.MobileBackupAttempt(ctx, a.OperationID)
	cancel()
	stopped := <-done
	// Assert: no second restore, listener or dispatch; a private offer isn't imported.
	if e != nil || retry == nil || read != nil || saved.State != "offer_ready" || offer.FileSize != 16 || restores.Load() != 1 || source.offers.Load() != 1 || stopped != nil {
		t.Fatal("service mobile ownership lost", e, read, stopped)
	}
}
func TestServiceMobilePortMissingSessionAndStoppedLifecycle(t *testing.T) {
	// Arrange.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &membershipPort{lifecycle: ctx}
	// Act / Assert: cancellation/unavailable source must not require a store or dispatch.
	if _, e := p.runPreparedMobileOffer(context.Background(), "synthetic-operation"); e == nil {
		t.Fatal("stopped service accepted operation")
	}
	p.lifecycle = nil
	if _, e := p.runPreparedMobileOffer(context.Background(), "synthetic-operation"); e == nil {
		t.Fatal("missing source accepted operation")
	}
}

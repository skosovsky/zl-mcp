package service

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestServiceRecoversMobileAttemptsBeforeExposingEndpoint(t *testing.T) {
	// Arrange: a previous process dispatched a request, preserving its public key.
	dir, e := os.MkdirTemp("/tmp", "zl-mobile-recovery-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	var c config.Config
	c.StateDir, c.Collection.Mode, c.Storage.RetentionDays = dir, "all", 90
	c.MCP.Listen, c.MCP.TokenFile = "127.0.0.1:0", filepath.Join(dir, "token")
	ctx := context.Background()
	s, e := storage.OpenWithPolicy(ctx, filepath.Join(dir, "messages.sqlite"), c.Policy(), 90)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.BindAccount(ctx, "test-account"); e != nil {
		s.Close()
		t.Fatal(e)
	}
	request := domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "synthetic-peer", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"}
	a, e := s.PrepareMobileBackup(ctx, request)
	if e != nil {
		s.Close()
		t.Fatal(e)
	}
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, _ := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	public := base64.StdEncoding.EncodeToString(der)
	a, e = s.DispatchMobileBackup(ctx, a.OperationID, a.Revision, public)
	if e != nil {
		s.Close()
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	live, cancel := context.WithCancel(ctx)
	defer cancel()
	exposed := false
	// Act: startup recovery must complete before ready callback or any restored session.
	e = run(live, c, func(context.Context, string) (collector.ListenerUpstream, error) {
		return nil, domain.ErrAuthenticationRequired
	}, func(collector.ListenerUpstream) error { t.Error("unexpected session save"); return nil }, func(net.Addr) {
		observed, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dir, "messages.sqlite"))+"?mode=ro")
		if err != nil {
			t.Error("cannot read recovery evidence")
			cancel()
			return
		}
		var state string
		err = observed.QueryRowContext(ctx, "SELECT state FROM mobile_backup_attempts WHERE operation_id=?", a.OperationID).Scan(&state)
		observed.Close()
		if err != nil || state != "interrupted" {
			t.Error("endpoint exposed before recovery")
		}
		exposed = true
		cancel()
	})
	if e != nil || !exposed {
		t.Fatal("service startup failed", e)
	}
	s, e = storage.OpenWithPolicy(ctx, filepath.Join(dir, "messages.sqlite"), c.Policy(), 90)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	saved, e := s.MobileBackupAttempt(ctx, a.OperationID)
	// Assert: exact operation/request/key retained, no dispatchable retry or corpus/event writes.
	if e != nil || saved.State != "interrupted" || saved.Revision != a.Revision+1 || saved.PublicKey != public || saved.Request.RequestID != request.RequestID {
		t.Fatal("mobile attempt not recovered", e)
	}
	for _, table := range []string{"messages", "message_events", "send_operations", "event_subscriptions"} {
		var count int
		if e = s.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); e != nil || count != 0 {
			t.Fatal("recovery changed unrelated state", table, e)
		}
	}
}

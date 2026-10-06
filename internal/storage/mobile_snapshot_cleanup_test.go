package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestMobileSnapshotCleanupOwnershipAndTerminalState(t *testing.T) {
	for _, mode := range []string{"running", "paused", "partial", "failed", "unsupported", "cancelled", "revoked-policy", "foreign-account", "changed-binding", "unlinked"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: account-bound mobile operation and permanent acquisition link.
			ctx := context.Background()
			s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err = s.BindAccount(ctx, "10"); err != nil {
				t.Fatal(err)
			}
			op, err := s.PrepareMobileHistoryOperation(ctx, domain.HistoryImportRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "12", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"})
			if err == nil {
				op, err = s.ClaimHistoryOperation(ctx, op.Status.OperationID, op.Revision)
			}
			if err == nil {
				op, err = s.ReserveMobileHistoryWork(ctx, op.Status.OperationID, op.Revision, 180*time.Second)
			}
			if err != nil {
				t.Fatal(err)
			}
			a, err := s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
			if err != nil {
				t.Fatal(err)
			}
			state, reason := "partial", "source_unavailable"
			switch mode {
			case "paused":
				state, reason = "paused", "auth_required"
			case "failed":
				state, reason = "failed", "invalid_source_page"
			case "unsupported":
				state, reason = "unsupported", "source_unsupported"
			case "cancelled":
				_, err = s.CancelHistoryOperation(ctx, op.Status.OperationID)
			}
			if mode != "running" && mode != "cancelled" {
				_, err = s.StopHistoryOperation(ctx, op.Status.OperationID, op.Revision, state, reason, 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte("10"))
			owner := hex.EncodeToString(digest[:])
			id, fingerprint := a.Request.RequestID, a.Request.Fingerprint()
			switch mode {
			case "foreign-account":
				owner = strings.Repeat("f", 64)
			case "changed-binding":
				fingerprint = strings.Repeat("f", 64)
			case "unlinked":
				_, err = s.DB.Exec("DELETE FROM history_mobile_attempts")
			case "revoked-policy":
				s.policy = domain.CollectionPolicy{}
			}
			if err != nil {
				t.Fatal(err)
			}
			// Act: cleanup authorization grants no read, retry or phone capability.
			eligible, err := s.MobileHistorySnapshotTerminal(ctx, id, fingerprint, owner)
			// Assert: terminal owned bindings alone qualify, even after collection revocation.
			want := mode == "partial" || mode == "failed" || mode == "unsupported" || mode == "cancelled" || mode == "revoked-policy"
			if err != nil || eligible != want {
				t.Fatal("incorrect cleanup authority", eligible, err)
			}
		})
	}
}

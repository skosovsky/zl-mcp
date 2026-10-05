package storage

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func mobileJournalRequest() domain.MobileBackupRequest {
	return domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "synthetic-peer", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"}
}
func mobileJournalPublic(t *testing.T) string {
	t.Helper()
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, err := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func TestMobileBackupJournalIdentityCASAndRestart(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.sqlite")
	s := historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	defer func() { s.Close() }()
	r := mobileJournalRequest()
	a, err := s.PrepareMobileBackup(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	equivalent := r
	equivalent.Since = "2026-09-01T07:00:00+07:00"
	equivalent.MaxMessages = 1000
	equivalent.MaxArchiveBytes = 64 << 20
	// Act / Assert: normalization and stable UUID identity precede network work.
	again, err := s.PrepareMobileBackup(ctx, equivalent)
	if err != nil || again.OperationID != a.OperationID {
		t.Fatal("equivalent retry lost identity")
	}
	conflict := r
	conflict.MaxMessages = 2
	if _, err = s.PrepareMobileBackup(ctx, conflict); !errors.Is(err, ErrMobileBackupConflict) {
		t.Fatal("changed arguments accepted")
	}
	other := r
	other.RequestID = "00000000-0000-4000-8000-000000000002"
	if _, err = s.PrepareMobileBackup(ctx, other); !errors.Is(err, ErrMobileBackupBusy) {
		t.Fatal("parallel account transfer accepted")
	}
	public := mobileJournalPublic(t)
	// Act: durably reserve the only dispatch.
	a, err = s.DispatchMobileBackup(ctx, a.OperationID, a.Revision, public)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DispatchMobileBackup(ctx, a.OperationID, 0, public); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("stale dispatch accepted")
	}
	a, err = s.ProgressMobileBackup(ctx, a.OperationID, a.Revision, "request_result_unknown")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	if err = s.RecoverMobileBackupAttempts(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.PrepareMobileBackup(ctx, r)
	// Assert: no restart/retry creates a new operation or resets it to dispatchable.
	if err != nil || recovered.OperationID != a.OperationID || recovered.State != "interrupted" || recovered.PublicKey != public {
		t.Fatal("restart identity or interrupted state lost")
	}
	if _, err = s.DispatchMobileBackup(ctx, recovered.OperationID, recovered.Revision, public); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("interrupted attempt redispatched")
	}
	serialized, _ := json.Marshal(recovered)
	var status any
	if err = json.Unmarshal(serialized, &status); err != nil {
		t.Fatal(err)
	}
	schema, err := contracts.Compile("mobile_backup_attempt", "output")
	if err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(status); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), public) || strings.Contains(fmt.Sprintf("%#v", recovered), public) {
		t.Fatal("public correlation key exposed")
	}
	for _, table := range []string{"messages", "message_events", "event_subscriptions", "send_operations"} {
		var count int
		if err = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("journal changed unrelated data")
		}
	}
	// A new explicit UUID can reserve a new attempt after interruption.
	if _, err = s.PrepareMobileBackup(ctx, other); err != nil {
		t.Fatal("new explicit attempt rejected")
	}
}

func TestMobileBackupJournalPolicyAndTransitions(t *testing.T) {
	// Arrange.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "corpus.sqlite")
	s := historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	defer func() { s.Close() }()
	r := mobileJournalRequest()
	a, err := s.PrepareMobileBackup(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	// Act / Assert: no progress before durable dispatch, and malformed public keys fail closed.
	if _, err = s.ProgressMobileBackup(ctx, a.OperationID, a.Revision, "waiting_for_backup"); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("progress bypassed dispatch")
	}
	if _, err = s.DispatchMobileBackup(ctx, a.OperationID, a.Revision, base64.StdEncoding.EncodeToString([]byte("not SPKI"))); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("malformed correlation key stored")
	}
	a, err = s.DispatchMobileBackup(ctx, a.OperationID, a.Revision, mobileJournalPublic(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"waiting_for_confirmation", "mobile_restoring", "waiting_for_backup", "offer_ready"} {
		a, err = s.ProgressMobileBackup(ctx, a.OperationID, a.Revision, state)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.ProgressMobileBackup(ctx, a.OperationID, a.Revision, "dispatching"); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("terminal attempt restarted")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = historyOperationStore(t, path, domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{{Type: domain.ConversationDirect, ID: "other-peer"}: true}})
	if _, err = s.MobileBackupAttempt(ctx, a.OperationID); err == nil {
		t.Fatal("revoked scope read accepted")
	}
}

func TestMobileBackupMigrationRollback(t *testing.T) {
	// Arrange: reject version marker after table/index creation.
	s := historyOperationStore(t, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true})
	defer s.Close()
	_, err := s.DB.Exec(`DROP TABLE mobile_backup_attempts;DELETE FROM schema_migrations WHERE version=9;
 CREATE TRIGGER reject_mobile_migration BEFORE INSERT ON schema_migrations WHEN new.version=9 BEGIN SELECT RAISE(ABORT,'synthetic failure');END`)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	err = s.migrateMobileBackup(context.Background())
	// Assert: migration failure never leaves partial table or version state.
	if err == nil {
		t.Fatal("migration failure ignored")
	}
	var count int
	if err = s.DB.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='mobile_backup_attempts'").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial table retained")
	}
}

func TestMobileBackupJournalAccountAndCapacity(t *testing.T) {
	// Arrange: bound request belongs to the original synthetic account.
	ctx := context.Background()
	s := historyOperationStore(t, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true})
	defer s.Close()
	r := mobileJournalRequest()
	a, err := s.PrepareMobileBackup(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	var original string
	if err = s.DB.QueryRow("SELECT value FROM metadata WHERE key='account'").Scan(&original); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE metadata SET value='different-synthetic-account' WHERE key='account'"); err != nil {
		t.Fatal(err)
	}
	// Act / Assert: account mismatch fails before saved arguments or state are exposed.
	if _, err = s.MobileBackupAttempt(ctx, a.OperationID); err == nil {
		t.Fatal("foreign account read accepted")
	}
	if _, err = s.PrepareMobileBackup(ctx, r); err == nil {
		t.Fatal("foreign account retry accepted")
	}
	if _, err = s.DB.Exec("UPDATE metadata SET value=? WHERE key='account'", original); err != nil {
		t.Fatal(err)
	}
	a, err = s.ProgressMobileBackup(ctx, a.OperationID, a.Revision, "cancelled")
	if err != nil {
		t.Fatal(err)
	}
	// Arrange: fill only the phone-attempt ledger with synthetic terminal rows.
	normalized, err := r.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(normalized)
	_, err = s.DB.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<9999)
 INSERT INTO mobile_backup_attempts(operation_id,request_id,account_key,fingerprint,state,request,created_at,updated_at)
 SELECT 'synthetic-operation-'||x,'synthetic-request-'||x,?,'synthetic-fingerprint','failed',?, ?,? FROM n`, original, string(body), now(), now())
	if err != nil {
		t.Fatal(err)
	}
	r.RequestID = "00000000-0000-4000-8000-000000000002"
	// Act.
	_, err = s.PrepareMobileBackup(ctx, r)
	// Assert.
	if !errors.Is(err, ErrMobileBackupCapacity) {
		t.Fatal("ledger cap bypassed")
	}
}

func TestMobileBackupObserverDurableCallbacks(t *testing.T) {
	// Arrange: two observers start from the same prepared revision.
	ctx := context.Background()
	s := historyOperationStore(t, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true})
	defer s.Close()
	a, err := s.PrepareMobileBackup(ctx, mobileJournalRequest())
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.MobileBackupObserver(ctx, a.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := s.MobileBackupObserver(ctx, a.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	// Act: commit dispatch then progress through one observer.
	public := mobileJournalPublic(t)
	if err = first.BeforeDispatch(public); err != nil {
		t.Fatal(err)
	}
	if err = stale.BeforeDispatch(public); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("stale observer permitted redispatch")
	}
	for _, state := range []string{"waiting_for_confirmation", "waiting_for_backup", "offer_ready"} {
		if err = first.Progress(state); err != nil {
			t.Fatal(err)
		}
	}
	// Assert: every callback committed exactly one revision; terminal attempts cannot attach again.
	saved, err := s.MobileBackupAttempt(ctx, a.OperationID)
	if err != nil || saved.State != "offer_ready" || saved.Revision != 4 || saved.PublicKey != public {
		t.Fatal("observer journal mismatch")
	}
	if _, err = s.MobileBackupObserver(ctx, a.OperationID); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("terminal attempt reattached")
	}
}

func TestMobileRecoveryFreshStoreAndMissingAccountEvidence(t *testing.T) {
	// Arrange: a genuinely new installation has neither account nor attempts.
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Act / Assert: startup must work before login.
	if err = s.RecoverMobileBackupAttempts(ctx); err != nil {
		t.Fatal("fresh startup rejected", err)
	}
	if err = s.BindAccount(ctx, "synthetic-account"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PrepareMobileBackup(ctx, mobileJournalRequest()); err != nil {
		t.Fatal(err)
	}
	// Arrange: orphaned ledger evidence must not be rebound or assumed empty.
	if _, err = s.DB.ExecContext(ctx, "DELETE FROM metadata WHERE key='account'"); err != nil {
		t.Fatal(err)
	}
	// Act / Assert.
	if err = s.RecoverMobileBackupAttempts(ctx); !errors.Is(err, ErrMobileBackupState) {
		t.Fatal("orphaned mobile evidence accepted")
	}
}

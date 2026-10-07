package service

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type accountCaptureSource struct{ archiveProbeSource }

func (s *accountCaptureSource) MapMobileBackupIdentities(ctx context.Context, request domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
	s.mappings.Add(1)
	pairs := make([]domain.MobileIdentityPair, 0, len(request.Direct)+len(request.Groups))
	for _, id := range request.Direct {
		session := "12"
		if id == "903" {
			session = "13"
		}
		pairs = append(pairs, domain.MobileIdentityPair{Plain: id, Session: session})
	}
	for _, id := range request.Groups {
		pairs = append(pairs, domain.MobileIdentityPair{Plain: id, Session: "12", Group: true})
	}
	return pairs, ctx.Err()
}

func TestAccountArchiveCaptureRestartOfflineInspectionAndRemoval(t *testing.T) {
	// Arrange: independently encrypted synthetic SQLite, bound store and one owner-only capture.
	ctx := context.Background()
	stateDir := t.TempDir()
	policy := domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{{Type: domain.ConversationGroup, ID: "999"}: true}}
	store, err := storage.OpenWithPolicy(ctx, filepath.Join(stateDir, "corpus.sqlite"), policy, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p := &membershipPort{stateDir: stateDir, store: store, lifecycle: ctx}
	if e := p.store.BindAccount(ctx, "10"); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(p.stateDir, "account-archives")
	archives, e := mobilebackup.NewRetainedArchiveStore(path, 8<<20)
	if e != nil {
		t.Fatal(e)
	}
	p.archives = archives
	defer func() { p.archives.Close() }()
	raw, e := os.ReadFile("../mobilebackup/testdata/format1-account-vector.json")
	if e != nil {
		t.Fatal(e)
	}
	var vector struct{ Ciphertext string }
	if e = json.Unmarshal(raw, &vector); e != nil {
		t.Fatal(e)
	}
	data, e := hex.DecodeString(vector.Ciphertext)
	if e != nil {
		t.Fatal(e)
	}
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, _ := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	source := &accountCaptureSource{archiveProbeSource: archiveProbeSource{mobilePageSource: mobilePageSource{data: data, mobileServiceSource: mobileServiceSource{public: base64.StdEncoding.EncodeToString(der)}}}}
	p.current = &collector.JoinManager{API: source}
	response, attempt := ledgerRequest(t, p, "cli_prepare_account_archive", map[string]any{"request_id": "00000000-0000-4000-8000-000000000001", "archive_scope": "account"})
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	d, e := mobilebackup.NewDownloader([]string{"archive.example.com"})
	if e != nil {
		t.Fatal(e)
	}
	// Act: reject stale revision, acquire once, restart storage and disconnect upstream.
	if _, e = p.captureAccountArchiveWithDownloader(ctx, attempt.OperationID, attempt.Revision+1, d); e == nil {
		t.Fatal("stale revision dispatched")
	}
	first, e := p.captureAccountArchiveWithDownloader(ctx, attempt.OperationID, attempt.Revision, d)
	if e != nil {
		t.Fatal(e)
	}
	if e = p.archives.Close(); e != nil {
		t.Fatal(e)
	}
	p.archives, e = mobilebackup.NewRetainedArchiveStore(path, 8<<20)
	if e != nil {
		t.Fatal(e)
	}
	p.current = nil
	p.library, e = mobilebackup.NewArchiveLibraryStore(filepath.Join(stateDir, "library"), 8<<20)
	if e != nil {
		t.Fatal(e)
	}
	defer p.library.Close()
	backup := filepath.Join(stateDir, "account-archive-library", "2026-10-07")
	if e = os.MkdirAll(backup, 0700); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{first.SourceID + ".archive", ".snapshot-key", ".snapshot-claims"} {
		data, err := os.ReadFile(filepath.Join(path, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(backup, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	restored, _ := ledgerRequest(t, p, "cli_restore_account_archive", map[string]any{"source_id": first.SourceID, "backup_name": "2026-10-07", "expected_digest": first.Digest, "retention": "until_owner_deletion"})
	if restored.Code != 200 {
		t.Fatal(restored.Body.String())
	}
	for _, name := range []string{"../2026-10-07", "2026-02-31"} {
		bad, _ := ledgerRequest(t, p, "cli_restore_account_archive", map[string]any{"source_id": first.SourceID, "backup_name": name, "expected_digest": first.Digest, "retention": "until_owner_deletion"})
		if bad.Code == 200 {
			t.Fatal("invalid backup directory selected")
		}
	}
	preserved, _ := ledgerRequest(t, p, "cli_preserve_account_archive", map[string]any{"source_id": first.SourceID, "retention": "until_owner_deletion"})
	if preserved.Code != 200 || preserved.Body.String() != restored.Body.String() {
		t.Fatal(preserved.Body.String())
	}
	again, e := p.captureAccountArchiveWithDownloader(ctx, attempt.OperationID, attempt.Revision, d)
	if e != nil {
		t.Fatal(e)
	}
	for _, window := range [][2]string{{"2026-09-01T00:00:00Z", "2026-10-06T17:00:00Z"}, {"2026-09-25T17:00:00Z", "2026-09-26T17:00:00Z"}} {
		inspect, _ := ledgerRequest(t, p, "cli_inspect_account_archive", map[string]any{"source_id": first.SourceID, "since": window[0], "until": window[1]})
		if inspect.Code != 200 {
			t.Fatal(inspect.Body.String())
		}
		var value any
		_ = json.Unmarshal(inspect.Body.Bytes(), &value)
		schema, e := contracts.Compile("cli_inspect_account_archive", "output")
		if e != nil || schema.Validate(value) != nil {
			t.Fatal("invalid diagnostic contract", e)
		}
		if strings.Contains(inspect.Body.String(), "synthetic text") || strings.Contains(inspect.Body.String(), "SenderId") {
			t.Fatal("raw source disclosed")
		}
	}

	exact, _ := ledgerRequest(t, p, "cli_inspect_account_archive", map[string]any{"source_id": first.SourceID, "since": "2026-09-01T00:00:00Z", "until": "2026-10-06T17:00:00Z", "conversation_type": "group", "conversation_id": "12", "include_metadata_diagnostics": true})
	if exact.Code != 200 {
		t.Fatal(exact.Body.String())
	}
	var exactOutput struct {
		Files []mobilebackup.AccountFileCoverage `json:"files"`
		More  bool                               `json:"has_more_files"`
	}
	if e = json.Unmarshal(exact.Body.Bytes(), &exactOutput); e != nil || len(exactOutput.Files) != 1 || exactOutput.Files[0].ConversationType != "group" || exactOutput.Files[0].Metadata == nil || exactOutput.More {
		t.Fatal("exact cached metadata selection failed", e)
	}
	for _, bad := range []map[string]any{
		{"source_id": first.SourceID, "since": "2026-09-01T00:00:00Z", "until": "2026-10-06T17:00:00Z", "conversation_type": "direct"},
		{"source_id": first.SourceID, "since": "2026-09-01T00:00:00Z", "until": "2026-10-06T17:00:00Z", "conversation_type": "direct", "conversation_id": "12", "offset": 1},
		{"source_id": first.SourceID, "since": "2026-09-01T00:00:00Z", "until": "2026-10-06T17:00:00Z", "conversation_type": "direct", "conversation_id": "999"},
	} {
		response, _ := ledgerRequest(t, p, "cli_inspect_account_archive", bad)
		if response.Code == 200 {
			t.Fatal("ambiguous or missing selection accepted")
		}
	}
	// Assert: one phone dispatch/download/map, immutable source and no runtime/corpus writes.
	if !reflect.DeepEqual(first, again) || first.FileCount != 3 || first.DirectFiles != 2 || first.GroupFiles != 1 || source.offers.Load() != 1 || source.downloads.Load() != 1 || source.mappings.Load() != 1 {
		t.Fatal("capture repeated or changed")
	}
	for _, table := range []string{"messages", "message_identities", "message_events", "event_deliveries", "send_operations", "event_subscriptions"} {
		var n int
		if e = p.store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil || n != 0 {
			t.Fatal("archive modified runtime data", table, e)
		}
	}
	removed, _ := ledgerRequest(t, p, "cli_remove_account_archive", map[string]any{"source_id": first.SourceID})
	if removed.Code != 200 {
		t.Fatal(removed.Body.String())
	}
	// Durable retry works without cache or connected upstream, and preserves the receipt.
	repeated, _ := ledgerRequest(t, p, "cli_preserve_account_archive", map[string]any{"source_id": first.SourceID, "retention": "until_owner_deletion"})
	if repeated.Code != 200 || repeated.Body.String() != preserved.Body.String() || source.offers.Load() != 1 || source.downloads.Load() != 1 || source.mappings.Load() != 1 {
		t.Fatal("preservation retry acquired or changed source")
	}
	inspection, _ := ledgerRequest(t, p, "cli_inspect_account_archive", map[string]any{"source_id": first.SourceID, "source_storage": "library", "since": "2026-09-01T00:00:00Z", "until": "2026-10-06T17:00:00Z"})
	if inspection.Code != 200 || !strings.Contains(inspection.Body.String(), "until_owner_deletion") {
		t.Fatal("durable inspection failed")
	}
	forgotten, _ := ledgerRequest(t, p, "cli_remove_account_archive", map[string]any{"source_id": first.SourceID, "source_storage": "library"})
	if forgotten.Code != 200 {
		t.Fatal("durable removal failed")
	}
	denied, _ := ledgerRequest(t, p, "cli_preserve_account_archive", map[string]any{"source_id": first.SourceID, "retention": "until_owner_deletion"})
	if denied.Code == 200 || source.offers.Load() != 1 {
		t.Fatal("removed durable source resurrected")
	}
	if _, e = p.captureAccountArchiveWithDownloader(ctx, attempt.OperationID, attempt.Revision, d); e == nil || source.offers.Load() != 1 {
		t.Fatal("removed source redispatched")
	}
	for _, name := range contracts.Names() {
		if accountArchiveMethod(name) {
			t.Fatal("owner command published as MCP tool")
		}
	}
}

func TestAccountArchivePrepareRejectsMixedScope(t *testing.T) {
	// Arrange: owner ledger with no collector or source.
	p := mobileLedgerPort(t)
	// Act: attempt a mixed date/acquisition scope through the executable schema.
	response, _ := ledgerRequest(t, p, "cli_prepare_account_archive", domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000002", ArchiveScope: "account", ConversationID: "12"})
	// Assert: rejected before durable preparation or dispatch.
	var count int
	if e := p.store.DB.QueryRow("SELECT count(*) FROM mobile_backup_attempts").Scan(&count); e != nil {
		t.Fatal(e)
	}
	if response.Code == 200 || count != 0 {
		t.Fatal("mixed owner scope accepted")
	}
}

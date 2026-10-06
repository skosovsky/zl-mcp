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
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
)

type archiveProbeSource struct{ mobilePageSource }

func (s *archiveProbeSource) MobileBackupContext(ctx context.Context) (context.Context, func(), error) {
	child, cancel := context.WithCancel(ctx)
	return child, cancel, nil
}

func TestMobileArchiveProbeBoundedOneShotWithoutPersistence(t *testing.T) {
	// Arrange: independent encrypted SQLite vector and the existing session port.
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
	p := mobileLedgerPort(t)
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, _ := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	source := &archiveProbeSource{mobilePageSource: mobilePageSource{data: data, mobileServiceSource: mobileServiceSource{public: base64.StdEncoding.EncodeToString(der)}}}
	p.current = &collector.JoinManager{API: source}
	request := domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "12", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"}
	attempt, err := p.store.PrepareMobileBackup(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	downloader, err := mobilebackup.NewDownloader([]string{"archive.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	// Act: reject stale revisions before dispatch, then consume exactly once.
	if _, err = p.probeMobileArchiveWithDownloader(context.Background(), attempt.OperationID, attempt.Revision+1, downloader); err == nil {
		t.Fatal("stale revision accepted")
	}
	result, err := p.probeMobileArchiveWithDownloader(context.Background(), attempt.OperationID, attempt.Revision, downloader)
	if err != nil {
		t.Fatal(err)
	}
	_, retry := p.probeMobileArchiveWithDownloader(context.Background(), attempt.OperationID, attempt.Revision, downloader)
	// Assert: schema counts only, no second phone request or writes to corpus/journal.
	encoded, _ := json.Marshal(result)
	var value any
	_ = json.Unmarshal(encoded, &value)
	schema, err := contracts.Compile("cli_probe_mobile_backup_archive", "output")
	if err != nil || schema.Validate(value) != nil {
		t.Fatal("invalid result contract", err)
	}
	if retry == nil || source.offers.Load() != 1 || source.downloads.Load() != 1 {
		t.Fatal("one-shot boundary lost")
	}
	for _, table := range []string{"messages", "message_events", "event_subscriptions", "send_operations"} {
		var count int
		if err = p.store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("unexpected persistence", table, count, err)
		}
	}
}

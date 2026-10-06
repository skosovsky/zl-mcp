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
	"strings"
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

func TestArchiveTargetComparisonPreservesGapsAndRedactsRecords(t *testing.T) {
	// Arrange: two target rows and another message with private candidate contents.
	batch := mobilebackup.SQLiteBatch{Examined: 5, Rejected: 2, HasMore: true, Rows: []mobilebackup.SQLiteRow{
		{MessageID: "18446744073709551615", SenderID: "private-sender", Text: "private-body", Type: 0, Status: 1},
		{MessageID: "18446744073709551615", Type: 36, Status: 2},
		{MessageID: "123", Type: 33, Status: 3},
	}}
	// Act: compare exact IDs without integer precision loss; then observe a missing target.
	got := archiveTargetComparison(batch, "18446744073709551615")
	absent := archiveTargetComparison(batch, "456")
	encoded, err := json.Marshal(got)
	// Assert: control counts apply only to the target; gaps remain explicit even when absent.
	if err != nil || got["matching_records"] != 2 || got["matching_controls"] != 1 || got["examined"] != 5 || got["rejected"] != 2 || got["has_more"] != true || absent["matching_records"] != 0 || absent["has_more"] != true || absent["rejected"] != 2 {
		t.Fatal("bounded comparison lost target or coverage")
	}
	for _, private := range []string{"18446744073709551615", "private-sender", "private-body"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("private record exposed")
		}
	}
	if len(got["matching_type_counts"].(map[string]int)) != 2 || len(got["matching_statuses"].([]int64)) != 2 {
		t.Fatal("type/status evidence missing")
	}
}

func TestArchiveComparisonRejectsDifferentConversationBeforePhoneDispatch(t *testing.T) {
	// Arrange: saved send belongs to peer 12, selected archive belongs to peer 13.
	p, _, sendID := recallTestPort(t, recallDiagnosticText, "10")
	source := &archiveProbeSource{}
	p.current = &collector.JoinManager{API: source}
	attempt, err := p.store.PrepareMobileBackup(context.Background(), domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000041", ConversationType: "direct", ConversationID: "13", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	// Act: no downloader is needed because target binding must reject before networking.
	_, err = p.probeMobileArchiveComparisonWithDownloader(context.Background(), attempt.OperationID, attempt.Revision, nil, sendID)
	// Assert: attempt stays prepared and no phone request occurs.
	current, statusErr := p.store.MobileBackupAttempt(context.Background(), attempt.OperationID)
	if err == nil || statusErr != nil || current.State != "prepared" || source.offers.Load() != 0 {
		t.Fatal("mismatched target dispatched")
	}
}

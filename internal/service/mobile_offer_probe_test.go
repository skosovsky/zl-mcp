package service

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type probeOfferSource struct{ mobileServiceSource }

func (s *probeOfferSource) ReceiveMobileBackupOffer(ctx context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
	offer, err := s.mobileServiceSource.ReceiveMobileBackupOffer(ctx, o)
	if err != nil {
		return offer, err
	}
	offer.URL = "https://archive.example.com/private-marker?token=private-query-marker"
	return offer, nil
}
func probeControl(t *testing.T, p *membershipPort, id string, revision int64) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"method": "cli_probe_mobile_backup_offer", "arguments": map[string]any{"operation_id": id, "revision": revision}})
	response := httptest.NewRecorder()
	p.cliHandler().ServeHTTP(response, httptest.NewRequest("POST", "/rpc", strings.NewReader(string(body))))
	return response
}
func TestMobileOfferProbeOneShotRedactedAndNotMCP(t *testing.T) {
	// Arrange: an owned prepared attempt and a synthetic current session source.
	p := mobileLedgerPort(t)
	n := new(big.Int).Lsh(big.NewInt(1), 2047)
	n.Add(n, big.NewInt(1))
	der, _ := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	source := &probeOfferSource{mobileServiceSource: mobileServiceSource{public: base64.StdEncoding.EncodeToString(der)}}
	p.current = &collector.JoinManager{API: source}
	request := domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "12", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z", MaxArchiveBytes: 8}
	attempt, err := p.store.PrepareMobileBackup(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	// Act: stale reference is rejected before dispatch, then exact reference dispatches once.
	stale := probeControl(t, p, attempt.OperationID, attempt.Revision+1)
	if stale.Code == 200 || source.offers.Load() != 0 {
		t.Fatal("stale revision dispatched")
	}
	result := probeControl(t, p, attempt.OperationID, attempt.Revision)
	repeat := probeControl(t, p, attempt.OperationID, attempt.Revision)
	// Assert: only safe source diagnostics, never private URL/key/material or message writes.
	if result.Code != http.StatusOK || repeat.Code == 200 || source.offers.Load() != 1 {
		t.Fatal("one-shot boundary lost", result.Code, repeat.Code)
	}
	var value map[string]any
	if err = json.Unmarshal(result.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	schema, err := contracts.Compile("cli_probe_mobile_backup_offer", "output")
	if err != nil || schema.Validate(value) != nil {
		t.Fatal("invalid probe contract", err)
	}
	if value["download_host"] != "archive.example.com" || value["archive_bytes"] != "16" || value["within_archive_budget"] != false || value["download_performed"] != false || value["import_performed"] != false {
		t.Fatal(value)
	}
	for _, private := range []string{"private-marker", "private-query-marker", "synthetic-private-key", source.public} {
		if strings.Contains(result.Body.String(), private) {
			t.Fatal("probe leaked private offer material")
		}
	}
	for _, table := range []string{"messages", "message_events", "event_subscriptions", "send_operations"} {
		var count int
		if err = p.store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("probe changed unrelated state", table, count, err)
		}
	}
	for _, name := range contracts.Names() {
		if strings.Contains(name, "mobile") {
			t.Fatal("local probe leaked into MCP tools")
		}
	}
}
func TestMobileOfferProbeMissingSessionPreservesPreparedAttempt(t *testing.T) {
	// Arrange.
	p := mobileLedgerPort(t)
	request := domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "12", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"}
	attempt, err := p.store.PrepareMobileBackup(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	response := probeControl(t, p, attempt.OperationID, attempt.Revision)
	// Assert.
	saved, err := p.store.MobileBackupAttempt(context.Background(), attempt.OperationID)
	if response.Code == 200 || err != nil || saved.State != "prepared" || saved.Revision != attempt.Revision {
		t.Fatal("session-less probe changed attempt", err)
	}
}

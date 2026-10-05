package service

import (
	"context"
	"encoding/json"
	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/control"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func mobileLedgerPort(t *testing.T) *membershipPort {
	t.Helper()
	s, e := storage.OpenWithPolicy(context.Background(), filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	if _, e = s.DB.Exec("INSERT OR REPLACE INTO metadata(key,value) VALUES('account','synthetic-account')"); e != nil {
		t.Fatal(e)
	}
	return &membershipPort{store: s, lifecycle: context.Background()}
}
func ledgerRequest(t *testing.T, p *membershipPort, method string, args any) (*httptest.ResponseRecorder, storage.MobileBackupAttempt) {
	t.Helper()
	b, e := json.Marshal(map[string]any{"method": method, "arguments": args})
	if e != nil {
		t.Fatal(e)
	}
	response := httptest.NewRecorder()
	p.cliHandler().ServeHTTP(response, httptest.NewRequest("POST", "/rpc", strings.NewReader(string(b))))
	var got storage.MobileBackupAttempt
	if response.Code == http.StatusOK && json.Unmarshal(response.Body.Bytes(), &got) != nil {
		t.Fatal("invalid attempt response")
	}
	return response, got
}
func TestMobileLedgerControlPrepareRetryReadCancelWithoutCollector(t *testing.T) {
	// Arrange: production store and control handler, no current session or phone source.
	p := mobileLedgerPort(t)
	args := map[string]any{"request_id": "00000000-0000-4000-8000-000000000001", "conversation_type": "direct", "conversation_id": "12", "since": "2026-09-01T00:00:00Z", "until": "2026-10-01T00:00:00Z"}
	// Act: prepare, repeat and read through the same local handler.
	first, a := ledgerRequest(t, p, "cli_prepare_mobile_backup", args)
	repeated, b := ledgerRequest(t, p, "cli_prepare_mobile_backup", args)
	read, c := ledgerRequest(t, p, "cli_mobile_backup_status", map[string]any{"operation_id": a.OperationID})
	// Assert: stable normalized identity and schema without a second listener.
	if first.Code != 200 || repeated.Code != 200 || read.Code != 200 || a.State != "prepared" || a.OperationID == "" || a.OperationID != b.OperationID || a.OperationID != c.OperationID || a.Request.MaxMessages != 1000 || a.Request.MaxArchiveBytes != 64<<20 {
		t.Fatal("local ledger operation mismatch")
	}
	var raw any
	_ = json.Unmarshal(first.Body.Bytes(), &raw)
	schema, e := contracts.Compile("cli_prepare_mobile_backup", "output")
	if e != nil || schema.Validate(raw) != nil {
		t.Fatal("response violates contract")
	}
	if strings.Contains(first.Body.String(), "public_key") || strings.Contains(first.Body.String(), "KeyText") {
		t.Fatal("private correlation material exposed")
	}
	args["max_messages"] = 2
	conflict, _ := ledgerRequest(t, p, "cli_prepare_mobile_backup", args)
	if conflict.Code == 200 || !strings.Contains(conflict.Body.String(), "REQUEST_CONFLICT") {
		t.Fatal("changed request accepted")
	}
	cancelled, d := ledgerRequest(t, p, "cli_cancel_prepared_mobile_backup", map[string]any{"operation_id": a.OperationID, "revision": a.Revision})
	stale, _ := ledgerRequest(t, p, "cli_cancel_prepared_mobile_backup", map[string]any{"operation_id": a.OperationID, "revision": a.Revision})
	retry, _ := ledgerRequest(t, p, "cli_cancel_prepared_mobile_backup", map[string]any{"operation_id": a.OperationID, "revision": d.Revision})
	if cancelled.Code != 200 || d.State != "cancelled" || d.Revision != 1 || stale.Code == 200 || retry.Code != 200 {
		t.Fatal("cancel revision semantics failed")
	}
	for _, table := range []string{"messages", "message_identities", "message_events", "event_deliveries", "send_operations", "event_subscriptions"} {
		var n int
		if e = p.store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil || n != 0 {
			t.Fatal("ledger changed unrelated data", table, e)
		}
	}
	for _, name := range contracts.Names() {
		if mobileLedgerMethod(name) {
			t.Fatal("local route advertised as MCP tool")
		}
	}
}
func TestMobileLedgerControlRejectsAmbiguousEnvelopesBeforeWrite(t *testing.T) {
	// Arrange: valid envelope used to build malformed variants.
	p := mobileLedgerPort(t)
	valid := `{"method":"cli_prepare_mobile_backup","arguments":{"request_id":"00000000-0000-4000-8000-000000000001","conversation_type":"direct","conversation_id":"12","since":"2026-09-01T00:00:00Z","until":"2026-10-01T00:00:00Z"}}`
	cases := []string{
		strings.Replace(valid, `"conversation_id":"12"`, `"conversation_id":"12","conversation_id":"13"`, 1),
		strings.Replace(valid, `"method":"cli_prepare_mobile_backup"`, `"method":"cli_prepare_mobile_backup","method":"cli_prepare_mobile_backup"`, 1),
		valid + ` {}`, strings.Replace(valid, `"conversation_id":"12"`, `"conversation_id":"12","url":"private-marker"`, 1),
		`{"method":"cli_prepare_mobile_backup","arguments":[]}`, strings.Repeat(" ", 16<<10) + valid,
	}
	// Act / Assert: none may reserve the account's active attempt.
	for _, body := range cases {
		response := httptest.NewRecorder()
		p.cliHandler().ServeHTTP(response, httptest.NewRequest("POST", "/rpc", strings.NewReader(body)))
		if response.Code == 200 || strings.Contains(response.Body.String(), "private-marker") {
			t.Fatal("invalid envelope accepted or echoed")
		}
	}
	response := httptest.NewRecorder()
	p.cliHandler().ServeHTTP(response, httptest.NewRequest("GET", "/rpc", strings.NewReader(valid)))
	if response.Code == 200 {
		t.Fatal("GET prepared attempt")
	}
	var count int
	if e := p.store.DB.QueryRow("SELECT count(*) FROM mobile_backup_attempts").Scan(&count); e != nil || count != 0 {
		t.Fatal("invalid input wrote ledger")
	}
}
func TestMobileLedgerControlAccountOwnershipAndDispatchedCancellation(t *testing.T) {
	// Arrange.
	p := mobileLedgerPort(t)
	args := map[string]any{"request_id": "00000000-0000-4000-8000-000000000001", "conversation_type": "direct", "conversation_id": "12", "since": "2026-09-01T00:00:00Z", "until": "2026-10-01T00:00:00Z"}
	response, a := ledgerRequest(t, p, "cli_prepare_mobile_backup", args)
	if response.Code != 200 {
		t.Fatal("prepare failed")
	}
	// Simulate the journal's already-dispatched boundary; no upstream is invoked.
	if _, e := p.store.DB.Exec("UPDATE mobile_backup_attempts SET state='dispatching',revision=1 WHERE operation_id=?", a.OperationID); e != nil {
		t.Fatal(e)
	}
	// Act / Assert: a local prepared-only cancel must not claim network cancellation.
	denied, _ := ledgerRequest(t, p, "cli_cancel_prepared_mobile_backup", map[string]any{"operation_id": a.OperationID, "revision": 1})
	if denied.Code == 200 || !strings.Contains(denied.Body.String(), "INVALID_OPERATION_STATE") {
		t.Fatal("dispatched cancellation accepted")
	}
	if _, e := p.store.DB.Exec("UPDATE metadata SET value='other-synthetic-account' WHERE key='account'"); e != nil {
		t.Fatal(e)
	}
	hidden, _ := ledgerRequest(t, p, "cli_mobile_backup_status", map[string]any{"operation_id": a.OperationID})
	if hidden.Code == 200 || strings.Contains(hidden.Body.String(), a.Request.ConversationID+`"`) {
		t.Fatal("foreign account attempt exposed")
	}
	// Routes are unavailable through the membership MCP call path.
	if _, e := p.Call(context.Background(), "cli_prepare_mobile_backup", args); e == nil {
		t.Fatal("local route reached MCP")
	}
}

func TestMobileLedgerUsesProductionControlSocketWithoutSessionRestore(t *testing.T) {
	// Arrange: known account ledger and the production service in auth_required.
	dir, e := os.MkdirTemp("/tmp", "zl-mobile-ledger-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, e := storage.OpenWithPolicy(ctx, filepath.Join(dir, "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("INSERT OR REPLACE INTO metadata(key,value) VALUES('account','synthetic-account')"); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	var c config.Config
	c.StateDir, c.Collection.Mode, c.Storage.RetentionDays = dir, "all", 90
	c.MCP.Listen, c.MCP.TokenFile = "127.0.0.1:0", filepath.Join(dir, "token")
	if e = os.WriteFile(c.MCP.TokenFile, []byte(strings.Repeat("a", 64)), 0600); e != nil {
		t.Fatal(e)
	}
	ready, done := make(chan net.Addr, 1), make(chan error, 1)
	var restores atomic.Int32
	go func() {
		done <- run(ctx, c, func(context.Context, string) (collector.ListenerUpstream, error) {
			restores.Add(1)
			return nil, domain.ErrAuthenticationRequired
		}, nil, func(addr net.Addr) { ready <- addr })
	}()
	defer func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(5 * time.Second):
			t.Error("ledger service did not stop")
		}
	}()
	select {
	case <-ready:
	case <-time.After(15 * time.Second):
		t.Fatal("ledger service not ready")
	}
	// Act: actual Unix client uses the one service's store, not a local second DB.
	request, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	args := map[string]any{"request_id": "00000000-0000-4000-8000-000000000001", "conversation_type": "direct", "conversation_id": "12", "since": "2026-09-01T00:00:00Z", "until": "2026-10-01T00:00:00Z"}
	client := control.New(dir)
	prepared, e := client.Call(request, "cli_prepare_mobile_backup", args)
	if e != nil {
		t.Fatal(e)
	}
	read, e := client.Call(request, "cli_mobile_backup_status", map[string]any{"operation_id": prepared["operation_id"]})
	if e != nil {
		t.Fatal(e)
	}
	cancelled, e := client.Call(request, "cli_cancel_prepared_mobile_backup", map[string]any{"operation_id": prepared["operation_id"], "revision": prepared["revision"]})
	if e != nil {
		t.Fatal(e)
	}
	// Assert: local lifecycle completes without authentication or a phone dispatch.
	if prepared["state"] != "prepared" || read["operation_id"] != prepared["operation_id"] || cancelled["state"] != "cancelled" || restores.Load() > 1 {
		t.Fatal("control socket created another session or lost attempt")
	}
	stat, e := os.Stat(filepath.Join(dir, "collector.sock"))
	if e != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("control socket permissions changed")
	}
}

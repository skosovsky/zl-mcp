package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	adapter "github.com/skosovsky/zl-mcp/internal/adapters/operations"
	"github.com/skosovsky/zl-mcp/internal/app/history"
	"github.com/skosovsky/zl-mcp/internal/app/membership"
	"github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type operationReadBackend struct {
	calls   []string
	err     error
	results map[string]any
}

func (b *operationReadBackend) Call(_ context.Context, method string, _ any) (map[string]any, error) {
	b.calls = append(b.calls, method)
	if b.err != nil {
		return nil, b.err
	}
	value, ok := b.results[method]
	if !ok {
		return nil, errors.New("unexpected command dispatch")
	}
	wire, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	err = json.Unmarshal(wire, &result)
	return result, err
}

func TestOperationReadsUseScopedExecutorAndNeverReplaceFailuresWithCommands(t *testing.T) {
	// Arrange: native status projections, existing real account/policy authority,
	// and an independent dispatch recorder which recognizes only fixed reads.
	base, authority := readRuntime(t)
	id := "00000000-0000-4000-8000-000000000001"
	at := "2026-10-01T00:00:00Z"
	backend := &operationReadBackend{results: map[string]any{
		"zalo_get_send_status":           domain.SendOperation{RequestID: id, RecipientID: "9007199254740993", Status: "unknown", UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		"zalo_get_history_import_status": domain.HistoryImportStatus{HistoryImportRequest: domain.HistoryImportRequest{ConversationType: "group", ConversationID: "g", RequestID: id, Since: at, Until: "2026-10-02T00:00:00Z", PageSize: 50, MaxPages: 20, MaxMessages: 1000}, OperationID: id, State: "paused", SourceKind: "group_cloud", NotificationPolicy: "none", CreatedAt: at, UpdatedAt: at},
		"zalo_get_join_status":           membership.JoinStatus{OperationID: id, RequestID: id, CheckedAt: at, Status: "unknown"},
		"zalo_inspect_invite":            membership.Invite{Name: "Cà phê 🍀", Membership: "unknown"},
	}}
	reader := adapter.NewReader(backend)
	r, err := NewReads(base.search, base.conversations, authority, OperationReads{Messaging: &messaging.Handler{Reads: reader}, History: &history.Handler{Reads: reader}, Membership: &membership.Handler{Reads: reader}})
	if err != nil {
		t.Fatal(err)
	}
	binding := bound(t, r)
	for _, test := range []struct{ name, raw string }{
		{"zalo_get_send_status", `{"request_id":"` + id + `"}`},
		{"zalo_get_history_import_status", `{"operation_id":"` + id + `"}`},
		{"zalo_get_join_status", `{"operation_id":"` + id + `"}`},
		{"zalo_inspect_invite", `{"invite_url":"https://zalo.me/g/synthetic"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := len(backend.calls)
			// Act: zero and foreign bindings, valid scoped execution, and native revoke.
			_, zeroErr := r.Execute(context.Background(), Binding{}, test.name, []byte(test.raw))
			_, foreignErr := r.Execute(context.Background(), bound(t, base), test.name, []byte(test.raw))
			// Assert: no native read before trusted scope.
			if domainCode(zeroErr) != "PERMISSION_DENIED" || domainCode(foreignErr) != "PERMISSION_DENIED" || len(backend.calls) != before {
				t.Fatal("untrusted status reached native owner")
			}
			result, err := r.Execute(context.Background(), binding, test.name, []byte(test.raw))
			if err != nil || result.Value() == nil || len(backend.calls) != before+1 || backend.calls[before] != test.name {
				t.Fatalf("status did not dispatch once: %v %+v", err, backend.calls)
			}
			revoked := &domain.Error{Code: "PERMISSION_DENIED", Message: "Synthetic operation access revoked."}
			backend.err = revoked
			_, err = r.Execute(context.Background(), binding, test.name, []byte(test.raw))
			if err != revoked || len(backend.calls) != before+2 || backend.calls[before+1] != test.name {
				t.Fatalf("read error changed or command started: %v %+v", err, backend.calls)
			}
			backend.err = nil
		})
	}
	// Arrange malformed UUID and a scope superseded after binding.
	before := len(backend.calls)
	// Act.
	_, invalid := r.Execute(context.Background(), binding, "zalo_get_send_status", []byte(`{"request_id":"invalid"}`))
	authority.revision.Add(1)
	_, stale := r.Execute(context.Background(), binding, "zalo_get_join_status", []byte(`{"operation_id":"`+id+`"}`))
	// Assert.
	if domainCode(invalid) != "INVALID_ARGUMENT" || domainCode(stale) != "PERMISSION_DENIED" || len(backend.calls) != before {
		t.Fatalf("invalid/stale scope dispatched: %v %v", invalid, stale)
	}
}

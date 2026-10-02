package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func TestControlWorkflowContracts(t *testing.T) {
	for _, outcome := range []string{"joined", "pending_approval"} {
		t.Run(outcome, func(t *testing.T) {
			// Arrange: real HTTP transport and SQLite ledger, synthetic upstream only.
			j, api := manager(t)
			if outcome == "pending_approval" {
				j.API = &pendingApprovalZalo{&moderatedZalo{api}}
			}
			server := httptest.NewServer(ControlHandler(j))
			defer server.Close()
			call := func(method string, args map[string]any, status int, schemaName string) map[string]any {
				t.Helper()
				body, err := json.Marshal(map[string]any{"method": method, "arguments": args})
				if err != nil {
					t.Fatal(err)
				}
				// Act
				response, err := http.Post(server.URL+"/rpc", "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				var result map[string]any
				if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
					t.Fatal(err)
				}
				// Assert: exact route contract, not the broader control union alone.
				if response.StatusCode != status {
					t.Fatalf("%s status %d: %+v", method, response.StatusCode, result)
				}
				schema, err := contracts.Compile(schemaName, "output")
				if err != nil {
					t.Fatal(err)
				}
				if err = schema.Validate(result); err != nil {
					t.Fatalf("%s: %v; %+v", method, err, result)
				}
				return result
			}
			call("zalo_get_group", map[string]any{"group_id": "g"}, http.StatusOK, "zalo_get_group")
			preview := call("zalo_inspect_invite", map[string]any{"invite_url": "https://zalo.me/g/abc"}, http.StatusOK, "zalo_inspect_invite")
			id := preview["preview_id"].(string)
			call("cli_preview", map[string]any{"preview_id": id}, http.StatusOK, "cli_preview")
			approval := call("cli_approve", map[string]any{"preview_id": id}, http.StatusOK, "cli_approve")
			requestID := uuid.NewString()
			args := map[string]any{"plan_token": approval["plan_token"], "request_id": requestID}
			started := call("zalo_join_group", args, http.StatusOK, "zalo_join_group")
			j.Wait()
			final := call("zalo_get_join_status", map[string]any{"operation_id": started["operation_id"]}, http.StatusOK, "zalo_get_join_status")
			repeated := call("zalo_join_group", args, http.StatusOK, "zalo_join_group")
			if final["status"] != outcome || repeated["status"] != outcome || final["error"] != nil || repeated["operation_id"] != started["operation_id"] || api.calls != 1 {
				t.Fatalf("unsafe workflow: final=%+v repeat=%+v mutations=%d", final, repeated, api.calls)
			}

			// Arrange/Act/Assert: every route rejects unexpected fields with a typed error.
			for _, method := range []string{"zalo_get_group", "zalo_inspect_invite", "zalo_join_group", "zalo_get_join_status", "cli_preview", "cli_approve"} {
				result := call(method, map[string]any{"unexpected": "value"}, http.StatusBadRequest, "control")
				if result["code"] != "INVALID_ARGUMENT" {
					t.Fatalf("%s: %+v", method, result)
				}
			}
		})
	}
}

func TestInProcessMembershipPort(t *testing.T) {
	// Arrange: same persistent approval ledger as the IPC path.
	j, api := manager(t)
	ctx := context.Background()
	// Act: the MCP domain port can inspect but cannot grant approval.
	preview, err := j.Call(ctx, "zalo_inspect_invite", map[string]any{"invite_url": "https://zalo.me/g/abc"})
	if err != nil {
		t.Fatal(err)
	}
	previewID := *preview["preview_id"].(*string)
	_, approvalErr := j.Call(ctx, "cli_approve", map[string]any{"preview_id": previewID})
	_, invalidErr := j.Call(ctx, "zalo_get_group", map[string]any{"group_id": "g", "extra": true})
	// Assert: caller-controlled routing cannot bypass the trusted CLI boundary.
	if approvalErr == nil || invalidErr == nil || api.calls != 0 {
		t.Fatal("membership port bypassed contract or approval")
	}

	// Arrange: a trusted local approval authorizes one operation.
	token, err := j.Approve(ctx, previewID)
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"plan_token": token, "request_id": uuid.NewString()}
	// Act: invoke and retry through the injected domain port.
	first, err := j.Call(ctx, "zalo_join_group", args)
	if err != nil {
		t.Fatal(err)
	}
	j.Wait()
	repeated, err := j.Call(ctx, "zalo_join_group", args)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: retries preserve the operation and perform one upstream mutation.
	if first["operation_id"] != repeated["operation_id"] || api.calls != 1 {
		t.Fatalf("unsafe retry: first=%v repeated=%v calls=%d", first, repeated, api.calls)
	}
}

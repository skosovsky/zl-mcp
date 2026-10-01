package evalfixture

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSyntheticMCPResearchAndJoinGuard(t *testing.T) {
	// Arrange: real MCP and storage; no session or upstream network.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fixture, err := New(ctx, Options{Fixtures: "../../docs/evals/fixtures.json"})
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := fixture.Server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	// Act
	search, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_search_messages", Arguments: map[string]any{"query": "ca phe", "group_id": "g-1"}})
	// Assert
	if err != nil || search.IsError {
		t.Fatalf("search: %+v %v", search, err)
	}
	encoded, _ := json.Marshal(search.StructuredContent)
	var found map[string]any
	_ = json.Unmarshal(encoded, &found)
	messages, ok := found["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("diacritic result: %s", encoded)
	}
	// Act
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_get_message_context", Arguments: map[string]any{"group_id": "g-1", "message_id": "m-long"}})
	// Assert: full evidence is accessed through the real resource path.
	if err != nil || result.IsError {
		t.Fatalf("context: %+v %v", result, err)
	}
	encoded, _ = json.Marshal(result.StructuredContent)
	var contextResult map[string]any
	_ = json.Unmarshal(encoded, &contextResult)
	anchor := contextResult["anchor"].(map[string]any)
	if anchor["text_truncated"] != true {
		t.Fatal("fixture does not exercise long messages")
	}
	resource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: anchor["text_resource_uri"].(string)})
	if err != nil || len(resource.Contents) == 0 || !strings.HasSuffix(resource.Contents[0].Text, "Итог: услуга не выполнена.") {
		t.Fatalf("resource: %v", err)
	}
	// Act
	denied, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_join_group", Arguments: map[string]any{"plan_token": "fabricated", "request_id": uuid.NewString()}})
	// Assert
	if err != nil || !denied.IsError || fixture.Upstream.JoinCalls() != 0 {
		t.Fatalf("join approval bypass: %+v %v", denied, err)
	}
	if _, err = os.Stat(filepath.Join(fixture.Dir, "session.json")); !os.IsNotExist(err) {
		t.Fatal("fixture unexpectedly has credentials")
	}
}

func connectFixture(t *testing.T, ctx context.Context, fixture *Fixture) *mcp.ClientSession {
	t.Helper()
	ct, st := mcp.NewInMemoryTransports()
	ss, err := fixture.Server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-contract-test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callFixture(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || result.IsError {
		t.Fatalf("%s failed: %+v %v", name, result, err)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestSyntheticMCPApprovedJoinKeepsOneMutation(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		name := "joined"
		if timeout {
			name = "unknown_after_timeout"
		}
		t.Run(name, func(t *testing.T) {
			// Arrange: fixture setup simulates prior trusted human approval.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			approvalPath := filepath.Join(t.TempDir(), "approval.json")
			fixture, err := New(ctx, Options{Fixtures: "../../docs/evals/fixtures.json", JoinTimeout: timeout, ApprovalFile: approvalPath})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fixture.Close() })
			session := connectFixture(t, ctx, fixture)
			data, err := os.ReadFile(approvalPath)
			if err != nil {
				t.Fatal(err)
			}
			var approval map[string]any
			if err = json.Unmarshal(data, &approval); err != nil {
				t.Fatal(err)
			}
			stat, err := os.Stat(approvalPath)
			if err != nil || stat.Mode().Perm() != 0600 {
				t.Fatal("approval is not private")
			}
			args := map[string]any{"plan_token": approval["plan_token"], "request_id": uuid.NewString()}
			// Act: MCP -> production control handler -> production JoinManager.
			first := callFixture(t, ctx, session, "zalo_join_group", args)
			fixture.joins.Wait()
			operation := callFixture(t, ctx, session, "zalo_get_join_status", map[string]any{"operation_id": first["operation_id"]})
			repeated := callFixture(t, ctx, session, "zalo_join_group", args)
			// Assert: status polling and retry never create another upstream mutation.
			expected := "joined"
			if timeout {
				expected = "unknown"
			}
			if operation["status"] != expected || repeated["status"] != expected || first["operation_id"] != repeated["operation_id"] || fixture.Upstream.JoinCalls() != 1 {
				t.Fatalf("unsafe join result: operation=%+v repeated=%+v calls=%d", operation, repeated, fixture.Upstream.JoinCalls())
			}
		})
	}
}

func TestSyntheticMCPEmptyStoppedCoverage(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fixture, err := New(ctx, Options{Fixtures: "../../docs/evals/fixtures.json", Empty: true, Stopped: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.Close() })
	session := connectFixture(t, ctx, fixture)
	// Act
	status := callFixture(t, ctx, session, "zalo_get_status", map[string]any{})
	search := callFixture(t, ctx, session, "zalo_search_messages", map[string]any{"query": "ремонт", "group_id": "g-1"})
	// Assert: empty data is distinguishable from a complete history with no matches.
	if status["collector_state"] != "stopped" || status["stored_message_count"] != float64(0) || search["empty_reason"] != "no_collected_data" {
		t.Fatalf("incorrect empty/stopped response: %+v %+v", status, search)
	}
	coverage := search["coverage"].([]any)
	if len(coverage) != 1 {
		t.Fatalf("coverage=%+v", coverage)
	}
	group := coverage[0].(map[string]any)
	if group["history_complete"] != false || len(group["known_gaps"].([]any)) == 0 {
		t.Fatalf("missing corpus limits: %+v", group)
	}
}

func TestHarnessSeedApprovalAndSkillResources(t *testing.T) {
	// Arrange: only synthetic harness input is available to the MCP process.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := strings.Repeat("a", 64)
	path := filepath.Join(t.TempDir(), "synthetic-token.txt")
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	fixture, err := New(ctx, Options{Fixtures: "../../docs/evals/fixtures.json", SkillDir: "../../skills/researching-zalo-groups", ApprovalTokenFile: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.Close() })
	session := connectFixture(t, ctx, fixture)
	// Act
	resources, err := session.ListResources(ctx, nil)
	// Assert: only curated skill files, no arbitrary file access.
	if err != nil {
		t.Fatal(err)
	}
	skillCount := 0
	for _, resource := range resources.Resources {
		if strings.HasPrefix(resource.URI, "eval://skills/") {
			skillCount++
			result, e := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: resource.URI})
			if e != nil || len(result.Contents) == 0 || result.Contents[0].Text == "" {
				t.Fatalf("resource %s: %v", resource.URI, e)
			}
		}
	}
	if skillCount != 3 {
		t.Fatalf("skill resources=%d", skillCount)
	}
	if _, err = session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "eval://skills/researching-zalo-groups/../../session.json"}); err == nil {
		t.Fatal("unexpected arbitrary resource access")
	}
	// Act
	preview := callFixture(t, ctx, session, "zalo_inspect_invite", map[string]any{"invite_url": InviteURL})
	if _, exists := preview["plan_token"]; exists {
		t.Fatal("MCP exposed approval token")
	}
	started := callFixture(t, ctx, session, "zalo_join_group", map[string]any{"plan_token": token, "request_id": uuid.NewString()})
	fixture.joins.Wait()
	status := callFixture(t, ctx, session, "zalo_get_join_status", map[string]any{"operation_id": started["operation_id"]})
	// Assert
	if status["status"] != "joined" || fixture.Upstream.JoinCalls() != 1 {
		t.Fatalf("seeded approval failed: %+v", status)
	}
}

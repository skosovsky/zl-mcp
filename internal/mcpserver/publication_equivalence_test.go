package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// publicationFixtureOwner supplies only synthetic native-owner outcomes. Actual
// corpus projections, runtime authorization, SDK and both schemas remain real.
type publicationFixtureOwner struct{ stamp string }

func (owner publicationFixtureOwner) Call(_ context.Context, name string, _ any) (map[string]any, error) {
	switch name {
	case "zalo_get_group":
		return map[string]any{
			"group": map[string]any{
				"group_id":           "g",
				"name":               "Publication fixture group",
				"member_count":       3,
				"collection_enabled": true,
			},
			"description": "Synthetic metadata",
			"membership":  "member",
			"updated_at":  owner.stamp,
			"stale":       false,
		}, nil
	case "zalo_inspect_invite":
		return map[string]any{
			"group_id":           "g",
			"name":               "Publication fixture group",
			"description":        "Synthetic invite",
			"member_count":       3,
			"approval_required":  false,
			"membership":         "not_member",
			"preview_id":         "synthetic-preview",
			"preview_expires_at": owner.stamp,
		}, nil
	case "zalo_join_group", "zalo_get_join_status":
		return map[string]any{
			"status":             "joined",
			"operation_id":       "synthetic-join-operation",
			"group_id":           "g",
			"collection_enabled": true,
			"request_id":         "ac777777-7777-4777-8777-777777777777",
			"checked_at":         owner.stamp,
			"retry_safe":         true,
			"poll_after_ms":      nil,
			"error":              nil,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported synthetic owner call %s", name)
	}
}

func publicationFixtureInputs() map[string]map[string]any {
	return map[string]map[string]any{
		"zalo_get_conversation": {"conversation_type": "direct", "conversation_id": "peer"},
		"zalo_get_conversation_message_context": {
			"conversation_type": "direct",
			"conversation_id":   "peer",
			"message_id":        "direct-1",
			"before":            1,
			"after":             1,
		},
		"zalo_get_group":           {"group_id": "g"},
		"zalo_get_join_status":     {"operation_id": "synthetic-join-operation"},
		"zalo_get_message_context": {"group_id": "g", "message_id": "group-1", "before": 1, "after": 1},
		"zalo_get_status":          {},
		"zalo_inspect_invite":      {"invite_url": "https://zalo.me/g/synthetic"},
		"zalo_join_group": {
			"plan_token": "synthetic-approved-preview",
			"request_id": "ac777777-7777-4777-8777-777777777777",
		},
		"zalo_list_groups": {"query": "Publication"},
		"zalo_search_conversation_messages": {
			"query":             "publication",
			"conversation_type": "direct",
			"conversation_id":   "peer",
		},
		"zalo_search_messages": {"query": "publication", "group_id": "g"},
	}
}

func publicationValidator(t *testing.T, document map[string]any) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const location = "https://zl-mcp.local/publication-equivalence.json"
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func publicationValue(t *testing.T, value any) map[string]any {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func assertPublicationFixture(t *testing.T, name, suffix string, valid map[string]any) {
	t.Helper()
	original, err := contracts.Document(name, suffix)
	if err != nil {
		t.Fatal(err)
	}
	published, err := contracts.PublishedDocument(name, suffix)
	if err != nil {
		t.Fatal(err)
	}
	before, after := publicationValidator(t, original), publicationValidator(t, published)
	for label, validator := range map[string]*jsonschema.Schema{"original": before, "published": after} {
		if err := validator.Validate(valid); err != nil {
			t.Fatalf("%s %s valid %s fixture rejected: %v", name, suffix, label, err)
		}
	}
	invalid := publicationValue(t, valid)
	if suffix == "input" {
		invalid["publication_forbidden_argument"] = true
	} else {
		required, ok := original["required"].([]any)
		if !ok || len(required) == 0 {
			t.Fatal("nontrivial output must have required fields")
		}
		key, ok := required[0].(string)
		if !ok {
			t.Fatal("required field is not string")
		}
		delete(invalid, key)
	}
	for label, validator := range map[string]*jsonschema.Schema{"original": before, "published": after} {
		if err := validator.Validate(invalid); err == nil {
			t.Fatalf("%s %s invalid %s fixture accepted", name, suffix, label)
		}
	}
	if suffix == "output" {
		malformed := publicationValue(t, valid)
		mutatePublicationFixture(t, name, malformed)
		for label, validator := range map[string]*jsonschema.Schema{"original": before, "published": after} {
			if err := validator.Validate(malformed); err == nil {
				t.Fatalf("%s malformed nested/enum fixture accepted by %s", name, label)
			}
		}
	}
}

func fixtureObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected nontrivial object fixture, got %T", value)
	}
	return object
}

func fixtureFirst(t *testing.T, value any) map[string]any {
	t.Helper()
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected nonempty fixture array, got %#v", value)
	}
	return fixtureObject(t, items[0])
}

func mutatePublicationFixture(t *testing.T, name string, fixture map[string]any) {
	t.Helper()
	switch name {
	case "zalo_get_conversation":
		fixtureObject(t, fixture["coverage"])["history_complete"] = true
	case "zalo_get_conversation_message_context", "zalo_get_message_context":
		// Ensure neighbor/reply references really participate in valid fixtures.
		fixtureFirst(t, fixture["before"])
		fixtureFirst(t, fixture["after"])
		fixtureObject(t, fixture["reply_to"])
		fixtureObject(t, fixture["anchor"])["sent_at"] = "not-a-timestamp"
	case "zalo_get_group":
		fixtureObject(t, fixture["group"])["member_count"] = float64(-1)
	case "zalo_get_status":
		if fixture["stored_message_count"] != float64(6) {
			t.Fatal("status fixture does not include all six seeded records")
		}
		fixtureObject(t, fixture["coverage_summary"])["group_count"] = float64(-1)
	case "zalo_list_groups":
		fixtureFirst(t, fixture["groups"])["member_count"] = float64(-1)
	case "zalo_search_messages", "zalo_search_conversation_messages":
		fixtureFirst(t, fixture["messages"])["sent_at"] = "not-a-timestamp"
	case "zalo_get_join_status", "zalo_join_group":
		fixture["status"] = "invalid-operation-state"
	case "zalo_inspect_invite":
		fixture["member_count"] = float64(-1)
	default:
		t.Fatalf("no malformed fixture mapping for %s", name)
	}
}

//nolint:gocognit // AAA retains SDK execution and complete coverage proof of every changed publication schema together.
func TestPrunedSchemasPreserveRealSDKRequestsAndNontrivialResponses(t *testing.T) {
	// Arrange: one synthetic corpus containing names, text, neighbors, reply links
	// and coverage; no account state, service or native Zalo requests are used.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "messages.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = owner.BindAccount(ctx, "synthetic-publication-account"); err != nil {
		t.Fatal(err)
	}
	if err = owner.ReplaceCatalog(
		ctx,
		[]domain.Group{{ID: "g", Name: "Publication fixture group", CollectionEnabled: true}},
	); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC().Add(-time.Hour)
	for _, kind := range []string{"direct", "group"} {
		id := "peer"
		if kind == "group" {
			id = "g"
		}
		for index := range 3 {
			message := domain.Message{
				Conversation: domain.ConversationRef{Type: kind, ID: id},
				ID:           fmt.Sprintf("%s-%d", kind, index),
				SenderID:     "synthetic-sender",
				SentAt:       stamp.Add(time.Duration(index) * time.Minute),
				Text:         fmt.Sprintf("publication fixture %s message %d", kind, index),
				Source:       "replay",
			}
			if index == 1 {
				reply := fmt.Sprintf("%s-0", kind)
				message.ReplyTo = &reply
			}
			if err = owner.Put(ctx, message); err != nil {
				t.Fatal(err)
			}
		}
	}
	server, err := New(migrationReadPorts(owner, publicationFixtureOwner{stamp: stamp.Format(time.RFC3339)}))
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	serving, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serving.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "publication-equivalence", Version: "1"}, nil).
		Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	inputs := publicationFixtureInputs()
	covered := map[string]bool{}
	for _, name := range contracts.Names() {
		original, err := contracts.Document(name, "output")
		if err != nil {
			t.Fatal(err)
		}
		published, err := contracts.PublishedDocument(name, "output")
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(original, published) {
			continue
		}
		args, exists := inputs[name]
		if !exists {
			t.Fatalf("pruned schema has no actual SDK fixture: %s", name)
		}
		t.Run(name, func(t *testing.T) {
			// Act: fixture response comes from the actual SDK/tool pipeline.
			assertPublicationFixture(t, name, "input", args)
			result, callErr := client.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
			// Assert: both schemas accept the complete SDK result and reject a
			// required-field removal from that same nontrivial result.
			if callErr != nil || result == nil || result.IsError {
				t.Fatalf("fixture SDK call failed: %v %+v", callErr, result)
			}
			response := publicationValue(t, result.StructuredContent)
			assertPublicationFixture(t, name, "output", response)
			covered[name] = true
			t.Logf(
				"%s: actual SDK request + complete response accepted by both; forbidden argument + missing required output + malformed nested/enum field rejected by both",
				name,
			)
		})
	}
	if len(covered) != len(inputs) {
		t.Fatalf("fixture coverage incomplete: covered%d fixturecount%d", len(covered), len(inputs))
	}
	t.Logf("all %d pruned output schemas covered with valid/invalid actual request and response fixtures", len(covered))
}

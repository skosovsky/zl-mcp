package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	storeadapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/toolruntime"
)

type auditAdmission struct{ allow bool }

func (a *auditAdmission) AllowRead(context.Context) (bool, error) { return a.allow, nil }

type auditAuthority struct {
	*storeadapter.Reader

	denied, panic bool
}

func (a *auditAuthority) ReadIdentity(ctx context.Context) (shared.ReadIdentity, error) {
	if a.panic {
		panic("PRIVATE_SENTINEL_PANIC")
	}
	if a.denied {
		return shared.ReadIdentity{}, &domain.Error{Code: "PERMISSION_DENIED", Message: "PRIVATE_SENTINEL_DENIAL"}
	}
	return a.Reader.ReadIdentity(ctx)
}

//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestFinalAuditIncludesPreflightAndPostExecutionFailures(t *testing.T) {
	// Arrange: real app runtime, synthetic native owner, independent execution counter.
	ctx := context.Background()
	owner, err := storage.Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	ports := migrationReadPorts(owner)
	authority := &auditAuthority{Reader: storeadapter.New(owner)}
	runtime, err := toolruntime.New(ports.Search, ports.Conversations, authority, ports.capabilities())
	if err != nil {
		t.Fatal(err)
	}
	admission := &auditAdmission{allow: true}
	service := &Service{
		runtime:   runtime,
		admission: admission,
		input:     map[string]*jsonschema.Schema{},
		output:    map[string]*jsonschema.Schema{},
	}
	for _, name := range contracts.Names() {
		service.input[name], err = contracts.Compile(name, "input")
		if err != nil {
			t.Fatal(err)
		}
		service.output[name], err = contracts.Compile(name, "output")
		if err != nil {
			t.Fatal(err)
		}
	}
	observation := &wireObservation{}
	ctx = shared.WithTelemetry(ctx, observation)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	tool := "zalo_list_groups"
	run := func(raw, code string, executions int) {
		t.Helper()
		logs.Reset()
		before := observation.executions
		// Act.
		result := service.call(ctx, tool, json.RawMessage(raw))
		// Assert: exactly one final audit, with execution observations only for app calls.
		if result == nil || !result.IsError || observation.executions-before != executions {
			t.Fatal("wrong failure/execution boundary")
		}
		lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
		if len(lines) != 1 {
			t.Fatal("missing or duplicate final audit")
		}
		var entry map[string]any
		if json.Unmarshal([]byte(lines[0]), &entry) != nil || entry["code"] != code || entry["outcome"] != "error" ||
			entry["correlation_id"] == "" {
			t.Fatalf("incorrect final audit: %s", lines[0])
		}
		var clientError map[string]any
		text := result.Content[0].(*mcp.TextContent).Text
		if json.Unmarshal([]byte(text), &clientError) != nil ||
			clientError["details"].(map[string]any)["correlation_id"] != entry["correlation_id"] {
			t.Fatal("client error cannot be linked to final audit")
		}
		if strings.Contains(logs.String(), "PRIVATE_SENTINEL") {
			t.Fatal("private value in audit")
		}
	}
	authority.denied = true
	run(`{}`, "PERMISSION_DENIED", 0)
	authority.denied = false
	admission.allow = false
	run(`{}`, "RATE_LIMITED", 0)
	admission.allow = true
	run(`{"PRIVATE_SENTINEL":`, "INVALID_ARGUMENT", 0)
	run(`{"unknown":"PRIVATE_SENTINEL"}`, "INVALID_ARGUMENT", 0)
	// Arrange: execution succeeds, but output validation fails afterwards.
	tool = "zalo_search_messages"
	if err = owner.Put(
		ctx,
		domain.Message{GroupID: "g", ID: "m", SenderID: "peer", Text: "search", SentAt: time.Now(), Source: "live"},
	); err != nil {
		t.Fatal(err)
	}
	original := service.output[tool]
	service.output[tool], err = contracts.Compile("control", "output")
	if err != nil {
		t.Fatal(err)
	}
	run(`{"query":"search"}`, "STORAGE_ERROR", 1)
	service.output[tool] = original
	// Arrange: native catalogue cannot fit even after shortening.
	if err = owner.ReplaceCatalog(ctx, []domain.Group{{ID: "g", Name: strings.Repeat("名", 20000)}}); err != nil {
		t.Fatal(err)
	}
	run(`{"query":"search"}`, "RESPONSE_TOO_LARGE", 1)
	// Act/Assert: panic identity survives but never reaches the final log.
	logs.Reset()
	authority.panic = true
	func() {
		defer func() {
			if recover() != "PRIVATE_SENTINEL_PANIC" {
				t.Fatal("panic identity changed")
			}
		}()
		service.call(ctx, "zalo_list_groups", json.RawMessage(`{}`))
	}()
	var entry map[string]any
	if json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry) != nil || entry["outcome"] != "panic" ||
		entry["code"] != "TOOL_ERROR" ||
		strings.Contains(logs.String(), "PRIVATE_SENTINEL") {
		t.Fatal("unsafe/missing panic audit")
	}
}

package collector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestTrustedPreloadProbeReturnsOnlyCountsThroughControl(t *testing.T) {
	// Arrange: a source has private identity/text data which the route must omit.
	source := &preloadListener{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}}
	source.fetch = func(context.Context) (domain.PreloadSnapshot, error) {
		name := "private-name-marker"
		return domain.PreloadSnapshot{Entries: []domain.PreloadEntry{{Conversation: domain.ConversationRef{Type: "direct", ID: "private-peer-marker"}, Name: &name}}, Messages: []domain.Message{{Conversation: domain.ConversationRef{Type: "direct", ID: "private-peer-marker"}, Text: "private-body-marker"}}, DirectMessagesAvailable: true}, nil
	}
	guard := newSessionGuard(context.Background(), source)
	defer guard.cancel()
	manager := NewJoin(context.Background(), nil, guard, false)
	request := httptest.NewRequest("POST", "/rpc", strings.NewReader(`{"method":"cli_probe_preload","arguments":{}}`))
	response := httptest.NewRecorder()
	// Act
	ControlHandler(manager).ServeHTTP(response, request)
	// Assert
	if response.Code != http.StatusOK {
		t.Fatalf("probe failed: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "private-") {
		t.Fatal("probe exposed source data")
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["source_status"] != "available" || result["metadata_count"] != float64(1) || result["direct_message_count"] != float64(1) || result["messages_persisted"] != false {
		t.Fatal("probe count/policy mismatch")
	}
	output, err := contracts.Compile("cli_probe_preload", "output")
	if err != nil {
		t.Fatal(err)
	}
	if err := output.Validate(result); err != nil {
		t.Fatal(err)
	}
	// The same route is not an externally callable membership/MCP operation.
	if _, err := manager.Call(context.Background(), "cli_probe_preload", map[string]any{}); err == nil {
		t.Fatal("private probe exposed through membership Call")
	}
	for _, name := range contracts.Names() {
		if name == "cli_probe_preload" {
			t.Fatal("private probe advertised as MCP tool")
		}
	}
}

func TestTrustedPreloadProbeRejectsInjectionAndRedactsFailures(t *testing.T) {
	// Arrange
	calls := 0
	code := 114
	source := &preloadListener{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}}
	source.fetch = func(context.Context) (domain.PreloadSnapshot, error) {
		calls++
		return domain.PreloadSnapshot{}, domain.NewHistorySourceFailure(&code, errors.New("private-url-and-error-text"))
	}
	manager := NewJoin(context.Background(), nil, source, false)
	handler := ControlHandler(manager)
	// Act / Assert: request injection is rejected before source access.
	for _, body := range []string{`{"method":"cli_probe_preload","arguments":{"url":"https://example.invalid"}}`, `{"method":"cli_probe_preload"}`, `{"method":"cli_probe_preload","arguments":{},"token":"private"}`} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("POST", "/rpc", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest || calls != 0 {
			t.Fatal("probe input bypassed contract")
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/rpc", strings.NewReader(`{"method":"cli_probe_preload","arguments":{}}`)))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"source_code":114`) || strings.Contains(response.Body.String(), "private-url") {
		t.Fatal("probe failure evidence unsafe or lost")
	}
}

func TestTrustedPreloadProbeNormalizationReasonsAreClosed(t *testing.T) {
	// Arrange: adapter-supplied diagnostic text must never become output.
	for _, tc := range []struct{ message, want string }{
		{"unknown preload dialogue classification", "unknown_preload_dialogue_classification"},
		{"private-message-and-peer-marker", "unknown"},
	} {
		source := &preloadListener{scriptedListener: &scriptedListener{fakeZalo: &fakeZalo{}}}
		source.fetch = func(context.Context) (domain.PreloadSnapshot, error) {
			return domain.PreloadSnapshot{}, domain.NewHistoryPageFailure(tc.message)
		}
		// Act
		result := probePreload(context.Background(), source)
		encoded, err := json.Marshal(result)
		// Assert
		if err != nil || result["error_category"] != "invalid_source_page" || result["error_reason"] != tc.want || strings.Contains(string(encoded), "private-") {
			t.Fatal("normalization diagnostic leaked or lost evidence")
		}
		schema, err := contracts.Compile("cli_probe_preload", "output")
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(result); err != nil {
			t.Fatal(err)
		}
	}
}

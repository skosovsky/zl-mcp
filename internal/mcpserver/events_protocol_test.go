package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/control"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestEventsProtocolErrorProjection(t *testing.T) {
	for _, test := range []struct {
		name    string
		err     error
		code    int
		message string
		data    map[string]any
	}{
		{"native wrapped", fmt.Errorf("adapter: %w", &events.RPCError{Code: -32601, Message: "Unknown method.", Data: map[string]any{"method": "events/unknown"}}), -32601, "Unknown method.", map[string]any{"method": "events/unknown"}},
		{"private failure", errors.New("secret backend cause"), -32603, "Internal server error.", nil},
		{"transport validation", &protocolError{Code: -32602, Message: "Invalid parameters."}, -32602, "Invalid parameters.", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Arrange.
			response := httptest.NewRecorder()
			// Act.
			writeRPC(response, json.RawMessage(`"opaque-id"`), nil, test.err)
			// Assert.
			var actual struct {
				ID    string        `json:"id"`
				Error protocolError `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &actual); err != nil {
				t.Fatal(err)
			}
			if actual.ID != "opaque-id" || actual.Error.Code != test.code || actual.Error.Message != test.message {
				t.Fatalf("unexpected response: %s", response.Body.String())
			}
			want, _ := json.Marshal(test.data)
			got, _ := json.Marshal(actual.Error.Data)
			if string(want) != string(got) {
				t.Fatalf("data: got %s want %s", got, want)
			}
		})
	}
}

func migrationHTTP(t *testing.T, store *storage.Store, dir, token string) (*HTTPService, error) {
	t.Helper()
	protocol, err := events.NewSubscriptionManager(store, dir)
	if err != nil {
		return nil, err
	}
	return NewHTTP(store, dir, token, migrationReadPorts(store, control.New(dir)), protocol)
}

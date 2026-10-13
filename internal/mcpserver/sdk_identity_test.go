package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type sdkIdentityCase struct{ input, want string }

var sdkIdentityCases = []sdkIdentityCase{
	{"9007199254740991", "9007199254740991"}, {"9007199254740992", "9007199254740992"},
	{"9007199254740993", "9007199254740993"}, {"9007199254740994", "9007199254740994"},
	{"9223372036854775807", "9223372036854775807"}, {"-9223372036854775808", "-9223372036854775808"},
	{"9007199254740993.0", "9007199254740993"}, {"9.007199254740993e15", "9007199254740993"},
	{"100000e-2", "1000"}, {"0e999999999", "0"},
	{"0." + strings.Repeat("0", 126), "0"}, {"-0", "0"}, {"1", "1"},
	{`"1"`, `"1"`}, {`"9007199254740993"`, `"9007199254740993"`}, {`"界🍀"`, `"界🍀"`},
}

// The oracle uses independent exact rational arithmetic, never the SDK parser
// or float64, so matching rounding on both sides cannot conceal a regression.
func assertSDKIdentity(t *testing.T, actual json.RawMessage, want string) {
	t.Helper()
	decode := func(raw []byte) any {
		t.Helper()
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var v any
		if err := d.Decode(&v); err != nil {
			t.Fatalf("invalid ID %s: %v", raw, err)
		}
		return v
	}
	expected := decode([]byte(want))
	got := decode(actual)
	switch expected := expected.(type) {
	case string:
		if got != expected {
			t.Fatalf("string ID got%#v want%#v", got, expected)
		}
	case json.Number:
		number, ok := got.(json.Number)
		if !ok {
			t.Fatalf("numeric ID became %T", got)
		}
		a, ok := new(big.Rat).SetString(string(number))
		if !ok {
			t.Fatal("invalid numeric result")
		}
		b, ok := new(big.Rat).SetString(string(expected))
		if !ok || a.Cmp(b) != 0 {
			t.Fatalf("ID got%s want%s", number, expected)
		}
	default:
		t.Fatalf("unsupported oracle %T", expected)
	}
}

func sdkRawBody(method, id string, params map[string]any) []byte {
	if params == nil {
		params = map[string]any{}
	}
	params["_meta"] = map[string]any{
		"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	return []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"method":%q,"params":%s}`, id, method, encoded))
}
func sdkHTTPHeaders(req *http.Request, token, method string, params map[string]any) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	req.Header.Set("Mcp-Method", method)
	if name, ok := params["name"].(string); ok {
		req.Header.Set("Mcp-Name", name)
	}
	if uri, ok := params["uri"].(string); ok {
		req.Header.Set("Mcp-Name", uri)
	}
}
func sdkIdentityService(t *testing.T) (*HTTPService, *storage.Store, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	owner, err := storage.Open(ctx, filepath.Join(dir, "synthetic.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Close() })
	if bindAccountErr := owner.BindAccount(ctx, "synthetic-sdk-account"); bindAccountErr != nil {
		t.Fatal(bindAccountErr)
	}
	token := strings.Repeat("synthetic", 8)
	handler, err := migrationHTTP(t, owner, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	return handler, owner, token
}
//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestSDK01CodecIdentityRequestsAndResponses(t *testing.T) {
	for _, tc := range sdkIdentityCases {
		for _, kind := range []string{"request", "response", "protocol-error"} {
			t.Run(tc.input+"/"+kind, func(t *testing.T) {
				// Arrange.
				suffix := `,"method":"ping"}`
				if kind == "response" {
					suffix = `,"result":{"ok":true}}`
				}
				if kind == "protocol-error" {
					suffix = `,"error":{"code":-32601,"message":"synthetic"}}`
				}
				input := []byte(`{"jsonrpc":"2.0","id":` + tc.input + suffix)
				// Act.
				message, err := jsonrpc.DecodeMessage(input)
				if err != nil {
					t.Fatal(err)
				}
				wire, err := jsonrpc.EncodeMessage(message)
				if err != nil {
					t.Fatal(err)
				}
				// Assert.
				var envelope struct {
					ID json.RawMessage `json:"ID"`
				}
				if err := json.Unmarshal(wire, &envelope); err != nil {
					t.Fatal(err)
				}
				assertSDKIdentity(t, envelope.ID, tc.want)
			})
		}
	}
}
//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestSDK01HTTPWrappersPreserveEnvelopeIdentity(t *testing.T) {
	// Arrange: native synthetic owner; real SDK discovery/resources/status and Events wrapper.
	for _, tc := range sdkIdentityCases {
		// Each ID gets a fresh native budget; no admission bypass or quota change.
		handler, _, token := sdkIdentityService(t)
		for _, method := range []string{"server/discover", "resources/read", "tools/call", "events/list", "events/unknown"} {
			t.Run(tc.input+"/"+method, func(t *testing.T) {
				params := map[string]any{}
				if method == "resources/read" {
					params["uri"] = "zalo://capabilities"
				}
				if method == "tools/call" {
					params["name"] = "zalo_get_status"
					params["arguments"] = map[string]any{}
				}
				request := httptest.NewRequest(
					http.MethodPost,
					"http://localhost/mcp",
					bytes.NewReader(sdkRawBody(method, tc.input, params)),
				)
				sdkHTTPHeaders(request, token, method, params)
				response := httptest.NewRecorder()
				// Act.
				handler.ServeHTTP(response, request)
				// Assert.
				var envelope struct {
					ID     json.RawMessage `json:"ID"`
					Result json.RawMessage `json:"Result"`
					Error  *protocolError  `json:"Error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatalf("status%d body%s: %v", response.Code, response.Body, err)
				}
				assertSDKIdentity(t, envelope.ID, tc.want)
				if method == "events/unknown" {
					if response.Code != 404 || envelope.Error == nil || envelope.Error.Code != -32601 {
						t.Fatal("protocol error lost", response.Body)
					}
				} else if response.Code != 200 || envelope.Error != nil || len(envelope.Result) == 0 {
					t.Fatalf("request failed status%d body%s", response.Code, response.Body)
				}
				if method == "server/discover" {
					var result struct {
						Capabilities map[string]json.RawMessage `json:"Capabilities"`
					}
					if err := json.Unmarshal(
						envelope.Result,
						&result,
					); err != nil ||
						string(result.Capabilities["events"]) != "{}" {
						t.Fatal("Events capability absent")
					}
				}
			})
		}
	}
}

type sdkCountEvents struct{ calls atomic.Int64 }

func (p *sdkCountEvents) Call(context.Context, string, string, json.RawMessage) (any, error) {
	p.calls.Add(1)
	return map[string]any{}, nil
}

type sdkCountAdmission struct {
	shared.AdmissionPort

	calls atomic.Int64
}

func (p *sdkCountAdmission) AllowRead(ctx context.Context) (bool, error) {
	p.calls.Add(1)
	return p.AdmissionPort.AllowRead(ctx)
}

func TestSDK01RejectInvalidIDsBeforeApplicationDispatch(t *testing.T) {
	// Arrange: observe the real wrapper's application boundary.
	_, owner, token := sdkIdentityService(t)
	port := &sdkCountEvents{}
	ports := migrationReadPorts(owner)
	admission := &sdkCountAdmission{AdmissionPort: ports.Admission}
	ports.Admission = admission
	handler, err := NewHTTP(token, ports, port)
	if err != nil {
		t.Fatal(err)
	}
	invalid := []string{
		"1.5",
		"-1.5",
		"9223372036854775808",
		"-9223372036854775809",
		"9223372036854775807.000000001",
		"1e100000000",
		"1e-100000000",
		"0." + strings.Repeat("0", 127),
		"0." + strings.Repeat("0", 100000),
	}
	for _, id := range invalid {
		for _, method := range []string{"events/list", "tools/call", "server/discover", "resources/read"} {
			params := map[string]any{}
			if method == "tools/call" {
				params["name"] = "zalo_get_status"
				params["arguments"] = map[string]any{}
			}
			if method == "resources/read" {
				params["uri"] = "zalo://capabilities"
			}
			request := httptest.NewRequest(
				http.MethodPost,
				"http://localhost/mcp",
				bytes.NewReader(sdkRawBody(method, id, params)),
			)
			sdkHTTPHeaders(request, token, method, params)
			response := httptest.NewRecorder()
			// Act.
			handler.ServeHTTP(response, request)
			// Assert: raw JSON null, fixed protocol code, never a rounded or truncated number.
			var envelope struct {
				ID    json.RawMessage `json:"ID"`
				Error *protocolError  `json:"Error"`
			}
			if err := json.Unmarshal(
				response.Body.Bytes(),
				&envelope,
			); err != nil || string(envelope.ID) != "null" || envelope.Error == nil ||
				envelope.Error.Code != -32600 {
				t.Fatalf(
					"method%s length%d status%d body%s error%v",
					method,
					len(id),
					response.Code,
					response.Body,
					err,
				)
			}
		}
	}
	if port.calls.Load() != 0 || admission.calls.Load() != 0 {
		t.Fatalf("invalid IDs dispatched Events%d nativeReadAdmission%d", port.calls.Load(), admission.calls.Load())
	}
}

func TestSDK01CancellationNumericDynamicTypeAndMetadata(t *testing.T) {
	for _, tc := range sdkIdentityCases {
		// Arrange.
		wire := []byte(`{"requestId":` + tc.input + `,"reason":"synthetic cancellation","_meta":{"number":42}}`)
		var params mcp.CancelledParams
		// Act.
		if err := json.Unmarshal(wire, &params); err != nil {
			t.Fatal(err)
		}
		id, err := jsonrpc.MakeID(params.RequestID)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(id.Raw())
		if err != nil {
			t.Fatal(err)
		}
		// Assert: the numeric value never travels through float64; unrelated metadata does.
		if !strings.HasPrefix(tc.input, `"`) {
			if _, ok := params.RequestID.(json.Number); !ok {
				t.Fatalf("numeric cancellation type %T", params.RequestID)
			}
		}
		assertSDKIdentity(t, encoded, tc.want)
		if params.Meta["number"] != float64(42) || params.Reason != "synthetic cancellation" {
			t.Fatal("unrelated cancellation fields changed")
		}
	}
}

package mcpserver

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/control"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type HTTPService struct {
	Events    *events.SubscriptionManager
	sdk       http.Handler
	tokenHash [32]byte
	principal string
}

func NewHTTP(store *storage.Store, dir, token string) (*HTTPService, error) {
	return NewHTTPWithControl(store, dir, token, control.New(dir))
}

func NewHTTPWithControl(store *storage.Store, dir, token string, backend ControlPort) (*HTTPService, error) {
	if len(token) < 32 {
		return nil, errors.New("MCP bearer token must contain at least 32 bytes")
	}
	server, err := NewWithControl(store, backend)
	if err != nil {
		return nil, err
	}
	manager, err := events.NewSubscriptionManager(store, dir)
	if err != nil {
		return nil, err
	}
	return &HTTPService{Events: manager, tokenHash: sha256.Sum256([]byte(token)), principal: "local:" + strconv.Itoa(os.Getuid()), sdk: mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})}, nil
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, result any, err error) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	response := map[string]any{"jsonrpc": "2.0", "id": id}
	if err != nil {
		var rpc *events.RPCError
		if !errors.As(err, &rpc) {
			rpc = &events.RPCError{Code: -32603, Message: "Internal server error."}
		}
		response["error"] = rpc
	} else {
		response["result"] = result
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func writeRPCStatus(w http.ResponseWriter, status int, id json.RawMessage, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeRPC(w, id, nil, err)
}

// Match the pinned SDK's Streamable HTTP media ranges, including repeated headers.
func acceptsMCP(values []string) bool {
	var jsonOK, streamOK bool
	for _, value := range values {
		for _, raw := range strings.Split(value, ",") {
			base, _, _ := strings.Cut(raw, ";")
			switch strings.ToLower(strings.TrimSpace(base)) {
			case "application/json", "application/*":
				jsonOK = true
			case "text/event-stream", "text/*":
				streamOK = true
			case "*/*":
				jsonOK, streamOK = true, true
			}
		}
	}
	return jsonOK && streamOK
}

func localHTTPHost(raw string) bool {
	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func (s *HTTPService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	if !localHTTPHost(r.Host) || r.Header.Get("Origin") != "" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	digest := sha256.Sum256([]byte(strings.TrimPrefix(auth, "Bearer ")))
	if subtle.ConstantTimeCompare(digest[:], s.tokenHash[:]) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		s.sdk.ServeHTTP(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var request rpcRequest
	if json.Unmarshal(body, &request) != nil {
		writeRPC(w, nil, nil, &events.RPCError{Code: -32700, Message: "Parse error."})
		return
	}
	if request.JSONRPC != "2.0" || request.Method == "" {
		writeRPC(w, nil, nil, &events.RPCError{Code: -32600, Message: "Invalid request."})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if strings.HasPrefix(request.Method, "events/") {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			http.Error(w, "Content-Type must be 'application/json'", http.StatusUnsupportedMediaType)
			return
		}
		if !acceptsMCP(r.Header.Values("Accept")) {
			http.Error(w, "Accept must contain both 'application/json' and 'text/event-stream'", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Mcp-Method") != request.Method {
			writeRPCStatus(w, http.StatusBadRequest, request.ID, &events.RPCError{Code: mcp.CodeHeaderMismatch, Message: "Missing or mismatched Mcp-Method header."})
			return
		}
		var params struct {
			Meta map[string]any `json:"_meta"`
		}
		if len(request.Params) != 0 && json.Unmarshal(request.Params, &params) != nil {
			writeRPC(w, request.ID, nil, &events.RPCError{Code: -32602, Message: "Invalid parameters."})
			return
		}
		if _, ok := params.Meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any); !ok {
			writeRPC(w, request.ID, nil, &events.RPCError{Code: -32602, Message: "Missing or invalid client capabilities."})
			return
		}
		version, _ := params.Meta["io.modelcontextprotocol/protocolVersion"].(string)
		header := r.Header.Get("MCP-Protocol-Version")
		if header == "" || (version != "" && header != version) {
			writeRPCStatus(w, http.StatusBadRequest, request.ID, &events.RPCError{Code: mcp.CodeHeaderMismatch, Message: "Missing or mismatched MCP-Protocol-Version header."})
			return
		}
		if version == "" {
			writeRPCStatus(w, http.StatusBadRequest, request.ID, &events.RPCError{Code: -32602, Message: "Missing or invalid protocol version metadata."})
			return
		}
		if version != "2026-07-28" {
			writeRPCStatus(w, http.StatusBadRequest, request.ID, &events.RPCError{Code: mcp.CodeUnsupportedProtocolVersion, Message: "Unsupported protocol version.", Data: map[string]any{"supported": []string{"2026-07-28"}, "requested": version}})
			return
		}
		var id any
		if len(request.ID) == 0 || json.Unmarshal(request.ID, &id) != nil {
			writeRPC(w, nil, nil, &events.RPCError{Code: -32600, Message: "Events require a request ID."})
			return
		}
		switch id.(type) {
		case string, float64:
		default:
			writeRPC(w, nil, nil, &events.RPCError{Code: -32600, Message: "Invalid request ID."})
			return
		}
		result, err := s.Events.Call(r.Context(), request.Method, s.principal, request.Params)
		var rpc *events.RPCError
		if errors.As(err, &rpc) && rpc.Code == -32601 {
			writeRPCStatus(w, http.StatusNotFound, request.ID, err)
			return
		}
		writeRPC(w, request.ID, result, err)
		return
	}
	if request.Method == "server/discover" {
		capture := &jsonResponseCapture{header: http.Header{}}
		s.sdk.ServeHTTP(capture, r)
		var response map[string]any
		if capture.status == http.StatusOK && json.Unmarshal(capture.body.Bytes(), &response) == nil {
			if result, ok := response["result"].(map[string]any); ok {
				if capabilities, ok := result["capabilities"].(map[string]any); ok {
					capabilities["events"] = map[string]any{}
				}
				wire, err := json.Marshal(response)
				if err == nil {
					capture.body.Reset()
					capture.body.Write(wire)
					capture.header.Del("Content-Length")
				}
			}
		}
		for key, values := range capture.header {
			w.Header()[key] = values
		}
		if capture.status == 0 {
			capture.status = http.StatusOK
		}
		w.WriteHeader(capture.status)
		w.Write(capture.body.Bytes())
		return
	}
	s.sdk.ServeHTTP(w, r)
}

// Discovery in stateless JSON mode is a finite response. SDK errors are forwarded
// intact; only the successful capability object gains the Events extension.
type jsonResponseCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *jsonResponseCapture) Header() http.Header { return c.header }
func (c *jsonResponseCapture) WriteHeader(code int) {
	if c.status == 0 {
		c.status = code
	}
}
func (c *jsonResponseCapture) Write(body []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(body)
}

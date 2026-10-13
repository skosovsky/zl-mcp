package mcpserver

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type HTTPService struct {
	Events    EventsProtocolPort
	sdk       http.Handler
	tokenHash [32]byte
	principal string
}

func NewHTTP(token string, reads Ports, protocol EventsProtocolPort) (*HTTPService, error) {
	if len(token) < minimumBearerTokenBytes {
		return nil, errors.New("MCP bearer token must contain at least 32 bytes")
	}
	server, err := New(reads)
	if err != nil {
		return nil, err
	}
	if protocol == nil {
		//nolint:staticcheck // Keep the established externally observable error spelling unchanged.
		return nil, errors.New("Events protocol port is required")
	}
	return &HTTPService{
		Events:    protocol,
		tokenHash: sha256.Sum256([]byte(token)),
		principal: "local:" + strconv.Itoa(os.Getuid()),
		sdk: mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return server },
			//nolint:exhaustruct_v5 // Keep pinned SDK defaults for omitted transport options; explicit security/session options are unchanged.
			&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, PropagateRequestCancellation: true},
		),
	}, nil
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
		response["error"] = projectProtocolError(err)
	} else {
		response["result"] = result
	}
	w.Header().Set("Content-Type", contentTypeJSON)
	if json.NewEncoder(w).Encode(response) != nil {
		return // The response may be partial; never retry it.
	}
}

func writeRPCStatus(w http.ResponseWriter, status int, id json.RawMessage, err error) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	writeRPC(w, id, nil, err)
}

// Match the pinned SDK's Streamable HTTP media ranges, including repeated headers.
func acceptsMCP(values []string) bool {
	var jsonOK, streamOK bool
	for _, value := range values {
		for raw := range strings.SplitSeq(value, ",") {
			base, _, _ := strings.Cut(raw, ";")
			switch strings.ToLower(strings.TrimSpace(base)) {
			case contentTypeJSON, "application/*":
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
	if !s.authorizeHTTP(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		s.sdk.ServeHTTP(w, r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumDiscoveryRequestBytes))
	if err != nil {
		http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var request rpcRequest
	if json.Unmarshal(body, &request) != nil {
		//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
		writeRPC(w, nil, nil, &protocolError{Code: -32700, Message: "Parse error."})
		return
	}
	if request.JSONRPC != "2.0" || request.Method == "" {
		//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
		writeRPC(w, nil, nil, &protocolError{Code: -32600, Message: "Invalid request."})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if observer := shared.Telemetry(
		r.Context(),
	); observer != nil &&
		(request.Method == "tools/call" || request.Method == "resources/read") {
		var params struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(request.Params, &params)
		transport := domain.TransportHTTP
		if request.Method == "resources/read" {
			transport = domain.TransportResource
		}
		//nolint:exhaustruct_v5 // telemetryResponse preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
		counted := &telemetryResponse{ResponseWriter: w}
		w = counted
		defer func() { observer.ResponseBytes(r.Context(), params.Name, transport, counted.bytes) }()
	}
	if discoveryMethod(request.Method) {
		//nolint:exhaustruct_v5 // discoveryResponse preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
		observed := &discoveryResponse{ResponseWriter: w, capture: request.Method == methodToolsList}
		w = observed
		defer logDiscovery(request.Method, r.Header.Get("Mcp-Protocol-Version"), observed, time.Now())
	}
	if strings.HasPrefix(request.Method, "events/") {
		s.serveEvents(w, r, request)
		return
	}
	if request.Method == "server/discover" {
		s.serveDiscovery(w, r)
		return
	}
	s.sdk.ServeHTTP(w, r)
}

// addEventsCapability preserves every SDK envelope field as raw JSON, especially
// the ID. It only re-encodes the capability extension it owns.
func addEventsCapability(wire []byte) ([]byte, error) {
	var response, result, capabilities map[string]json.RawMessage
	if err := json.Unmarshal(wire, &response); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(response["result"], &result); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(result["capabilities"], &capabilities); err != nil {
		return nil, err
	}
	if response == nil || result == nil || capabilities == nil {
		return nil, errors.New("missing discovery capabilities")
	}
	capabilities["events"] = json.RawMessage("{}")
	encoded, err := json.Marshal(capabilities)
	if err != nil {
		return nil, err
	}
	result["capabilities"] = encoded
	encoded, err = json.Marshal(result)
	if err != nil {
		return nil, err
	}
	response["result"] = encoded
	return json.Marshal(response)
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

// telemetryResponse counts bytes accepted by the actual wire writer, including
// the JSON-RPC envelope and SDK escaping; it neither buffers nor re-encodes.
type telemetryResponse struct {
	http.ResponseWriter

	bytes int64
}

func (w *telemetryResponse) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)
	w.bytes += int64(n)
	return n, err
}
func (w *telemetryResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *telemetryResponse) Flush()                      { _ = http.NewResponseController(w.ResponseWriter).Flush() }

func (s *HTTPService) serveEvents(w http.ResponseWriter, r *http.Request, request rpcRequest) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != contentTypeJSON {
		http.Error(w, "Content-Type must be 'application/json'", http.StatusUnsupportedMediaType)
		return
	}
	if !acceptsMCP(r.Header.Values("Accept")) {
		http.Error(w, "Accept must contain both 'application/json' and 'text/event-stream'", http.StatusBadRequest)
		return
	}
	var id any
	decoder := json.NewDecoder(bytes.NewReader(request.ID))
	decoder.UseNumber()
	idDecodeErr := decoder.Decode(&id)
	validated, idError := jsonrpc.MakeID(id)
	// Invalid numeric IDs cannot be echoed by any subsequent admission error.
	// Other malformed/missing IDs retain the established metadata-error order.
	if _, numeric := id.(json.Number); numeric && idError != nil {
		//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
		writeRPC(w, nil, nil, &protocolError{Code: -32600, Message: "Invalid request ID."})
		return
	}
	if r.Header.Get("Mcp-Method") != request.Method {
		writeRPCStatus(
			w,
			http.StatusBadRequest,
			request.ID,
			//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
			&protocolError{Code: mcp.CodeHeaderMismatch, Message: "Missing or mismatched Mcp-Method header."},
		)
		return
	}
	var params struct {
		Meta map[string]any `json:"_meta"`
	}
	if len(request.Params) != 0 && json.Unmarshal(request.Params, &params) != nil {
		//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
		writeRPC(w, request.ID, nil, &protocolError{Code: -32602, Message: "Invalid parameters."})
		return
	}
	if _, ok := params.Meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any); !ok {
		writeRPC(
			w,
			request.ID,
			nil,
			//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
			&protocolError{Code: -32602, Message: "Missing or invalid client capabilities."},
		)
		return
	}
	version, _ := params.Meta["io.modelcontextprotocol/protocolVersion"].(string)
	header := r.Header.Get("Mcp-Protocol-Version")
	if header == "" || (version != "" && header != version) {
		writeRPCStatus(
			w,
			http.StatusBadRequest,
			request.ID,
			//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
			&protocolError{
				Code:    mcp.CodeHeaderMismatch,
				Message: "Missing or mismatched MCP-Protocol-Version header.",
			},
		)
		return
	}
	if version == "" {
		writeRPCStatus(
			w,
			http.StatusBadRequest,
			request.ID,
			//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
			&protocolError{Code: -32602, Message: "Missing or invalid protocol version metadata."},
		)
		return
	}
	if version != protocolVersion {
		writeRPCStatus(
			w,
			http.StatusBadRequest,
			request.ID,
			&protocolError{
				Code:    mcp.CodeUnsupportedProtocolVersion,
				Message: "Unsupported protocol version.",
				Data:    map[string]any{"supported": []string{protocolVersion}, "requested": version},
			},
		)
		return
	}
	if len(request.ID) == 0 || idDecodeErr != nil || id == nil {
		//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
		writeRPC(w, nil, nil, &protocolError{Code: -32600, Message: "Events require a request ID."})
		return
	}
	if idError != nil || !validated.IsValid() {
		//nolint:exhaustruct_v5 // Protocol error details remain absent for this error code; preserve the existing JSON-RPC response.
		writeRPC(w, nil, nil, &protocolError{Code: -32600, Message: "Invalid request ID."})
		return
	}
	result, err := s.Events.Call(r.Context(), request.Method, s.principal, request.Params)
	if projectProtocolError(err).Code == -32601 {
		writeRPCStatus(w, http.StatusNotFound, request.ID, err)
		return
	}
	writeRPC(w, request.ID, result, err)
}

func (s *HTTPService) serveDiscovery(w http.ResponseWriter, r *http.Request) {
	//nolint:exhaustruct_v5 // jsonResponseCapture preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
	capture := &jsonResponseCapture{header: http.Header{}}
	s.sdk.ServeHTTP(capture, r)
	if capture.status == http.StatusOK {
		if wire, err := addEventsCapability(capture.body.Bytes()); err == nil {
			capture.body.Reset()
			capture.body.Write(wire)
			capture.header.Del("Content-Length")
		}
	}
	maps.Copy(w.Header(), capture.header)
	if capture.status == 0 {
		capture.status = http.StatusOK
	}
	w.WriteHeader(capture.status)
	if _, err := w.Write(capture.body.Bytes()); err != nil {
		return // Do not replay a partially delivered response.
	}
}

func (s *HTTPService) authorizeHTTP(w http.ResponseWriter, r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	digest := sha256.Sum256([]byte(strings.TrimPrefix(auth, "Bearer ")))
	if subtle.ConstantTimeCompare(digest[:], s.tokenHash[:]) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

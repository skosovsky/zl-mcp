package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	operationsadapter "github.com/skosovsky/zl-mcp/internal/adapters/operations"
	storeadapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	messagingapp "github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	nativeMessaging "github.com/skosovsky/zl-mcp/internal/messaging"
)

type sdkInvocation struct {
	message string
	ctx     context.Context
}
type sdkBlockingRead struct {
	*storeadapter.Reader

	entered chan sdkInvocation
	release map[string]chan struct{}
	calls   atomic.Int64
}

func (p *sdkBlockingRead) GroupContext(
	ctx context.Context,
	input conversations.ContextRequest,
) (conversations.GroupContext, error) {
	p.calls.Add(1)
	if gate := p.release[input.MessageID]; gate != nil {
		select {
		case p.entered <- sdkInvocation{input.MessageID, ctx}:
		case <-ctx.Done():
			return conversations.GroupContext{}, ctx.Err()
		}
		select {
		case <-gate:
		case <-ctx.Done():
			return conversations.GroupContext{}, ctx.Err()
		}
	}
	return p.Reader.GroupContext(ctx, input)
}
func sdkBlockingHTTP(t *testing.T) (*HTTPService, string, *sdkBlockingRead) {
	t.Helper()
	_, owner, token := sdkIdentityService(t)
	for _, id := range []string{"first", "second", "reused"} {
		if err := owner.Put(
			context.Background(),
			domain.Message{
				GroupID:  "g",
				ID:       id,
				SenderID: "synthetic-peer",
				Text:     "synthetic " + id,
				SentAt:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
				Source:   "live",
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	reader := &sdkBlockingRead{
		Reader:  storeadapter.New(owner),
		entered: make(chan sdkInvocation, 8),
		release: map[string]chan struct{}{
			"first":  make(chan struct{}),
			"second": make(chan struct{}),
			"reused": make(chan struct{}),
		},
	}
	ports := migrationReadPorts(owner)
	ports.Conversations.Reads = reader
	protocol, err := events.NewSubscriptionManager(owner, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHTTP(token, ports, protocol)
	if err != nil {
		t.Fatal(err)
	}
	return handler, token, reader
}
func sdkNextInvocation(t *testing.T, reader *sdkBlockingRead) sdkInvocation {
	t.Helper()
	select {
	case invocation := <-reader.entered:
		return invocation
	case <-time.After(5 * time.Second):
		t.Fatal("application invocation did not arrive")
		return sdkInvocation{}
	}
}
func sdkContextArgs(message string) map[string]any {
	return map[string]any{
		"name":      "zalo_get_message_context",
		"arguments": map[string]any{"group_id": "g", "message_id": message, "before": 0, "after": 0},
	}
}

type sdkHTTPResult struct {
	id, body []byte
	err      error
}

func sdkHTTPCall(ctx context.Context, endpoint, token, id, message string) <-chan sdkHTTPResult {
	done := make(chan sdkHTTPResult, 1)
	go func() {
		params := sdkContextArgs(message)
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			endpoint+"/mcp",
			bytes.NewReader(sdkRawBody("tools/call", id, params)),
		)
		if err != nil {
			done <- sdkHTTPResult{err: err}
			return
		}
		sdkHTTPHeaders(request, token, "tools/call", params)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			done <- sdkHTTPResult{err: err}
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 256<<10))
		var envelope struct {
			ID json.RawMessage `json:"ID"`
		}
		if err == nil {
			err = json.Unmarshal(body, &envelope)
		}
		done <- sdkHTTPResult{envelope.ID, body, err}
	}()
	return done
}
func sdkNextHTTP(t *testing.T, done <-chan sdkHTTPResult) sdkHTTPResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP response stuck")
		return sdkHTTPResult{}
	}
}
func sdkAssertContextReply(t *testing.T, result sdkHTTPResult, id, message string) {
	t.Helper()
	if result.err != nil {
		t.Fatal(result.err)
	}
	assertSDKIdentity(t, result.id, id)
	var envelope struct {
		Result struct {
			IsError           bool `json:"IsError"`
			StructuredContent struct {
				Anchor domain.Message `json:"Anchor"`
			} `json:"StructuredContent"`
		} `json:"Result"`
		Error json.RawMessage `json:"Error"`
	}
	if err := json.Unmarshal(result.body, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Result.IsError || len(envelope.Error) != 0 || envelope.Result.StructuredContent.Anchor.ID != message ||
		envelope.Result.StructuredContent.Anchor.Text != "synthetic "+message {
		t.Fatalf("wrong correlation: %s", result.body)
	}
}
func TestSDK01HTTPNeighboringIDsOutOfOrder(t *testing.T) {
	// Arrange: native typed reads are gated independently; IDs collapse on float64.
	handler, token, reader := sdkBlockingHTTP(t)
	endpoint := httptest.NewServer(handler)
	defer endpoint.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first := sdkHTTPCall(ctx, endpoint.URL, token, "9007199254740992", "first")
	second := sdkHTTPCall(ctx, endpoint.URL, token, "9007199254740993", "second")
	a, b := sdkNextInvocation(t, reader), sdkNextInvocation(t, reader)
	if a.message == b.message {
		t.Fatal("calls collapsed")
	}
	// Act: complete the second application invocation first.
	close(reader.release["second"])
	reply := sdkNextHTTP(t, second)
	// Assert: result and envelope both belong to the second request.
	sdkAssertContextReply(t, reply, "9007199254740993", "second")
	select {
	case <-first:
		t.Fatal("first returned before release")
	default:
	}
	close(reader.release["first"])
	sdkAssertContextReply(t, sdkNextHTTP(t, first), "9007199254740992", "first")
}
func TestSDK01HTTPDisconnectCancelsOnlySelectedInvocation(t *testing.T) {
	// Arrange: stateless HTTP does not route separate notification cancellation;
	// the supported disconnect lifecycle is tied to each request's carrier.
	handler, token, reader := sdkBlockingHTTP(t)
	endpoint := httptest.NewServer(handler)
	defer endpoint.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	selectedCtx, selectedCancel := context.WithCancel(ctx)
	defer selectedCancel()
	first := sdkHTTPCall(selectedCtx, endpoint.URL, token, "9007199254740992", "first")
	neighbor := sdkHTTPCall(ctx, endpoint.URL, token, "9007199254740993", "second")
	invocations := map[string]sdkInvocation{}
	for range 2 {
		v := sdkNextInvocation(t, reader)
		invocations[v.message] = v
	}
	// Act.
	selectedCancel()
	failed := sdkNextHTTP(t, first)
	// Assert.
	if !errors.Is(failed.err, context.Canceled) {
		t.Fatal("selected carrier not cancelled", failed.err)
	}
	select {
	case <-invocations["first"].ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("application context survived disconnect")
	}
	if invocations["second"].ctx.Err() != nil {
		t.Fatal("adjacent invocation cancelled")
	}
	close(reader.release["second"])
	sdkAssertContextReply(t, sdkNextHTTP(t, neighbor), "9007199254740993", "second")
	// Reuse the old envelope ID for a new invocation after the cancelled one.
	reused := sdkHTTPCall(ctx, endpoint.URL, token, "9007199254740992", "reused")
	v := sdkNextInvocation(t, reader)
	if v.message != "reused" || v.ctx.Err() != nil {
		t.Fatal("old lifecycle damaged reuse")
	}
	close(reader.release["reused"])
	sdkAssertContextReply(t, sdkNextHTTP(t, reused), "9007199254740992", "reused")
}

type sdkSyntheticSender struct {
	calls     atomic.Int64
	ambiguous bool
}

func (s *sdkSyntheticSender) SendDirect(context.Context, string, string, *domain.SendQuote) (string, error) {
	s.calls.Add(1)
	if s.ambiguous {
		return "", context.DeadlineExceeded
	}
	return "synthetic-platform-message", nil
}

type sdkLostResponse struct {
	*httptest.ResponseRecorder

	attempted atomic.Int64
}

func (w *sdkLostResponse) Write(body []byte) (int, error) {
	w.attempted.Add(int64(len(body)))
	return 0, io.ErrClosedPipe
}
func (w *sdkLostResponse) Unwrap() http.ResponseWriter { return w.ResponseRecorder }

//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestSDK01LostReplyRetainsDurableUUIDAndNeverResends(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "unknown"}[ambiguous], func(t *testing.T) {
			// Arrange: real native ledger owner; only the upstream sender is synthetic.
			_, owner, token := sdkIdentityService(t)
			sender := &sdkSyntheticSender{ambiguous: ambiguous}
			manager := &nativeMessaging.Manager{
				Store:      owner,
				Sender:     sender,
				Enabled:    true,
				Recipients: map[string]bool{"synthetic-peer": true},
			}
			ports := migrationReadPorts(owner)
			adapter := operationsadapter.NewReader(manager)
			ports.Operations.Messaging = &messagingapp.Handler{Reads: adapter, Commands: adapter}
			protocol, err := events.NewSubscriptionManager(owner, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			handler, err := NewHTTP(token, ports, protocol)
			if err != nil {
				t.Fatal(err)
			}
			uuid := "00000000-0000-4000-8000-000000000071"
			params := map[string]any{
				"name": "zalo_send_direct_message",
				"arguments": map[string]any{
					"recipient_id": "synthetic-peer",
					"text":         "synthetic effect only",
					"request_id":   uuid,
				},
			}
			request := httptest.NewRequest(
				http.MethodPost,
				"http://localhost/mcp",
				bytes.NewReader(sdkRawBody("tools/call", "9007199254740992", params)),
			)
			sdkHTTPHeaders(request, token, "tools/call", params)
			lost := &sdkLostResponse{ResponseRecorder: httptest.NewRecorder()}
			// Act: effect/ledger completion occurs, but the actual transport write fails.
			handler.ServeHTTP(lost, request)
			// Assert: no reply reached the client and native UUID state survived.
			state, err := owner.SendStatus(context.Background(), uuid)
			if err != nil {
				t.Fatal(err)
			}
			wantState := "sent"
			if ambiguous {
				wantState = "unknown"
			}
			if sender.calls.Load() != 1 || state.Status != wantState || lost.attempted.Load() == 0 ||
				lost.Body.Len() != 0 {
				t.Fatalf("effect%d state%+v attempted%d", sender.calls.Load(), state, lost.attempted.Load())
			}
			// Act: retry with the same domain UUID and a different envelope ID.
			retry := httptest.NewRequest(
				http.MethodPost,
				"http://localhost/mcp",
				bytes.NewReader(sdkRawBody("tools/call", `"new-envelope"`, params)),
			)
			sdkHTTPHeaders(retry, token, "tools/call", params)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, retry)
			// Assert: correlation changes, domain identity/state and single effect do not.
			var envelope struct {
				ID     json.RawMessage `json:"ID"`
				Result struct {
					IsError           bool `json:"IsError"`
					StructuredContent struct {
						RequestID string `json:"request_id"`
						Status    string `json:"Status"`
						RetrySafe bool   `json:"retry_safe"`
					} `json:"StructuredContent"`
				} `json:"Result"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			assertSDKIdentity(t, envelope.ID, `"new-envelope"`)
			if sender.calls.Load() != 1 || envelope.Result.IsError ||
				envelope.Result.StructuredContent.RequestID != uuid ||
				envelope.Result.StructuredContent.Status != wantState ||
				envelope.Result.StructuredContent.RetrySafe {
				t.Fatalf("resend/identity/regression: %s calls%d", response.Body, sender.calls.Load())
			}
			if !strings.Contains(response.Body.String(), wantState) {
				t.Fatal("state not delivered")
			}
		})
	}
}

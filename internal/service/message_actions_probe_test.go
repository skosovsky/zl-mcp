package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type actionProbeSource struct{ calls int }

func (s *actionProbeSource) MarkSeen(context.Context, []domain.MessageActionTarget) (domain.ReadReceiptResult, error) {
	s.calls++
	return domain.ReadReceiptResult{}, nil
}
func (s *actionProbeSource) React(context.Context, domain.MessageActionTarget, string, bool) error {
	s.calls++
	return nil
}

func TestMessageActionProbePrivateRouteAndDurableReplay(t *testing.T) {
	// Arrange: real owner-only handler and synthetic retained provenance.
	store, err := storage.OpenWithPolicy(
		t.Context(),
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.BindAccount(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	ref := domain.ConversationRef{Type: "direct", ID: "peer"}
	message := domain.Message{
		Conversation:            ref,
		ID:                      "123",
		SenderID:                "peer",
		Direction:               "incoming",
		Source:                  "live",
		SentAt:                  time.UnixMilli(1791028800000),
		Text:                    "synthetic",
		ProtocolClientMessageID: "456",
		SeenMetadata: &domain.SeenMetadata{
			ClientMessageID: "456",
			RecipientID:     "owner",
			MessageType:     "webchat",
			Timestamp:       "1791028800000",
		},
	}
	if err = store.Put(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	source := &actionProbeSource{}
	port := &membershipPort{store: store, actions: source, readChats: map[domain.ConversationRef]bool{ref: true}}
	body := `{"method":"cli_message_action_probe","arguments":{"request_id":"a0000000-0000-4000-8000-000000000001","conversation_type":"direct","conversation_id":"peer","message_ids":["123"],"plan":"read"}}`
	// Act / Assert: same UUID cannot repeat the native step.
	for range 2 {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(body))
		port.cliHandler().ServeHTTP(response, request)
		var result domain.MessageActionOperation
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &result) != nil ||
			result.Status != "accepted" {
			t.Fatal("valid local probe failed", response.Code)
		}
	}
	if source.calls != 1 {
		t.Fatal("diagnostic replay duplicated native write")
	}
	// A malformed or duplicate argument never reaches the source.
	invalid := strings.Replace(body, `"plan":"read"`, `"plan":"read","plan":"add"`, 1)
	response := httptest.NewRecorder()
	port.cliHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(invalid)))
	if response.Code == 200 || source.calls != 1 {
		t.Fatal("duplicate JSON bypassed the contract")
	}
	// Explicit write probe routes are absent from public tool discovery.
	for _, name := range contracts.Names() {
		if strings.Contains(name, "probe") {
			t.Fatal("private probe exposed as MCP tool")
		}
	}
}

func TestMessageActionProbeArgumentsRejectReplacementAndNativeFields(t *testing.T) {
	base := `{"request_id":"a0000000-0000-4000-8000-000000000001","conversation_type":"direct","conversation_id":"peer","message_ids":["123"],"plan":"add","reaction":"heart"}`
	for _, body := range []string{
		strings.Replace(base, `"plan":"add"`, `"plan":"clear_add"`, 1),
		strings.Replace(base, `"heart"`, `"arbitrary-emoji"`, 1),
		strings.Replace(base, `"reaction":"heart"`, `"reaction":"heart","rType":5`, 1),
		strings.Replace(base, `"reaction":"heart"`, `"reaction":"heart","reaction":"like"`, 1),
	} {
		// Arrange / Act.
		_, err := messageActionProbeArguments(bytes.NewBufferString(body))
		// Assert: the diagnostic primitive never silently composes replacement steps.
		if err == nil {
			t.Fatal("unsupported diagnostic arguments accepted")
		}
	}
	if _, err := messageActionProbeArguments(strings.NewReader(base)); err != nil {
		t.Fatal("valid selected reaction rejected", err)
	}
}

type cancellingActionProbeSource struct{ started chan struct{} }

func (s cancellingActionProbeSource) MarkSeen(
	ctx context.Context,
	_ []domain.MessageActionTarget,
) (domain.ReadReceiptResult, error) {
	close(s.started)
	<-ctx.Done()
	return domain.ReadReceiptResult{}, ctx.Err()
}
func (s cancellingActionProbeSource) React(context.Context, domain.MessageActionTarget, string, bool) error {
	return nil
}

func TestMessageActionProbeFollowsServiceAndSessionCancellation(t *testing.T) {
	for _, parent := range []string{"service", "session"} {
		t.Run(parent, func(t *testing.T) {
			assertActionProbeCancellation(t, parent)
		})
	}
}

func assertActionProbeCancellation(t *testing.T, parent string) {
	t.Helper()
	// Arrange: a cancellable lifecycle and a blocked synthetic native request.
	store, err := storage.OpenWithPolicy(
		t.Context(),
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.BindAccount(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	ref := domain.ConversationRef{Type: "direct", ID: "peer"}
	message := domain.Message{
		Conversation:            ref,
		ID:                      "123",
		SenderID:                "peer",
		Direction:               "incoming",
		Source:                  "live",
		SentAt:                  time.UnixMilli(1791028800000),
		Text:                    "synthetic",
		ProtocolClientMessageID: "456",
		SeenMetadata: &domain.SeenMetadata{
			ClientMessageID: "456",
			RecipientID:     "owner",
			MessageType:     "webchat",
			Timestamp:       "1791028800000",
		},
	}
	if err = store.Put(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	port := &membershipPort{
		store:     store,
		actions:   cancellingActionProbeSource{started: started},
		readChats: map[domain.ConversationRef]bool{ref: true},
	}
	if parent == "service" {
		port.lifecycle = ctx
	} else {
		port.actionContext = ctx
	}
	body := `{"method":"cli_message_action_probe","arguments":{"request_id":"a0000000-0000-4000-8000-000000000001","conversation_type":"direct","conversation_id":"peer","message_ids":["123"],"plan":"read"}}`
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		port.cliHandler().
			ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(body)))
		close(done)
	}()
	// Act: cancel the service/session while the upstream call is in flight.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("probe never reached synthetic source")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("probe ignored lifecycle cancellation")
	}
	// Assert: no success is invented, and cancellation does not repeat the write.
	var op domain.MessageActionOperation
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &op) != nil || op.Status != "unknown" {
		t.Fatal("cancelled probe reported success or lost result")
	}
}

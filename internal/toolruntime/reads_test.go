package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/toolsy"
	adapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type observedReader struct {
	*adapter.Reader
	calls        atomic.Int64
	checks       atomic.Int64
	revision     atomic.Int64
	checkFailure atomic.Bool
	mu           sync.Mutex
	requests     []search.GroupRequest
}

func (r *observedReader) ReadIdentity(ctx context.Context) (shared.ReadIdentity, error) {
	v, err := r.Reader.ReadIdentity(ctx)
	v.PolicyRevision += fmt.Sprint(r.revision.Load())
	return v, err
}
func (r *observedReader) GroupPage(ctx context.Context, in search.GroupRequest) (*search.Page, error) {
	r.calls.Add(1)
	r.mu.Lock()
	r.requests = append(r.requests, in)
	r.mu.Unlock()
	return r.Reader.GroupPage(ctx, in)
}
func (r *observedReader) ConversationPage(ctx context.Context, in search.ConversationRequest) (*search.Page, error) {
	r.calls.Add(1)
	return r.Reader.ConversationPage(ctx, in)
}
func (r *observedReader) GroupContext(ctx context.Context, in conversations.ContextRequest) (conversations.GroupContext, error) {
	r.calls.Add(1)
	return r.Reader.GroupContext(ctx, in)
}
func (r *observedReader) ConversationContext(ctx context.Context, in conversations.ContextRequest) (conversations.ConversationContext, error) {
	r.calls.Add(1)
	return r.Reader.ConversationContext(ctx, in)
}
func (r *observedReader) CheckGroupSearch(ctx context.Context, in search.GroupRequest) error {
	r.checks.Add(1)
	if r.checkFailure.Load() {
		return errors.New("private storage failure")
	}
	return r.Reader.CheckGroupSearch(ctx, in)
}

func readRuntime(t *testing.T) (*Reads, *observedReader) {
	t.Helper()
	ctx := context.Background()
	owner, err := storage.Open(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	if err = owner.BindAccount(ctx, "synthetic-runtime-account"); err != nil {
		t.Fatal(err)
	}
	if err = owner.UpsertGroup(ctx, domain.Group{ID: "g", Name: "synthetic"}, nil); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for n := range 2 {
		if err = owner.Put(ctx, domain.Message{GroupID: "g", ID: fmt.Sprint(int64(9007199254740993) + int64(n)), SenderID: fmt.Sprint(n), Text: fmt.Sprintf("synthetic Cà phê 🍀 term%d", n), SentAt: at.Add(time.Duration(n) * time.Minute), Source: "live"}); err != nil {
			t.Fatal(err)
		}
	}
	reader := &observedReader{Reader: adapter.New(owner)}
	runtime, err := NewReads(&search.Handler{Reads: reader}, &conversations.Handler{Reads: reader}, reader)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, reader
}
func bound(t *testing.T, r *Reads) Binding {
	t.Helper()
	b, err := r.Bind(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func domainCode(err error) string {
	var e *domain.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestReadRuntimeRejectsUntrustedAndForeignCalls(t *testing.T) {
	// Arrange: genuine runtimes and a valid input; no model-derived capability.
	r, reader := readRuntime(t)
	foreign, _ := readRuntime(t)
	raw := []byte(`{"query":"synthetic","group_id":"g"}`)
	// Act.
	_, zeroErr := r.Execute(context.Background(), Binding{}, "zalo_search_messages", raw)
	_, foreignErr := r.Execute(context.Background(), bound(t, foreign), "zalo_search_messages", raw)
	viewErr := r.view.Execute(context.Background(), toolsy.ToolCall{ToolName: "zalo_search_messages", Input: toolsy.ToolInput{ArgsJSON: raw}, Env: toolsy.NewRunEnv(nil)}, func(toolsy.Chunk) error { return nil })
	// Assert: nothing reaches application/read ports, including direct view access.
	if domainCode(zeroErr) != "PERMISSION_DENIED" || domainCode(foreignErr) != "PERMISSION_DENIED" || viewErr == nil || reader.calls.Load() != 0 || reader.checks.Load() != 0 {
		t.Fatal(zeroErr, foreignErr, viewErr, reader.calls.Load())
	}
}

func TestReadRuntimeInvalidArgumentsNeverDispatch(t *testing.T) {
	cases := []string{`[]`, `{"query":"synthetic","limit":null}`, `{"query":"synthetic","since":"not-date"}`, `{"query":"synthetic","limit":"51"}`, `{"query":"synthetic","limit":0}`, `{"query":"synthetic","subject":{"principal":"owner"}}`, `{"query":"synthetic","group_id":9007199254740993}`}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			// Arrange.
			r, reader := readRuntime(t)
			binding := bound(t, r)
			// Act.
			_, err := r.Execute(context.Background(), binding, "zalo_search_messages", []byte(raw))
			// Assert.
			if domainCode(err) != "INVALID_ARGUMENT" || reader.calls.Load() != 0 || reader.checks.Load() != 0 {
				t.Fatal(err, reader.calls.Load(), reader.checks.Load())
			}
		})
	}
}

func TestReadRuntimeRevocationAndPolicyFailurePrecedeHandler(t *testing.T) {
	// Arrange.
	r, reader := readRuntime(t)
	binding := bound(t, r)
	reader.revision.Add(1)
	// Act.
	_, revoked := r.Execute(context.Background(), binding, "zalo_search_messages", []byte(`{"query":"synthetic"}`))
	// Assert.
	if domainCode(revoked) != "PERMISSION_DENIED" || reader.calls.Load() != 0 {
		t.Fatal(revoked)
	}
	// Arrange/Act: the same real policy denies direct/group collision and disabled group.
	binding = bound(t, r)
	_, groupErr := r.Execute(context.Background(), binding, "zalo_search_messages", []byte(`{"query":"synthetic","group_id":"forbidden"}`))
	_, directErr := r.Execute(context.Background(), binding, "zalo_search_conversation_messages", []byte(`{"query":"synthetic","conversation_type":"direct","conversation_id":"g"}`))
	// Assert: native domain envelopes, no selected-message handler.
	if domainCode(groupErr) != "INVALID_ARGUMENT" || domainCode(directErr) != "PERMISSION_DENIED" || reader.calls.Load() != 0 {
		t.Fatal(groupErr, directErr)
	}
	// Arrange/Act: technical policy error remains private storage error, not denial.
	reader.checkFailure.Store(true)
	_, technical := r.Execute(context.Background(), binding, "zalo_search_messages", []byte(`{"query":"synthetic"}`))
	// Assert.
	if technical == nil || domainCode(technical) != "" || reader.calls.Load() != 0 {
		t.Fatal(technical)
	}
}

func TestReadRuntimeEachHandlerExactlyOnceAndNormalization(t *testing.T) {
	calls := []struct{ name, raw string }{
		{"zalo_search_messages", `{"query":"Cà phê 🍀","group_id":"g","limit":"1"}`},
		{"zalo_search_conversation_messages", `{"query":"synthetic","conversation_type":"group","conversation_id":"g"}`},
		{"zalo_get_message_context", `{"group_id":"g","message_id":"9007199254740993","before":"0","after":0}`},
		{"zalo_get_conversation_message_context", `{"conversation_type":"group","conversation_id":"g","message_id":"9007199254740993","before":0,"after":0}`},
	}
	// Arrange.
	r, reader := readRuntime(t)
	for i, call := range calls {
		// Act.
		result, err := r.Execute(context.Background(), bound(t, r), call.name, []byte(call.raw))
		// Assert: one handler each; actual executable output schema is toolsy-validated.
		if err != nil || result == nil || reader.calls.Load() != int64(i+1) {
			t.Fatal(call.name, err, reader.calls.Load())
		}
		if result.GroupSearch != nil && (len(result.GroupSearch.Result.Messages) != 1 || result.GroupSearch.Result.Messages[0].ID != "9007199254740994") {
			t.Fatal(result.GroupSearch.Result)
		}
		if result.GroupContext != nil && (result.GroupContext.Anchor.ID != "9007199254740993" || len(result.GroupContext.Before) != 0 || len(result.GroupContext.After) != 0) {
			t.Fatal(result.GroupContext)
		}
	}
}

func TestReadRuntimeConcurrentInvocationIsolation(t *testing.T) {
	// Arrange: distinct calls through one immutable view; no admission within runtime.
	r, reader := readRuntime(t)
	var wg sync.WaitGroup
	// Act.
	for n := range 32 {
		wg.Go(func() {
			raw, _ := json.Marshal(map[string]any{"query": fmt.Sprintf("term%d", n%2), "group_id": "g", "limit": 1})
			binding, err := r.Bind(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			result, err := r.Execute(context.Background(), binding, "zalo_search_messages", raw)
			if err != nil {
				t.Error(err)
				return
			}
			hits := result.GroupSearch.Result.Messages
			if len(hits) != 1 || hits[0].SenderID != fmt.Sprint(n%2) {
				t.Errorf("call%d crossed arguments: %+v", n, hits)
			}
		})
	}
	wg.Wait()
	// Assert.
	if reader.calls.Load() != 32 || reader.checks.Load() != 32 || len(reader.requests) != 32 {
		t.Fatal(reader.calls.Load(), reader.checks.Load(), len(reader.requests))
	}
}

func TestReadRuntimeCancellationDoesNotDispatch(t *testing.T) {
	// Arrange.
	r, reader := readRuntime(t)
	binding := bound(t, r)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act.
	_, err := r.Execute(ctx, binding, "zalo_search_messages", []byte(`{"query":"synthetic"}`))
	// Assert: cancellation survives toolsy wrapping and no handler executes.
	if !errors.Is(err, context.Canceled) || reader.calls.Load() != 0 {
		t.Fatal(err, reader.calls.Load())
	}
}

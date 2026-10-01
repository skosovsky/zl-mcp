package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type fakeZalo struct {
	mu     sync.Mutex
	joined bool
	calls  int
	name   string
}

func (f *fakeZalo) AccountID() string { return "test" }
func (f *fakeZalo) Groups(context.Context) ([]domain.Group, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.joined {
		return []domain.Group{{ID: "g", Name: f.name}}, nil
	}
	return []domain.Group{}, nil
}
func (f *fakeZalo) Group(context.Context, string) (domain.Group, *string, error) {
	return domain.Group{ID: "g", Name: f.name}, nil, nil
}
func (f *fakeZalo) Inspect(context.Context, string) (domain.Invite, error) {
	return domain.Invite{Group: domain.Group{ID: "g", Name: f.name}}, nil
}
func (f *fakeZalo) Join(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.joined = true
	return nil
}
func manager(t *testing.T) (*JoinManager, *fakeZalo) {
	t.Helper()
	ctx := context.Background()
	s, e := storage.Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), []string{"g"}, 90)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	api := &fakeZalo{name: "Test group"}
	j := NewJoin(ctx, s, api, true)
	t.Cleanup(j.Wait)
	return j, api
}
func TestApprovalIdempotencyAndTargetBinding(t *testing.T) {
	// Arrange
	j, api := manager(t)
	ctx := context.Background()
	v, e := j.Inspect(ctx, "https://zalo.me/g/abc")
	if e != nil {
		t.Fatal(e)
	}
	previewID := *v["preview_id"].(*string)
	token, e := j.Approve(ctx, previewID)
	if e != nil {
		t.Fatal(e)
	}
	requestID := uuid.NewString()
	// Act
	result, e := j.Start(ctx, token, requestID)
	if e != nil {
		t.Fatal(e)
	}
	j.Wait()
	again, e := j.Start(ctx, token, requestID)
	if e != nil {
		t.Fatal(e)
	}
	// Assert
	if result["operation_id"] != again["operation_id"] || again["status"] != "joined" {
		t.Fatalf("unexpected operation: %+v", again)
	}
	if api.calls != 1 {
		t.Fatalf("expected one mutation, got %d", api.calls)
	}
	if _, e = j.Start(ctx, token, uuid.NewString()); e == nil {
		t.Fatal("consumed approval reused")
	}
	if _, e = j.Start(ctx, "different", requestID); e == nil {
		t.Fatal("request token substituted")
	}
}
func TestMissingApprovalAndChangedPreviewNeverJoin(t *testing.T) {
	// Arrange
	j, api := manager(t)
	ctx := context.Background()
	// Act
	_, e := j.Start(ctx, "invented", uuid.NewString())
	// Assert
	if e == nil || api.calls != 0 {
		t.Fatal("unapproved mutation")
	}
	// Arrange
	v, e := j.Inspect(ctx, "https://zalo.me/g/abc")
	if e != nil {
		t.Fatal(e)
	}
	token, e := j.Approve(ctx, *v["preview_id"].(*string))
	if e != nil {
		t.Fatal(e)
	}
	api.name = "Changed group"
	// Act
	_, e = j.Start(ctx, token, uuid.NewString())
	// Assert
	if e == nil || api.calls != 0 {
		t.Fatal("changed preview accepted")
	}
}
func TestInviteRejectsForeignAndAmbiguousURLs(t *testing.T) {
	// Arrange
	invalid := []string{"http://zalo.me/g/abc", "https://evil.test/g/abc", "https://zalo.me.evil.test/g/abc", "https://zalo.me/g/abc?x=y", "https://zalo.me:443/g/abc", "https://u@zalo.me/g/abc", "https://zalo.me/g/a/b", "https://zalo.me/g/%2f", "https://zalo.me/g/"}
	for _, link := range invalid {
		t.Run(link, func(t *testing.T) {
			// Act
			_, e := ValidateInvite(link)
			// Assert
			if e == nil {
				t.Fatal("unsafe invitation accepted")
			}
		})
	}
}

func TestExpiredApprovalAndCrashRecovery(t *testing.T) {
	// Arrange
	j, api := manager(t)
	ctx := context.Background()
	v, err := j.Inspect(ctx, "https://zalo.me/g/abc")
	if err != nil {
		t.Fatal(err)
	}
	token, err := j.Approve(ctx, *v["preview_id"].(*string))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = j.Store.DB.ExecContext(ctx, "UPDATE join_approvals SET expires_at='2000-01-01T00:00:00Z'"); err != nil {
		t.Fatal(err)
	}
	// Act
	_, err = j.Start(ctx, token, uuid.NewString())
	// Assert
	if err == nil || api.calls != 0 {
		t.Fatal("expired approval mutated membership")
	}
	// Arrange: persisted operation that lost its process before confirmation.
	requestID := uuid.NewString()
	operationID := uuid.NewString()
	payload := fmt.Sprintf(`{"request_id":%q,"operation_id":%q,"status":"running","group_id":"g","collection_enabled":true,"checked_at":"2026-10-01T00:00:00Z","retry_safe":false,"poll_after_ms":1000,"error":null}`, requestID, operationID)
	if _, err = j.Store.DB.ExecContext(ctx, "INSERT INTO join_requests VALUES(?,?,?,?,?)", requestID, operationID, "test-hash", payload, "2026-10-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	// Act
	err = j.Recover(ctx)
	result, readErr := j.Operation(ctx, operationID)
	// Assert
	if err != nil || readErr != nil || result["status"] != "unknown" || api.calls != 0 {
		t.Fatalf("unsafe recovery: %+v %v %v", result, err, readErr)
	}
}

func TestRecoveredOperationReconcilesWithoutRepeatingJoin(t *testing.T) {
	// Arrange
	j, api := manager(t)
	ctx := context.Background()
	requestID := uuid.NewString()
	operationID := uuid.NewString()
	payload := fmt.Sprintf(`{"request_id":%q,"operation_id":%q,"status":"running","group_id":"g","collection_enabled":true,"checked_at":"2026-10-01T00:00:00Z","retry_safe":false,"poll_after_ms":1000,"error":null}`, requestID, operationID)
	if _, err := j.Store.DB.ExecContext(ctx, "INSERT INTO join_requests VALUES(?,?,?,?,?)", requestID, operationID, "test-hash", payload, "2026-10-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := j.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	// Act: first refreshed catalog does not yet prove membership.
	if err := j.Reconcile(ctx, []domain.Group{}); err != nil {
		t.Fatal(err)
	}
	absent, err := j.Operation(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	if absent["status"] != "unknown" || api.calls != 0 {
		t.Fatalf("unsafe negative reconciliation: %+v", absent)
	}
	// Act: later catalog confirms membership, without another network mutation.
	if err = j.Reconcile(ctx, []domain.Group{{ID: "g", Name: "Test group"}}); err != nil {
		t.Fatal(err)
	}
	confirmed, err := j.Operation(ctx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	if confirmed["status"] != "joined" || confirmed["poll_after_ms"] != nil || api.calls != 0 {
		t.Fatalf("bad reconciliation: %+v calls=%d", confirmed, api.calls)
	}
}

func TestCanceledExecutionPersistsTerminalOutcome(t *testing.T) {
	// Arrange: cancellation while another request owns the upstream gate.
	j, api := manager(t)
	ctx, cancel := context.WithCancel(context.Background())
	j.ctx = ctx
	if err := j.enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer j.leave()
	requestID, operationID := uuid.NewString(), uuid.NewString()
	result := map[string]any{"request_id": requestID, "operation_id": operationID, "status": "running", "group_id": "g", "collection_enabled": true, "checked_at": "2026-10-01T00:00:00Z", "retry_safe": false, "poll_after_ms": 1000, "error": nil}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = j.Store.DB.Exec("INSERT INTO join_requests VALUES(?,?,?,?,?)", requestID, operationID, "test-hash", string(payload), "2026-10-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	cancel()
	// Act
	j.execute(preview{GroupID: "g", URL: "https://zalo.me/g/abc"}, result)
	saved, err := j.Operation(context.Background(), operationID)
	// Assert: no restart required to clear running, and no network mutation.
	if err != nil || saved["status"] != "unknown" || saved["poll_after_ms"] != nil || api.calls != 0 {
		t.Fatalf("interrupted outcome: %+v error=%v mutations=%d", saved, err, api.calls)
	}
}

func TestJoinRateLimitSurvivesManagerRestart(t *testing.T) {
	// Arrange: an accepted operation is persisted before replacing the manager.
	j, api := manager(t)
	ctx := context.Background()
	v, err := j.Inspect(ctx, "https://zalo.me/g/abc")
	if err != nil {
		t.Fatal(err)
	}
	token, err := j.Approve(ctx, *v["preview_id"].(*string))
	if err != nil {
		t.Fatal(err)
	}
	requestID := uuid.NewString()
	original, err := j.Start(ctx, token, requestID)
	if err != nil {
		t.Fatal(err)
	}
	j.Wait()
	restarted := NewJoin(ctx, j.Store, api, true)
	defer restarted.Wait()
	preview, err := restarted.Inspect(ctx, "https://zalo.me/g/abc")
	if err != nil {
		t.Fatal(err)
	}
	freshToken, err := restarted.Approve(ctx, *preview["preview_id"].(*string))
	if err != nil {
		t.Fatal(err)
	}
	// Act
	_, newErr := restarted.Start(ctx, freshToken, uuid.NewString())
	repeated, retryErr := restarted.Start(ctx, token, requestID)
	// Assert: a replay remains readable while a new operation is rate limited.
	var typed *domain.Error
	if !errors.As(newErr, &typed) || typed.Code != "RATE_LIMITED" || retryErr != nil || repeated["operation_id"] != original["operation_id"] || api.calls != 1 {
		t.Fatalf("rate=%v retry=%v operation=%+v mutations=%d", newErr, retryErr, repeated, api.calls)
	}
}

func TestAlreadyMemberConsumesApprovalWithoutMutation(t *testing.T) {
	// Arrange: a reviewed invite for a group already in the membership catalog.
	j, api := manager(t)
	api.joined = true
	ctx := context.Background()
	preview, err := j.Inspect(ctx, "https://zalo.me/g/abc")
	if err != nil {
		t.Fatal(err)
	}
	if preview["membership"] != "member" {
		t.Fatalf("membership not confirmed: %+v", preview)
	}
	token, err := j.Approve(ctx, *preview["preview_id"].(*string))
	if err != nil {
		t.Fatal(err)
	}
	requestID := uuid.NewString()

	// Act: complete and retry the original approved operation.
	started, err := j.Start(ctx, token, requestID)
	if err != nil {
		t.Fatal(err)
	}
	j.Wait()
	result, err := j.Operation(ctx, started["operation_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	retry, err := j.Start(ctx, token, requestID)
	if err != nil {
		t.Fatal(err)
	}
	_, reuseErr := j.Start(ctx, token, uuid.NewString())

	// Assert: membership confirmation never sends Join or grants a reusable approval.
	if result["status"] != "already_member" || retry["status"] != "already_member" || retry["operation_id"] != result["operation_id"] {
		t.Fatalf("incorrect member operation: result=%+v retry=%+v", result, retry)
	}
	if api.calls != 0 || reuseErr == nil {
		t.Fatalf("unexpected mutation or approval reuse: calls=%d error=%v", api.calls, reuseErr)
	}
	if result["collection_enabled"] != true || result["retry_safe"] != false || result["poll_after_ms"] != nil {
		t.Fatalf("incorrect terminal metadata: %+v", result)
	}
}

type moderatedZalo struct{ *fakeZalo }

func (f *moderatedZalo) Inspect(context.Context, string) (domain.Invite, error) {
	return domain.Invite{Group: domain.Group{ID: "g", Name: f.name}, ApprovalRequired: true}, nil
}
func (f *moderatedZalo) Join(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	// Upstream acknowledges the request without proving pending membership.
	return nil
}

func TestModeratedInviteDoesNotInventPendingApprovalOrRepeatJoin(t *testing.T) {
	// Arrange: approval-required invite, successful opaque Join, no confirmed membership.
	j, api := manager(t)
	j.API = &moderatedZalo{api}
	ctx := context.Background()
	preview, err := j.Inspect(ctx, "https://zalo.me/g/abc")
	if err != nil {
		t.Fatal(err)
	}
	token, err := j.Approve(ctx, *preview["preview_id"].(*string))
	if err != nil {
		t.Fatal(err)
	}
	requestID := uuid.NewString()
	// Act
	started, err := j.Start(ctx, token, requestID)
	if err != nil {
		t.Fatal(err)
	}
	j.Wait()
	result, err := j.Operation(ctx, started["operation_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := j.Start(ctx, token, requestID)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: joinAppr and a successful response do not prove a recorded approval request.
	if result["status"] != "unknown" || result["retry_safe"] != false || result["poll_after_ms"] != nil || repeated["status"] != "unknown" || repeated["operation_id"] != result["operation_id"] || api.calls != 1 {
		t.Fatalf("unsafe moderated outcome: result=%+v retry=%+v calls=%d", result, repeated, api.calls)
	}
}

type pendingApprovalZalo struct{ *moderatedZalo }

func (f *pendingApprovalZalo) Join(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return fmt.Errorf("wrapped: %w", domain.ErrJoinPendingApproval)
}

func TestExplicitPendingApprovalPersistsReconcilesAndNeverRepeatsJoin(t *testing.T) {
	// Arrange: typed pending signal, successful catalog without membership.
	j, api := manager(t)
	j.API = &pendingApprovalZalo{&moderatedZalo{api}}
	ctx := context.Background()
	preview, err := j.Inspect(ctx, "https://zalo.me/g/abc")
	if err != nil {
		t.Fatal(err)
	}
	token, err := j.Approve(ctx, *preview["preview_id"].(*string))
	if err != nil {
		t.Fatal(err)
	}
	requestID := uuid.NewString()
	// Act: submit, recover into a new manager, repeat UUID and refresh absent membership.
	started, err := j.Start(ctx, token, requestID)
	if err != nil {
		t.Fatal(err)
	}
	j.Wait()
	restored := NewJoin(ctx, j.Store, j.API, true)
	if err = restored.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err = restored.Reconcile(ctx, nil); err != nil {
		t.Fatal(err)
	}
	repeated, err := restored.Start(ctx, token, requestID)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: pending survives restart and absence, without another Join.
	if repeated["status"] != "pending_approval" || repeated["error"] != nil || repeated["retry_safe"] != false || repeated["poll_after_ms"] != nil || repeated["operation_id"] != started["operation_id"] || api.calls != 1 {
		t.Fatalf("bad pending state: %+v calls=%d", repeated, api.calls)
	}
	// Act: eventual membership reconciles the same operation.
	if err = restored.Reconcile(ctx, []domain.Group{{ID: "g", Name: "Test group"}}); err != nil {
		t.Fatal(err)
	}
	result, err := restored.Operation(ctx, started["operation_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	if result["status"] != "joined" || result["error"] != nil || result["operation_id"] != started["operation_id"] || api.calls != 1 {
		t.Fatalf("bad reconciliation: %+v calls=%d", result, api.calls)
	}
}

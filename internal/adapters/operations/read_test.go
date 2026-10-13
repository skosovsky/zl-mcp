package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/app/history"
	"github.com/skosovsky/zl-mcp/internal/app/membership"
	"github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type observedBackend struct {
	method   string
	args     any
	calls    int
	deadline time.Time
	value    map[string]any
	err      error
}

func (b *observedBackend) Call(ctx context.Context, name string, args any) (map[string]any, error) {
	b.method, b.args = name, args
	b.calls++
	b.deadline, _ = ctx.Deadline()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b.value, b.err
}

func canonical(t *testing.T, value any) any {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var result any
	if err = d.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestReadCapabilitiesPreserveNativeProjectionAndCannotDispatchCommands(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	at := "2026-10-01T00:00:00Z"
	messageID := "9007199254740993"
	nativeHistory := domain.HistoryImportStatus{
		ConversationType:   "group",
		ConversationID:     messageID,
		RequestID:          id,
		Since:              at,
		Until:              "2026-10-02T00:00:00Z",
		PageSize:           50,
		MaxPages:           20,
		MaxMessages:        1000,
		OperationID:        id,
		NotificationPolicy: "none",
		SourceKind:         "group_cloud",
		State:              "paused",
		CreatedAt:          at,
		UpdatedAt:          at,
	}
	for _, test := range []struct {
		name     string
		key, arg string
		seconds  int
		value    any
		call     func(*Reader) (any, error)
	}{
		{"zalo_get_send_status", "request_id", id, 35, domain.SendOperation{RequestID: id, RecipientID: messageID, Status: "unknown", UpdatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}, func(r *Reader) (any, error) { return (&messaging.Handler{Reads: r}).Status(context.Background(), id) }},
		{"zalo_get_history_import_status", "operation_id", id, 35, nativeHistory, func(r *Reader) (any, error) { return (&history.Handler{Reads: r}).Status(context.Background(), id) }},
		{"zalo_get_join_status", "operation_id", id, 6, membership.JoinStatus{Status: "unknown", OperationID: id, GroupID: &messageID, RequestID: id, CheckedAt: at}, func(r *Reader) (any, error) { return (&membership.Handler{Reads: r}).Status(context.Background(), id) }},
		{"zalo_inspect_invite", "invite_url", "https://zalo.me/g/synthetic", 6, membership.Invite{Name: "Cà phê 🍀", Membership: "unknown"}, func(r *Reader) (any, error) {
			return (&membership.Handler{Reads: r}).Inspect(context.Background(), "https://zalo.me/g/synthetic")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: the observed native backend cannot silently start replacement work.
			value := canonical(t, test.value).(map[string]any)
			backend := &observedBackend{value: value}
			reader := NewReader(backend)
			started := time.Now()
			// Act.
			result, err := test.call(reader)
			// Assert: exact input/method, one native read, original deadline, nulls/IDs/Unicode.
			if err != nil {
				t.Fatal(err)
			}
			if backend.calls != 1 || backend.method != test.name ||
				!reflect.DeepEqual(backend.args, map[string]any{test.key: test.arg}) {
				t.Fatalf("unexpected dispatch: %+v", backend)
			}
			if backend.deadline.Before(started.Add(time.Duration(test.seconds)*time.Second)) ||
				backend.deadline.After(time.Now().Add(time.Duration(test.seconds)*time.Second)) {
				t.Fatalf("changed read deadline: %s", backend.deadline)
			}
			if !reflect.DeepEqual(canonical(t, result), value) {
				t.Fatalf("projection changed: got %#v want %#v", canonical(t, result), value)
			}
			schema, err := contracts.Compile(test.name, "output")
			if err != nil {
				t.Fatal(err)
			}
			// The validator consumes its normal JSON representation; comparison above uses UseNumber.
			wire, _ := json.Marshal(result)
			var validated any
			if err = json.Unmarshal(wire, &validated); err != nil {
				t.Fatal(err)
			}
			if err = schema.Validate(validated); err != nil {
				t.Fatal(err)
			}
			backend.err = &domain.Error{Code: "PERMISSION_DENIED", Message: "Synthetic revoked operation."}
			_, err = test.call(reader)
			if !errors.Is(err, backend.err) || backend.calls != 2 || backend.method != test.name {
				t.Fatalf("status failure dispatched replacement work: %v %+v", err, backend)
			}
		})
	}
}

func TestReadCapabilitiesKeepCancellationAndPrivateCauses(t *testing.T) {
	// Arrange.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	backend := &observedBackend{}
	handler := &messaging.Handler{Reads: NewReader(backend)}
	// Act.
	_, err := handler.Status(ctx, "opaque")
	// Assert: cancellation reaches the borrowed owner; no send/retry fallback.
	if !errors.Is(err, context.Canceled) || backend.calls != 1 || backend.method != "zalo_get_send_status" {
		t.Fatalf("cancellation lost: %v %+v", err, backend)
	}
	private := errors.New("private owner cause")
	backend.err = private
	_, err = handler.Status(context.Background(), "opaque")
	if !errors.Is(err, private) || backend.calls != 2 {
		t.Fatalf("cause replaced: %v", err)
	}
}

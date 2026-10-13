package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	adapter "github.com/skosovsky/zl-mcp/internal/adapters/operations"
	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
	"github.com/skosovsky/zl-mcp/internal/app/history"
	"github.com/skosovsky/zl-mcp/internal/app/membership"
	"github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type commandRecorder struct {
	calls           int
	name, principal string
	args            []byte
	remaining       time.Duration
	err             error
}

func (c *commandRecorder) Call(ctx context.Context, name string, args any) (map[string]any, error) {
	c.calls++
	c.name = name
	c.args, _ = json.Marshal(args)
	if deadline, ok := ctx.Deadline(); ok {
		c.remaining = time.Until(deadline)
	}
	return nil, c.err
}

func (c *commandRecorder) Acknowledge(
	_ context.Context,
	principal, id, receipt string,
) (eventaccess.Acknowledgement, error) {
	c.calls++
	c.name = "zalo_ack_subscription_events"
	c.principal = principal
	c.args, _ = json.Marshal(struct {
		ID      string `json:"ID"`
		Receipt string `json:"Receipt"`
	}{id, receipt})
	return eventaccess.Acknowledgement{Acknowledged: true}, nil
}

//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestCommandsRejectUntrustedScopeBeforeEffectsAndNeverRetryOwnerFailure(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	for _, test := range []struct {
		name, raw string
		seconds   int
	}{
		{"zalo_send_direct_message", `{"recipient_id":"9007199254740993","text":"Cà phê 🍀","request_id":"` + id + `"}`, 35},
		{"zalo_join_group", `{"plan_token":"opaque-plan","request_id":"` + id + `"}`, 6},
		{"zalo_import_conversation_history", `{"conversation_type":"group","conversation_id":"g","request_id":"` + id + `","since":"2026-10-01T00:00:00Z","until":"2026-10-02T00:00:00Z"}`, 35},
		{"zalo_cancel_history_import", `{"operation_id":"` + id + `"}`, 35},
		{"zalo_ack_subscription_events", `{"subscription_id":"s","receipt":"opaque-receipt"}`, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: one native dispatch recorder, current real authority and domain failure.
			base, reader := readRuntime(t)
			authority := &catalogAuthority{observedReader: reader}
			failure := &domain.Error{Code: "PERMISSION_DENIED", Message: "Native command authority revoked."}
			recorder := &commandRecorder{err: failure}
			operations := adapter.NewReader(recorder)
			r, err := New(base.search, base.conversations, authority, Capabilities{
				Messaging: &messaging.Handler{
					Reads:    operations,
					Commands: operations,
				},
				History:    &history.Handler{Reads: operations, Commands: operations},
				Membership: &membership.Handler{Reads: operations, Commands: operations},
				Events:     &eventaccess.Handler{Journal: reader, Commands: recorder},
			})
			if err != nil {
				t.Fatal(err)
			}
			binding := bound(t, r)
			// Act: caller-controlled, foreign, malformed and stale scopes.
			_, zero := r.Execute(context.Background(), Binding{}, test.name, []byte(test.raw))
			_, foreign := r.Execute(context.Background(), bound(t, base), test.name, []byte(test.raw))
			_, invalid := r.Execute(context.Background(), binding, test.name, []byte(`{}`))
			authority.shifted.Store(true)
			_, stale := r.Execute(context.Background(), binding, test.name, []byte(test.raw))
			authority.shifted.Store(false)
			// Assert: no command/ACK reached the native owner before all gates.
			if domainCode(zero) != "PERMISSION_DENIED" || domainCode(foreign) != "PERMISSION_DENIED" ||
				domainCode(invalid) != "INVALID_ARGUMENT" ||
				domainCode(stale) != "PERMISSION_DENIED" ||
				recorder.calls != 0 {
				t.Fatal(zero, foreign, invalid, stale, recorder.calls)
			}
			// Act: one authorized invocation; the native owner remains the final authority.
			result, err := r.Execute(context.Background(), binding, test.name, []byte(test.raw))
			// Assert: exactly one dispatch, no retry/fallback, original error and deadline.
			if recorder.calls != 1 || recorder.name != test.name {
				t.Fatal(recorder.calls, recorder.name)
			}
			if test.seconds > 0 {
				if !errors.Is(err, failure) || recorder.remaining <= time.Duration(test.seconds-1)*time.Second ||
					recorder.remaining > time.Duration(test.seconds)*time.Second {
					t.Fatal(err, recorder.remaining)
				}
			} else if err != nil || result.Acknowledgement == nil || !result.Acknowledgement.Acknowledged || recorder.principal == "" {
				t.Fatal(result, err, recorder.principal)
			}
			if test.name == "zalo_import_conversation_history" {
				var args map[string]any
				if err = json.Unmarshal(recorder.args, &args); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"page_size", "max_pages", "max_messages", "source"} {
					if _, present := args[key]; present {
						t.Fatalf("missing %s became an explicit default/zero: %s", key, recorder.args)
					}
				}
			}
		})
	}
}

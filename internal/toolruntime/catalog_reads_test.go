package toolruntime

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
)

type catalogAuthority struct {
	*observedReader

	shifted     atomic.Bool
	statusCalls atomic.Int32
}

func (a *catalogAuthority) ReadIdentity(ctx context.Context) (shared.ReadIdentity, error) {
	identity, err := a.observedReader.ReadIdentity(ctx)
	if a.shifted.Load() {
		identity.Account = "synthetic-switched-account"
	}
	return identity, err
}
func (a *catalogAuthority) CollectorStatus(ctx context.Context) (conversations.CollectorStatus, error) {
	a.statusCalls.Add(1)
	return a.observedReader.CollectorStatus(ctx)
}

func TestCollectorStatusKeepsNullCompatibilityAndRejectsStaleAccountBeforeRead(t *testing.T) {
	// Arrange: the trusted host account can change while a startup call is queued.
	base, reader := readRuntime(t)
	authority := &catalogAuthority{observedReader: reader}
	r, err := New(base.search, &conversations.Handler{Reads: reader, Catalog: authority}, authority)
	if err != nil {
		t.Fatal(err)
	}
	binding := bound(t, r)
	// Act: original null/missing-object forms all run as empty arguments.
	for _, raw := range []string{"null", "", "{}"} {
		result, executeErr := r.Execute(context.Background(), binding, "zalo_get_status", []byte(raw))
		if executeErr != nil || result.CollectorStatus == nil {
			t.Fatalf("legacy empty arguments changed: %q %v", raw, executeErr)
		}
	}
	// Assert: exactly one native read per accepted call.
	if authority.statusCalls.Load() != 3 {
		t.Fatal("collector read repeated")
	}
	// Act: an already-bound call cannot observe the newly bound account.
	authority.shifted.Store(true)
	_, err = r.Execute(context.Background(), binding, "zalo_get_status", []byte("{}"))
	// Assert: a stale scope remains denied; a fresh host binding restores the read.
	if domainCode(err) != "PERMISSION_DENIED" || authority.statusCalls.Load() != 3 {
		t.Fatal("stale account reached collector read")
	}
	_, err = r.Execute(context.Background(), bound(t, r), "zalo_get_status", []byte("{}"))
	if err != nil || authority.statusCalls.Load() != 4 {
		t.Fatalf("fresh startup scope failed: %v", err)
	}
}

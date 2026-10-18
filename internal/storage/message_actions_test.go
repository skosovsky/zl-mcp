package storage

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

const actionTestID = "a0000000-0000-4000-8000-000000000001"
const otherActionTestID = "a0000000-0000-4000-8000-000000000002"

func prepareAction(t *testing.T, s *Store, id, kind string) {
	t.Helper()
	plan := "clear_add"
	if kind == "read" {
		plan = "read"
	}
	if _, err := s.PrepareMessageAction(
		t.Context(),
		id,
		kind,
		plan,
		domain.ConversationRef{Type: "direct", ID: "peer"},
		strings.Repeat("a", 64),
	); err != nil {
		t.Fatal(err)
	}
}

func TestMessageActionIdentityAndConversationClaim(t *testing.T) {
	// Arrange: separate requests, shared typed conversation.
	s := seenTargetStore(t)
	prepareAction(t, s, actionTestID, "reaction")
	prepareAction(t, s, otherActionTestID, "read")
	// Act: race multiple callers claiming the same request.
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			won, err := s.ClaimMessageAction(t.Context(), actionTestID)
			if err != nil {
				t.Error(err)
			}
			if won {
				wins.Add(1)
			}
		})
	}
	wg.Wait()
	// Assert: one dispatch; another action in the same chat cannot interleave.
	if wins.Load() != 1 {
		t.Fatal("multiple dispatchers")
	}
	if won, err := s.ClaimMessageAction(t.Context(), otherActionTestID); err != nil || won {
		t.Fatal("conversation writes interleaved", err)
	}
	assertMessageActionConflicts(t, s)
	// Act: pre-write stage checkpoint survives restart as unknown, not pending.
	if err := s.BeginMessageActionStep(t.Context(), actionTestID, "clear", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverInterruptedMessageActions(t.Context()); err != nil {
		t.Fatal(err)
	}
	op, err := s.MessageActionStatus(t.Context(), strings.ToUpper(actionTestID))
	if err != nil || op.Status != "unknown" || op.Stage != "clear" || op.CompletedSteps != 0 || op.RetrySafe ||
		op.Reason == nil ||
		*op.Reason != "interrupted" {
		t.Fatal("restart hid uncertainty", err)
	}
	if won, err := s.ClaimMessageAction(t.Context(), actionTestID); err != nil || won {
		t.Fatal("unknown write was retried", err)
	}
	if won, err := s.ClaimMessageAction(t.Context(), otherActionTestID); err != nil || !won {
		t.Fatal("terminal intent kept conversation locked", err)
	}
}

func TestMessageActionStepsNeverReplayOrInventAcceptance(t *testing.T) {
	// Arrange.
	s := seenTargetStore(t)
	prepareAction(t, s, actionTestID, "reaction")
	if won, err := s.ClaimMessageAction(t.Context(), actionTestID); err != nil || !won {
		t.Fatal(err)
	}
	// Act / Assert: acceptance requires a confirmed step; one step cannot repeat.
	if err := s.CompleteMessageAction(t.Context(), actionTestID, "accepted", ""); !errors.Is(err, ErrActionState) {
		t.Fatal("unattempted intent accepted")
	}
	if err := s.BeginMessageActionStep(t.Context(), actionTestID, "clear", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginMessageActionStep(t.Context(), actionTestID, "clear", 0); !errors.Is(err, ErrActionState) {
		t.Fatal("step redispatched")
	}
	if err := s.ConfirmMessageActionStep(t.Context(), actionTestID, "clear", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginMessageActionStep(t.Context(), actionTestID, "clear", 1); !errors.Is(err, ErrActionState) {
		t.Fatal("clear repeated")
	}
	if err := s.CompleteMessageAction(t.Context(), actionTestID, "accepted", ""); !errors.Is(err, ErrActionState) {
		t.Fatal("clear-only replacement reported accepted")
	}
	if err := s.BeginMessageActionStep(t.Context(), actionTestID, "add", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteMessageAction(t.Context(), actionTestID, "accepted", ""); !errors.Is(err, ErrActionState) {
		t.Fatal("unconfirmed second step accepted")
	}
	if err := s.ConfirmMessageActionStep(t.Context(), actionTestID, "add", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginMessageActionStep(t.Context(), actionTestID, "add", 2); !errors.Is(err, ErrActionState) {
		t.Fatal("add repeated")
	}
	if err := s.CompleteMessageAction(t.Context(), actionTestID, "accepted", ""); err != nil {
		t.Fatal(err)
	}
	op, err := s.MessageActionStatus(t.Context(), actionTestID)
	if err != nil || op.Status != "accepted" || op.CompletedSteps != 2 || op.Stage != "none" || op.Reason != nil {
		t.Fatal("confirmed result lost", err)
	}
	if err = s.RecoverInterruptedMessageActions(t.Context()); err != nil {
		t.Fatal(err)
	}
	op, err = s.MessageActionStatus(t.Context(), actionTestID)
	if err != nil || op.Status != "accepted" {
		t.Fatal("recovery changed terminal result")
	}
}

func TestMessageActionPartialAndUnknownKeepCompletedSteps(t *testing.T) {
	for _, state := range []string{"partial", "unknown"} {
		// Arrange: first replacement step was accepted; second might have failed.
		s := seenTargetStore(t)
		prepareAction(t, s, actionTestID, "reaction")
		if _, err := s.ClaimMessageAction(t.Context(), actionTestID); err != nil {
			t.Fatal(err)
		}
		if err := s.BeginMessageActionStep(t.Context(), actionTestID, "clear", 0); err != nil {
			t.Fatal(err)
		}
		if err := s.ConfirmMessageActionStep(t.Context(), actionTestID, "clear", 0); err != nil {
			t.Fatal(err)
		}
		if err := s.BeginMessageActionStep(t.Context(), actionTestID, "add", 1); err != nil {
			t.Fatal(err)
		}
		reason := "upstream_rejected"
		if state == "unknown" {
			reason = "upstream_ambiguous"
		}
		// Act.
		err := s.CompleteMessageAction(t.Context(), actionTestID, state, reason)
		// Assert.
		op, readErr := s.MessageActionStatus(t.Context(), actionTestID)
		if err != nil || readErr != nil || op.Status != state || op.CompletedSteps != 1 || op.Stage != "add" {
			t.Fatal("partial progress hidden", err, readErr)
		}
		if won, err := s.ClaimMessageAction(t.Context(), actionTestID); err != nil || won {
			t.Fatal("terminal action resumed", err)
		}
	}
}

func TestMessageActionMigrationRollbackAndExplicitRecovery(t *testing.T) {
	// Arrange: abort migration after its DDL but before the version marker.
	s := seenTargetStore(t)
	if _, err := s.DB.Exec(`DROP TABLE message_action_operations;DELETE FROM schema_migrations WHERE version=19;
 CREATE TRIGGER reject_action_migration BEFORE INSERT ON schema_migrations WHEN NEW.version=19 BEGIN SELECT RAISE(ABORT,'synthetic');END;`); err != nil {
		t.Fatal(err)
	}
	// Act / Assert: no half ledger survives.
	if err := s.migrateMessageActions(t.Context()); err == nil {
		t.Fatal("marker failure ignored")
	}
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='message_action_operations'`).
		Scan(&n); err != nil ||
		n != 0 {
		t.Fatal("half ledger survived")
	}
	if _, err := s.DB.Exec(`DROP TRIGGER reject_action_migration`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateMessageActions(t.Context()); err != nil {
		t.Fatal(err)
	}
	prepareAction(t, s, actionTestID, "read")
	if _, err := s.ClaimMessageAction(t.Context(), actionTestID); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginMessageActionStep(t.Context(), actionTestID, "read", 0); err != nil {
		t.Fatal(err)
	}
	// Restart recovery is deliberately explicit, never a side effect of status reads.
	op, err := s.MessageActionStatus(t.Context(), actionTestID)
	if err != nil || op.Status != "dispatching" {
		t.Fatal("read interrupted active dispatcher")
	}
	if err = s.RecoverInterruptedMessageActions(t.Context()); err != nil {
		t.Fatal(err)
	}
	op, err = s.MessageActionStatus(t.Context(), actionTestID)
	if err != nil || op.Status != "unknown" || op.Stage != "read" {
		t.Fatal("interrupted receipt can replay")
	}
}

func TestMessageActionSurvivesDatabaseReopen(t *testing.T) {
	// Arrange: durable on-disk intent and a confirmed first replacement step.
	path := filepath.Join(t.TempDir(), "actions.sqlite")
	s, err := OpenWithPolicy(t.Context(), path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.BindAccount(t.Context(), "owner"); err != nil {
		t.Fatal(err)
	}
	prepareAction(t, s, actionTestID, "reaction")
	if _, err = s.ClaimMessageAction(t.Context(), actionTestID); err != nil {
		t.Fatal(err)
	}
	if err = s.BeginMessageActionStep(t.Context(), actionTestID, "clear", 0); err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmMessageActionStep(t.Context(), actionTestID, "clear", 0); err != nil {
		t.Fatal(err)
	}
	if err = s.BeginMessageActionStep(t.Context(), actionTestID, "add", 1); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	// Act: recreate Store, recover under startup ownership, then replay the UUID.
	reopened, err := OpenWithPolicy(t.Context(), path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = reopened.RecoverInterruptedMessageActions(t.Context()); err != nil {
		t.Fatal(err)
	}
	op, err := reopened.PrepareMessageAction(
		t.Context(),
		actionTestID,
		"reaction",
		"clear_add",
		domain.ConversationRef{Type: "direct", ID: "peer"},
		strings.Repeat("a", 64),
	)
	// Assert: step/counter/fingerprint survive; no path to re-dispatch the add.
	if err != nil || op.Status != "unknown" || op.Stage != "add" || op.CompletedSteps != 1 {
		t.Fatal("restart lost uncertainty", err)
	}
	if won, err := reopened.ClaimMessageAction(t.Context(), actionTestID); err != nil || won {
		t.Fatal("restart re-dispatched add", err)
	}
}

func assertMessageActionConflicts(t *testing.T, s *Store) {
	t.Helper()
	for _, tc := range []struct{ kind, typ, id, hash string }{
		{"read", "direct", "peer", strings.Repeat("a", 64)},
		{"reaction", "group", "peer", strings.Repeat("a", 64)},
		{"reaction", "direct", "other", strings.Repeat("a", 64)},
		{"reaction", "direct", "peer", strings.Repeat("b", 64)},
	} {
		plan := "clear_add"
		if tc.kind == "read" {
			plan = "read"
		}
		_, err := s.PrepareMessageAction(
			t.Context(),
			actionTestID,
			tc.kind,
			plan,
			domain.ConversationRef{Type: tc.typ, ID: tc.id},
			tc.hash,
		)
		if !errors.Is(err, ErrActionConflict) {
			t.Fatal("UUID accepted changed arguments", err)
		}
	}
}

func TestPublicMessageActionStatusRequiresBoundAccountAndCurrentPolicy(t *testing.T) {
	// Arrange: an existing durable operation independent of any active source.
	store := seenTargetStore(t)
	prepareAction(t, store, actionTestID, "read")
	if _, err := store.AccessibleMessageActionStatus(t.Context(), actionTestID); err != nil {
		t.Fatal(err)
	}
	// Act: collection policy revokes precisely that typed conversation.
	store.policy = domain.CollectionPolicy{}
	_, hidden := store.AccessibleMessageActionStatus(t.Context(), actionTestID)
	// Assert: public lookup hides it; internal ledger remains intact for completion.
	if !errors.Is(hidden, sql.ErrNoRows) {
		t.Fatal("out-of-policy action visible", hidden)
	}
	if _, err := store.MessageActionStatus(t.Context(), actionTestID); err != nil {
		t.Fatal("ledger was deleted", err)
	}
	// Act: restoring policy alone cannot replace bound account identity.
	store.policy = domain.CollectionPolicy{All: true}
	if _, err := store.DB.Exec("DELETE FROM metadata WHERE key='account'"); err != nil {
		t.Fatal(err)
	}
	_, unbound := store.AccessibleMessageActionStatus(t.Context(), actionTestID)
	// Assert.
	if !errors.Is(unbound, sql.ErrNoRows) {
		t.Fatal("unbound account could read operation", unbound)
	}
}

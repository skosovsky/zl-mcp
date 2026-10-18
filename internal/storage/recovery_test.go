package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func recoveryFixture(t *testing.T) (*Store, string, string, time.Time) {
	t.Helper()
	s, path, generation, at := journalFixture(t)
	revision, err := s.SubscriptionRevision(context.Background(), "recovery")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := s.ActivateRecoverySubscription(
		context.Background(),
		RecoverySubscription{
			ID:                 "recovery",
			Principal:          "owner",
			BusinessID:         "sub",
			BusinessGeneration: generation,
			Callback:           "https://control.example",
			Secret:             "synthetic-control",
		},
		revision,
		at,
	)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Generation == generation || sub.Generation == "" {
		t.Fatal("control generation must be independent")
	}
	return s, path, generation, at
}

func recoverySweep(t *testing.T, s *Store, at time.Time) int {
	t.Helper()
	n, err := s.SweepRecovery(context.Background(), at, DefaultDeliveryPolicy())
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func recoveryClaim(t *testing.T, s *Store, at time.Time) *RecoveryDelivery {
	t.Helper()
	d, err := s.ClaimRecoveryDelivery(context.Background(), at, DefaultDeliveryPolicy())
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func terminalBusiness(t *testing.T, s *Store, at time.Time, state string) {
	t.Helper()
	d, err := s.ClaimDelivery(context.Background(), at, DefaultDeliveryPolicy())
	if err != nil || d == nil {
		t.Fatalf("business claim: %v %v", d, err)
	}
	if err = s.FinishDelivery(
		context.Background(),
		*d,
		DeliveryOutcome{State: state, Reason: "http_2xx"},
		at,
	); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryAutonomousTerminalRestartReminderAndAck(t *testing.T) {
	// Arrange: no agent journal read occurs before the business callback commits.
	s, path, _, at := recoveryFixture(t)
	ctx := context.Background()
	defer func() { _ = s.Close() }()
	if got := recoverySweep(t, s, at); got != 0 {
		t.Fatalf("pendinghead queued=%d", got)
	}
	if d := recoveryClaim(t, s, at); d != nil {
		t.Fatal("control callback before business terminal state")
	}
	terminalBusiness(t, s, at, stateDeliveredText)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	// Act: startup sweep reconstructs the durable armed watch and queues one round.
	if got := recoverySweep(t, s, at); got != 1 {
		t.Fatalf("startup queued=%d", got)
	}
	first := recoveryClaim(t, s, at)
	if first == nil {
		t.Fatal("missingreadycallback")
	}
	allowed, err := s.recoveryDispatchAllowedAt(ctx, *first, at)
	if err != nil || !allowed {
		t.Fatalf("dispatch=%v %v", allowed, err)
	}
	if err = s.FinishRecoveryDelivery(
		ctx,
		*first,
		DeliveryOutcome{State: stateDeliveredText, Reason: "http_2xx"},
		at,
	); err != nil {
		t.Fatal(err)
	}
	if d := recoveryClaim(t, s, at.Add(29*time.Second)); d != nil {
		t.Fatal("reminder before minimum interval")
	}
	second := recoveryClaim(t, s, at.Add(30*time.Second))
	if second == nil || second.EventID == first.EventID || second.EpisodeID != first.EpisodeID {
		t.Fatal("new round requires freshID sameepisode")
	}
	assertRecoveryPrivacy(t, first.Payload)
	page, err := s.ReadSubscriptionEvents(ctx, "owner", "sub", 1)
	if err != nil {
		t.Fatal(err)
	}
	receipt := page["events"].([]map[string]any)[0]["receipt"].(string)
	if _, err = s.AckSubscriptionEvents(ctx, "owner", "sub", receipt); err != nil {
		t.Fatal(err)
	}
	allowed, err = s.recoveryDispatchAllowedAt(ctx, *second, at.Add(30*time.Second))
	if err != nil || allowed {
		t.Fatalf("ackedhead dispatch=%v %v", allowed, err)
	}
	if err = s.FinishRecoveryDelivery(
		ctx,
		*second,
		DeliveryOutcome{State: stateDeliveredText},
		at.Add(30*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	recoverySweep(t, s, at.Add(31*time.Second))
	var state string
	if err = s.DB.QueryRow(`SELECT state FROM recovery_deliveries WHERE id=?`, second.ID).
		Scan(&state); err != nil ||
		state != stateCancelledText {
		t.Fatalf("cancelled=%s %v", state, err)
	}
}

func assertRecoveryPrivacy(t *testing.T, payload []byte) {
	t.Helper()
	// Assert: data are routing references only; native content is not copied.
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatal(err)
	}
	if body[fieldRecoveryName] != recoveryProfile || body["cursor"] != nil {
		t.Fatalf("envelope=%v", body)
	}
	data := body["data"].(map[string]any)
	if len(data) != 3 || data[fieldRecoverySubscriptionID] != "sub" {
		t.Fatalf("data=%v", data)
	}
	for _, private := range []string{"synthetic-control", "https://", "one", "peer", "message_id"} {
		if strings.Contains(string(payload), private) {
			t.Fatalf("private data %s", private)
		}
	}
}

func TestRecoveryRetryLeaseStaleCompletionAndGenerationGuard(t *testing.T) {
	// Arrange.
	s, _, _, at := recoveryFixture(t)
	defer s.Close()
	ctx := context.Background()
	terminalBusiness(t, s, at, stateFailedText)
	first := recoveryClaim(t, s, at)
	if first == nil {
		t.Fatal("failedgap must wake")
	}
	// Act: worker dies with a held lease and restarts after expiry.
	next := recoveryClaim(t, s, first.LeaseUntil.Add(time.Second))
	// Assert: retries preserve immutable identity/body, stale leases cannot complete.
	if next == nil || next.EventID != first.EventID || string(next.Payload) != string(first.Payload) ||
		next.Attempts != 2 {
		t.Fatal("lease retry changed round")
	}
	if err := s.FinishRecoveryDelivery(
		ctx,
		*first,
		DeliveryOutcome{State: stateDeliveredText},
		first.LeaseUntil.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := s.DB.QueryRow(`SELECT state FROM recovery_deliveries WHERE id=?`, first.ID).
		Scan(&state); err != nil ||
		state != recoveryStateSending {
		t.Fatalf("stale completion %s %v", state, err)
	}
	if _, err := s.DB.Exec(`UPDATE event_subscriptions SET generation='replaced' WHERE id='sub'`); err != nil {
		t.Fatal(err)
	}
	allowed, err := s.recoveryDispatchAllowedAt(ctx, *next, next.LeaseUntil.Add(-time.Second))
	if err != nil || allowed {
		t.Fatalf("replacedgen=%v %v", allowed, err)
	}
	recoverySweep(t, s, at.Add(time.Hour))
	if d := recoveryClaim(t, s, at.Add(time.Hour)); d != nil {
		t.Fatal("oldgeneration claimed")
	}
}

func TestRecoveryLongBlockTerminalPrunedAndExhaustion(t *testing.T) {
	s, _, generation, at := recoveryFixture(t)
	defer s.Close()
	ctx := context.Background()
	recoverySweep(t, s, at)
	// A pending business delivery never expires the recovery watch prematurely.
	if d := recoveryClaim(t, s, at.Add(8*24*time.Hour)); d != nil {
		t.Fatal("pendinghead woke")
	}
	if _, err := s.DB.Exec(`UPDATE event_deliveries SET state='failed' WHERE subscription_id='sub'`); err != nil {
		t.Fatal(err)
	}
	readyAt := at.Add(8 * 24 * time.Hour)
	if d := recoveryClaim(t, s, readyAt); d == nil {
		t.Fatal("longblock lost continuation")
	}
	if _, err := s.DB.Exec(
		`INSERT INTO event_processing(subscription_id,generation,ack_id,pruned_id) VALUES('sub',?,0,99) ON CONFLICT(subscription_id,generation) DO UPDATE SET pruned_id=99`,
		generation,
	); err != nil {
		t.Fatal(err)
	}
	if d := recoveryClaim(t, s, readyAt.Add(time.Second)); d == nil {
		t.Fatal("pruned watermark needs wake")
	}
	recoverySweep(t, s, readyAt.Add(8*24*time.Hour))
	var exhausted int
	if err := s.DB.QueryRow(`SELECT count(*) FROM recovery_episodes WHERE state='exhausted'`).
		Scan(&exhausted); err != nil ||
		exhausted != 1 {
		t.Fatalf("exhaustion=%d %v", exhausted, err)
	}
	if d := recoveryClaim(t, s, readyAt.Add(30*24*time.Hour)); d != nil {
		t.Fatal("exhausted currenthead rearmed after retention")
	}
	status, err := s.RecoveryStatus(ctx, "owner", "sub")
	if err != nil || status["state"] != recoveryStateExhausted {
		t.Fatalf("status=%v %v", status, err)
	}
}

func TestRecoveryActivationCancellationOwnershipAndExpiry(t *testing.T) {
	s, _, generation, at := recoveryFixture(t)
	defer s.Close()
	ctx := context.Background()
	if err := s.CheckRecoveryTarget(ctx, "foreign", "sub", generation); err == nil {
		t.Fatal("foreign target allowed")
	}
	if err := s.CancelRecoverySubscription(ctx, "recovery", "foreign", at); err == nil {
		t.Fatal("foreigncancel allowed")
	}
	rev, err := s.SubscriptionRevision(ctx, "recovery")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CancelRecoverySubscription(ctx, "recovery", "owner", at); err != nil {
		t.Fatal(err)
	}
	_, err = s.ActivateRecoverySubscription(
		ctx,
		RecoverySubscription{
			ID:                 "recovery",
			Principal:          "owner",
			BusinessID:         "sub",
			BusinessGeneration: generation,
			Callback:           "https://control.example",
			Secret:             "synthetic",
		},
		rev,
		at,
	)
	if err == nil {
		t.Fatal("cancelledduringverification resurrected")
	}
	terminalBusiness(t, s, at, stateDeliveredText)
	if d := recoveryClaim(t, s, at); d != nil {
		t.Fatal("cancelledcontrol woke")
	}
	rev, err = s.SubscriptionRevision(ctx, "recovery")
	if err != nil {
		t.Fatal(err)
	}
	expiry := at.Add(time.Second)
	_, err = s.ActivateRecoverySubscription(
		ctx,
		RecoverySubscription{
			ID:                 "recovery",
			Principal:          "owner",
			BusinessID:         "sub",
			BusinessGeneration: generation,
			Callback:           "https://control.example",
			Secret:             "synthetic",
			ExpiresAt:          &expiry,
		},
		rev,
		at,
	)
	if err != nil {
		t.Fatal(err)
	}
	d := recoveryClaim(t, s, at)
	if d == nil {
		t.Fatal("freshcontrol shouldwake")
	}
	allowed, err := s.recoveryDispatchAllowedAt(ctx, *d, expiry)
	if err != nil || allowed {
		t.Fatalf("expiryguard=%v %v", allowed, err)
	}
}

func TestRecoveryMigrationDoesNotChangeBusinessRows(t *testing.T) {
	// Arrange: the same migration is applied to a populated state a second time.
	s, _, _, _ := recoveryFixture(t)
	defer s.Close()
	ctx := context.Background()
	var payload string
	var messageCount, deliveryCount int
	if err := s.DB.QueryRow(`SELECT group_concat(hex(payload),'|'),count(*) FROM event_deliveries`).
		Scan(&payload, &deliveryCount); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM messages`).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	// Act.
	if err := s.migrateRecovery(ctx); err != nil {
		t.Fatal(err)
	}
	// Assert.
	var after string
	var count int
	if err := s.DB.QueryRow(`SELECT group_concat(hex(payload),'|'),count(*) FROM event_deliveries`).
		Scan(&after, &count); err != nil || after != payload ||
		count != deliveryCount {
		t.Fatalf("businesschanged %v", err)
	}
	var version int
	if err := s.DB.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil || version != 20 {
		t.Fatalf("schema=%d %v", version, err)
	}
}

func TestRecoveryOneActiveCompanionAndNamespaces(t *testing.T) {
	// Arrange.
	s, _, generation, at := recoveryFixture(t)
	defer s.Close()
	ctx := context.Background()
	next := RecoverySubscription{
		ID:                 "other-control",
		Principal:          "owner",
		BusinessID:         "sub",
		BusinessGeneration: generation,
		Callback:           "https://other.example",
		Secret:             "synthetic",
	}
	revision, err := s.SubscriptionRevision(ctx, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Act/Assert: one attached receiver is explicit and cancellation releases it.
	if _, err = s.ActivateRecoverySubscription(ctx, next, revision, at); err == nil {
		t.Fatal("multiple active controls allowed")
	}
	if err = s.CancelRecoverySubscription(ctx, "recovery", "owner", at); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateRecoverySubscription(ctx, next, revision, at); err != nil {
		t.Fatal(err)
	}
	business := EventSubscription{
		ID:        next.ID,
		Principal: "owner",
		Profile:   domain.ConversationMessageCreatedV2,
		Scope:     "all",
		Callback:  "https://other.example",
		Secret:    "synthetic",
	}
	if _, err = s.ActivateSubscription(ctx, business, revision, at); err == nil {
		t.Fatal("business reused control namespace")
	}
	revBefore, err := s.SubscriptionRevision(ctx, "sub")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CancelRecoverySubscription(ctx, "sub", "owner", at); err == nil {
		t.Fatal("control cancellation accepted businessID")
	}
	revAfter, err := s.SubscriptionRevision(ctx, "sub")
	if err != nil || revAfter != revBefore {
		t.Fatalf("business revision mutated %v", err)
	}
}

func TestRecoveryMigrationAtomicFailureAndRetry(t *testing.T) {
	// Arrange: reproduce a version19 database with business rows and a failing migration.
	s, _, _, _ := recoveryFixture(t)
	defer s.Close()
	ctx := context.Background()
	if _, err := s.DB.Exec(
		`DROP TABLE recovery_deliveries;DROP TABLE recovery_episodes;DROP TABLE recovery_subscriptions;DELETE FROM schema_migrations WHERE version=20;CREATE TRIGGER reject_recovery_migration BEFORE INSERT ON schema_migrations WHEN NEW.version=20 BEGIN SELECT RAISE(ABORT,'synthetic');END;`,
	); err != nil {
		t.Fatal(err)
	}
	// Act.
	if err := s.migrateRecovery(ctx); err == nil {
		t.Fatal("expected atomic migration failure")
	}
	// Assert: partial control tables are absent; business corpus/deliveries survive.
	var tables, messages, deliveries int
	if err := s.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name LIKE 'recovery_%'`).
		Scan(&tables); err != nil ||
		tables != 0 {
		t.Fatalf("partialtables=%d %v", tables, err)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM messages`).Scan(&messages); err != nil || messages != 3 {
		t.Fatalf("messages=%d %v", messages, err)
	}
	if err := s.DB.QueryRow(`SELECT count(*) FROM event_deliveries`).Scan(&deliveries); err != nil || deliveries != 3 {
		t.Fatalf("deliveries=%d %v", deliveries, err)
	}
	if _, err := s.DB.Exec(`DROP TRIGGER reject_recovery_migration`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateRecovery(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryRevocationAndCallbackGoneAreDurable(t *testing.T) {
	for _, kind := range []string{"business_cancel", "policy_revoke", "callback_gone"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange.
			s, _, _, at := recoveryFixture(t)
			defer s.Close()
			ctx := context.Background()
			if kind == "policy_revoke" {
				if _, err := s.DB.Exec(
					`UPDATE event_subscriptions SET scope='conversation',conversation_type='direct',group_id='peer' WHERE id='sub'`,
				); err != nil {
					t.Fatal(err)
				}
			}
			terminalBusiness(t, s, at, stateDeliveredText)
			d := recoveryClaim(t, s, at)
			if d == nil {
				t.Fatal("missing claim")
			}
			// Act.
			applyRecoveryRevocation(t, s, *d, kind, at)
			// Assert.
			allowed, err := s.recoveryDispatchAllowedAt(ctx, *d, at)
			if err != nil || allowed {
				t.Fatalf("revoked=%v %v", allowed, err)
			}
			recoverySweep(t, s, at.Add(time.Second))
			if next := recoveryClaim(t, s, at.Add(2*time.Second)); next != nil {
				t.Fatal("revoked recovery redispatched")
			}
		})
	}
}

func applyRecoveryRevocation(t *testing.T, s *Store, d RecoveryDelivery, kind string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	switch kind {
	case "business_cancel":
		if err := s.CancelSubscription(ctx, "sub", "owner", at); err != nil {
			t.Fatal(err)
		}
	case "policy_revoke":
		s.policy = domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{}}
	case "callback_gone":
		if err := s.FinishRecoveryDelivery(
			ctx,
			d,
			DeliveryOutcome{State: stateFailedText, Reason: "callback_gone", Gone: true},
			at,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecoveryCapacityAndAttemptsHaveExplicitStopReasons(t *testing.T) {
	for _, scenario := range []string{"capacity", "attempts"} {
		t.Run(scenario, func(t *testing.T) { assertRecoveryPolicyStop(t, scenario) })
	}
}

func assertRecoveryPolicyStop(t *testing.T, scenario string) {
	t.Helper()
	// Arrange.
	s, _, _, at := recoveryFixture(t)
	defer s.Close()
	ctx := context.Background()
	terminalBusiness(t, s, at, stateDeliveredText)
	policy := DefaultDeliveryPolicy()
	if scenario == "capacity" {
		policy.MaxBytes = 1
	} else {
		policy.MaxAttempts = 1
	}
	// Act.
	d, err := s.ClaimRecoveryDelivery(ctx, at, policy)
	if err != nil {
		t.Fatal(err)
	}
	if scenario == "attempts" {
		if d == nil {
			t.Fatal("missinginitialclaim")
		}
		if err = s.FinishRecoveryDelivery(
			ctx,
			*d,
			DeliveryOutcome{State: statePendingText, Reason: "network", RetryAt: at.Add(time.Second)},
			at,
		); err != nil {
			t.Fatal(err)
		}
		if d, err = s.ClaimRecoveryDelivery(ctx, at.Add(time.Second), policy); err != nil {
			t.Fatal(err)
		}
	}
	// Assert: exhausted policy is visible and never modifies a business delivery.
	if d != nil {
		t.Fatal("policyalloweddispatch")
	}
	status, err := s.RecoveryStatus(ctx, "owner", "sub")
	if err != nil || status["reason"] != scenario {
		t.Fatalf("status=%v %v", status, err)
	}
	var delivered int
	if err = s.DB.QueryRow(`SELECT count(*) FROM event_deliveries WHERE state='delivered'`).
		Scan(&delivered); err != nil ||
		delivered != 1 {
		t.Fatalf("businesschanged %d %v", delivered, err)
	}
}

func TestRecoveryReminderBackoffCapsAtOneHour(t *testing.T) {
	// Arrange.
	s, _, _, at := recoveryFixture(t)
	defer s.Close()
	ctx := context.Background()
	terminalBusiness(t, s, at, stateDeliveredText)
	// Act/Assert: each delivered control starts a distinct immutable reminder round.
	interval := recoveryMinimumInterval
	priorID := ""
	for range 10 {
		d := recoveryClaim(t, s, at)
		if d == nil || d.EventID == priorID {
			t.Fatal("missing fresh reminder")
		}
		priorID = d.EventID
		if err := s.FinishRecoveryDelivery(
			ctx,
			*d,
			DeliveryOutcome{State: stateDeliveredText, Reason: "http_2xx"},
			at,
		); err != nil {
			t.Fatal(err)
		}
		if early := recoveryClaim(t, s, at.Add(interval-time.Second)); early != nil {
			t.Fatal("reminder accelerated")
		}
		at = at.Add(interval)
		interval *= 2
		if interval > recoveryMaximumInterval {
			interval = recoveryMaximumInterval
		}
	}
}

func TestRecoveryBroadRevokedGapDoesNotStrandAllowedTail(t *testing.T) {
	for _, scope := range []string{"all", "direct"} {
		t.Run(scope, func(t *testing.T) { assertRecoveryBroadGap(t, scope) })
	}
}

func assertRecoveryBroadGap(t *testing.T, scope string) {
	t.Helper()
	// Arrange: both callbacks finish before revocation; no further message arrives.
	s, _, _, at := recoveryFixture(t)
	defer s.Close()
	ctx := context.Background()
	for range 3 {
		terminalBusiness(t, s, at, stateDeliveredText)
	}
	initial, err := s.ReadSubscriptionEvents(ctx, "owner", "sub", 3)
	if err != nil {
		t.Fatal(err)
	}
	initialEntries := initial["events"].([]map[string]any)
	for _, entry := range initialEntries {
		recoveryAckOne(t, s, entry)
	}
	if _, err = s.DB.Exec(`UPDATE event_subscriptions SET scope=? WHERE id='sub'`, scope); err != nil {
		t.Fatal(err)
	}
	for _, peer := range []string{"revoked-peer", "allowed-peer"} {
		if err = s.Put(
			ctx,
			domain.Message{
				Conversation: domain.ConversationRef{Type: domain.ConversationDirect, ID: peer},
				ID:           peer + "-message",
				SenderID:     peer,
				Direction:    directionIncoming,
				Text:         peer + "-private-text",
				SentAt:       at,
				Source:       "live",
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.FanoutProfileEvents(
		ctx,
		at,
		DefaultDeliveryPolicy(),
		func(_, id string, m domain.Message) ([]byte, error) {
			return json.Marshal(map[string]string{"eventId": id, "text": m.Text})
		},
	); err != nil {
		t.Fatal(err)
	}
	terminalBusiness(t, s, at, stateDeliveredText)
	terminalBusiness(t, s, at, stateDeliveredText)
	s.policy = domain.CollectionPolicy{
		Selected: map[domain.ConversationRef]bool{{Type: domain.ConversationDirect, ID: "allowed-peer"}: true},
	}
	// Act/Assert: the revoked terminal head is a status-only processable gap.
	first := recoveryClaim(t, s, at)
	if first == nil {
		t.Fatal("revoked head stranded broad journal")
	}
	assertRecoveryPrivacy(t, first.Payload)
	gap := recoveryReadOne(t, s)
	if gap["event"] != nil || gap["gap_reason"] != "message_unavailable_or_access_revoked" {
		t.Fatalf("unsafe gap=%v", gap)
	}
	recoveryAckOne(t, s, gap)
	second := recoveryClaim(t, s, at)
	if second == nil || second.EpisodeID == first.EpisodeID {
		t.Fatal("allowed tail did not wake after gap ack")
	}
	entry := recoveryReadOne(t, s)
	event := entry["event"].(map[string]any)
	if event["text"] != "allowed-peer-private-text" || entry["gap_reason"] != nil {
		t.Fatalf("allowed event=%v", entry)
	}
	recoveryAckOne(t, s, entry)
	if extra := recoveryClaim(t, s, at); extra != nil {
		t.Fatal("fully acked journal kept waking")
	}
	var messages, deliveries int
	if err = s.DB.QueryRow(`SELECT (SELECT count(*) FROM messages),(SELECT count(*) FROM event_deliveries)`).
		Scan(&messages, &deliveries); err != nil || messages != 5 ||
		deliveries != 5 {
		t.Fatalf("business corpus changed %d/%d %v", messages, deliveries, err)
	}
}

func recoveryReadOne(t *testing.T, s *Store) map[string]any {
	t.Helper()
	page, err := s.ReadSubscriptionEvents(context.Background(), "owner", "sub", 1)
	if err != nil {
		t.Fatal(err)
	}
	entries := page["events"].([]map[string]any)
	if len(entries) != 1 || page["blocked_on_delivery"] != false {
		t.Fatalf("page=%v", page)
	}
	return entries[0]
}
func recoveryAckOne(t *testing.T, s *Store, entry map[string]any) {
	t.Helper()
	if _, err := s.AckSubscriptionEvents(context.Background(), "owner", "sub", entry["receipt"].(string)); err != nil {
		t.Fatal(err)
	}
}

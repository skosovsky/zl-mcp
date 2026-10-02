package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestDeliveryStopsAtExpiryDeadlineAttemptsAndAccessRevocation(t *testing.T) {
	for _, scenario := range []string{"expiration", "deadline", "attempts", "access_revoked"} {
		t.Run(scenario, func(t *testing.T) {
			// Arrange: a persisted job accepted while the subscription has access.
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "messages.sqlite")
			s, err := Open(ctx, path, []string{"g1"}, 90)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			at := time.Now().UTC()
			sub := queueSubscription(t, s, "sub", at)
			if scenario == "expiration" {
				expiry := at.Add(time.Minute)
				sub.ExpiresAt = &expiry
				rev, _ := s.SubscriptionRevision(ctx, sub.ID)
				if _, err := s.ActivateSubscription(ctx, sub, rev, at); err != nil {
					t.Fatal(err)
				}
			}
			putTest(t, s, "message", "g1", "bounded-policy", at)
			policy := DefaultDeliveryPolicy()
			if _, err := s.FanoutEvents(ctx, at, policy, queueEncoder); err != nil {
				t.Fatal(err)
			}
			claimAt := at.Add(time.Second)
			wantState, wantReason := "cancelled", "subscription_inactive"
			switch scenario {
			case "expiration":
				claimAt = at.Add(61 * time.Second)
			case "deadline":
				claimAt = at.Add(policy.MaxAge + time.Second)
				wantState, wantReason = "failed", "deadline"
			case "attempts":
				if _, err := s.DB.Exec("UPDATE event_deliveries SET attempts=?", policy.MaxAttempts); err != nil {
					t.Fatal(err)
				}
				wantState, wantReason = "failed", "attempts"
			case "access_revoked":
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				s, err = Open(ctx, path, nil, 90)
				if err != nil {
					t.Fatal(err)
				}
				wantReason = "access_revoked"
			}
			// Act: scheduler must not hand a stopped job to the network worker.
			job, err := s.ClaimDelivery(ctx, claimAt, policy)
			var state, reason string
			var payloadBytes int
			scanErr := s.DB.QueryRow("SELECT state,last_reason,length(payload) FROM event_deliveries").Scan(&state, &reason, &payloadBytes)
			// Assert: each stop condition is explicit and durable.
			if err != nil || scanErr != nil || job != nil || state != wantState || reason != wantReason {
				t.Fatalf("job=%v state=%s reason=%s err=%v scan=%v", job, state, reason, err, scanErr)
			}
			if wantState == "cancelled" && payloadBytes != 0 {
				t.Fatal("unauthorized queued text was retained")
			}
		})
	}
}
func TestQueueByteCapacityDoesNotDiscardTheCollectedMessage(t *testing.T) {
	// Arrange: payload cannot fit, despite space in the job count budget.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	queueSubscription(t, s, "sub", at)
	putTest(t, s, "large", "g1", "collected body survives delivery capacity failure", at)
	policy := DefaultDeliveryPolicy()
	policy.MaxBytes = 1
	// Act
	_, err := s.FanoutEvents(ctx, at, policy, queueEncoder)
	message, readErr := s.Message(ctx, "g1", "large")
	diagnostics, diagErr := s.DeliveryDiagnostics(ctx, at)
	// Assert: corpus is retained, and failed delivery is observable without payload.
	if err != nil || readErr != nil || diagErr != nil || message.Text == "" || diagnostics.ReasonCounts["queue_capacity"] != 1 || diagnostics.PendingPayloadBytes != 0 {
		t.Fatalf("fanout=%v read=%v diag=%v", err, readErr, diagErr)
	}
}
func TestConcurrentActivationHasOneDurableInsertionBoundary(t *testing.T) {
	// Arrange: activation and first insert contend for the service's SQLite connection.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	sub := EventSubscription{ID: "sub", Principal: "owner", GroupID: "g1", Callback: "https://receiver.example", Secret: "synthetic"}
	rev, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	inserted := make(chan error, 1)
	activated := make(chan error, 1)
	var boundary int64
	go func() { <-start; inserted <- s.Put(ctx, testEventMessage("concurrent", at)) }()
	go func() {
		<-start
		result, err := s.ActivateSubscription(ctx, sub, rev, at)
		boundary = result.StartSeq
		activated <- err
	}()
	// Act
	close(start)
	if err := <-inserted; err != nil {
		t.Fatal(err)
	}
	if err := <-activated; err != nil {
		t.Fatal(err)
	}
	_, err = s.FanoutEvents(ctx, at, DefaultDeliveryPolicy(), queueEncoder)
	if err != nil {
		t.Fatal(err)
	}
	var sequence int64
	var jobs int
	if err := s.DB.QueryRow("SELECT seq FROM messages WHERE message_id='concurrent'").Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.QueryRow("SELECT count(*) FROM event_deliveries").Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	// Assert: whichever transaction commits first determines inclusion, without loss
	// relative to the successful activation boundary or historical backfill.
	want := 0
	if sequence > boundary {
		want = 1
	}
	if jobs != want {
		t.Fatalf("sequence=%d boundary=%d jobs=%d", sequence, boundary, jobs)
	}
}

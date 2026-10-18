package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

//nolint:gocognit // Keep the full Arrange–Act–Assert state matrix and privacy/read-only assertions together.
func TestLegacyRollbackRecoveryControlState(t *testing.T) {
	cases := []struct {
		name, subscription, episode, delivery, payload string
		active, work, retained                         int64
	}{
		{name: "empty"},
		{name: "active_companion", subscription: "active", active: 1},
		{name: "inactive_companion", subscription: "inactive"},
		{name: "inactive_dormant_armed", subscription: "inactive", episode: "armed", work: 1},
		{name: "inactive_ready", subscription: "inactive", episode: "ready", work: 1},
		{name: "inactive_exhausted", subscription: "inactive", episode: "exhausted", work: 1},
		{name: "suppressed_episode", subscription: "inactive", episode: "suppressed"},
		{name: "pending_empty_payload", delivery: "pending", work: 1},
		{name: "sending_empty_payload", delivery: "sending", work: 1},
		{name: "delivered_retained_orphan", delivery: "delivered", payload: "synthetic-control", retained: 1},
		{name: "failed_retained_orphan", delivery: "failed", payload: "synthetic-control", retained: 1},
		{name: "terminal_empty", delivery: "delivered"},
		{
			name:         "active_with_work_and_payload",
			subscription: "active",
			episode:      "armed",
			delivery:     "pending",
			payload:      "synthetic-control",
			active:       1,
			work:         2,
			retained:     1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: records may be orphaned or belong to an inactive companion.
			ctx := t.Context()
			s := openTest(t)
			if tc.subscription != "" {
				active := 0
				if tc.subscription == "active" {
					active = 1
				}
				_, err := s.DB.ExecContext(
					ctx,
					`INSERT INTO recovery_subscriptions(id,principal,business_id,business_generation,callback,secret,generation,active,created_at) VALUES('control','owner','business','business-gen','https://synthetic.example','private-synthetic-secret','generation',?,'2026-10-18')`,
					active,
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			if tc.episode != "" {
				_, err := s.DB.ExecContext(
					ctx,
					`INSERT INTO recovery_episodes(id,subscription_id,generation,head,state,next_at,deadline) VALUES('episode','control','generation',1,?,'2026-10-18','2026-10-19')`,
					tc.episode,
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			if tc.delivery != "" {
				_, err := s.DB.ExecContext(
					ctx,
					`INSERT INTO recovery_deliveries(event_id,episode_id,subscription_id,generation,payload,state,next_attempt_at,deadline) VALUES('event','episode','control','generation',?,?, '2026-10-18','2026-10-19')`,
					[]byte(tc.payload),
					tc.delivery,
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			// Act: checking never mutates or cleans the state to produce compatibility.
			report, err := CheckLegacyRollback(ctx, s.DB)
			// Assert: every incompatible category blocks independently; output is counts only.
			if err != nil {
				t.Fatal(err)
			}
			wantCompatible := tc.active == 0 && tc.work == 0 && tc.retained == 0
			if report.Compatible != wantCompatible || report.ActiveRecoverySubscriptions != tc.active ||
				report.UndrainedRecoveryWork != tc.work ||
				report.RetainedRecoveryPayloads != tc.retained {
				t.Fatalf(
					"report %+v expected compatible=%v active=%d work=%d payloads=%d",
					report,
					wantCompatible,
					tc.active,
					tc.work,
					tc.retained,
				)
			}
			raw, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{"synthetic.example", "private-synthetic-secret", "synthetic-control", "business-gen"} {
				if strings.Contains(string(raw), private) {
					t.Fatal("private value in rollback report")
				}
			}
			again, err := CheckLegacyRollback(ctx, s.DB)
			if err != nil || again != report {
				t.Fatalf("read-only report changed %+v %+v %v", report, again, err)
			}
		})
	}
}

func TestLegacyRollbackOlderSchemaWithoutRecoveryTables(t *testing.T) {
	// Arrange: a pre-recovery database must be checked without requiring a migration.
	s := openTest(t)
	ctx := t.Context()
	if _, err := s.DB.ExecContext(
		ctx,
		`DROP TABLE recovery_deliveries;DROP TABLE recovery_episodes;DROP TABLE recovery_subscriptions`,
	); err != nil {
		t.Fatal(err)
	}
	// Act
	report, err := CheckLegacyRollback(ctx, s.DB)
	// Assert: missing optional tables mean zero, and checking never recreates them.
	if err != nil || !report.Compatible || report.ActiveRecoverySubscriptions != 0 ||
		report.UndrainedRecoveryWork != 0 ||
		report.RetainedRecoveryPayloads != 0 {
		t.Fatal(report, err)
	}
	var tables int
	if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name IN ('recovery_deliveries','recovery_episodes','recovery_subscriptions')`).
		Scan(&tables); err != nil ||
		tables != 0 {
		t.Fatal("check migrated legacy schema", tables, err)
	}
}

func TestLegacyRollbackRecoveryDeliveryWithoutOtherTables(t *testing.T) {
	// Arrange: do not hide orphan retained payloads behind companion-table absence.
	s := openTest(t)
	ctx := t.Context()
	if _, err := s.DB.ExecContext(
		ctx,
		`DROP TABLE recovery_episodes;DROP TABLE recovery_subscriptions;INSERT INTO recovery_deliveries(event_id,episode_id,subscription_id,generation,payload,state,next_attempt_at,deadline) VALUES('orphan','none','none','generation',X'0102','delivered','2026-10-18','2026-10-19')`,
	); err != nil {
		t.Fatal(err)
	}
	// Act
	report, err := CheckLegacyRollback(ctx, s.DB)
	// Assert
	if err != nil || report.Compatible || report.ActiveRecoverySubscriptions != 0 ||
		report.UndrainedRecoveryWork != 0 ||
		report.RetainedRecoveryPayloads != 1 {
		t.Fatal(report, err)
	}
}

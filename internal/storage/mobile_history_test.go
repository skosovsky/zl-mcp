package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func mobileOperation(t *testing.T) (*Store, HistoryOperation, string, domain.MobileHistoryPage) {
	t.Helper()
	return mobileOperationRequest(t, nil)
}

func mobileOperationRequest(t *testing.T, configure func(*domain.HistoryImportRequest)) (*Store, HistoryOperation, string, domain.MobileHistoryPage) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s := historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	request := historyRequest()
	request.ConversationType = "direct"
	request.ConversationID = "12"
	request.PageSize = 3
	if configure != nil {
		configure(&request)
	}
	op, err := s.PrepareMobileHistoryOperation(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	op, err = s.ClaimHistoryOperation(context.Background(), op.Status.OperationID, op.Revision)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().Add(-time.Second).UnixMilli()
	ts := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	message := historicalFixture(request.Ref(), "13")
	message.SentAt = ts
	message.Direction = "incoming"
	message.SenderID = request.ConversationID
	source := domain.MobileHistorySnapshot{ID: op.Status.RequestID, Digest: [32]byte{1, 2, 3}, CreatedMS: created, ExpiresMS: created + int64(15*time.Minute/time.Millisecond), SourceRows: 4, PeriodRows: 4, HasRange: true, EarliestMS: ts.UnixMilli(), LatestMS: ts.Add(time.Second).UnixMilli()}
	page := domain.MobileHistoryPage{Snapshot: source, Records: []domain.ExpiringHistoryRecord{{Message: message, ExpiresAtMS: time.Now().Add(time.Hour).UnixMilli()}}, Counts: domain.MobileHistoryCounts{Examined: 3, Rejected: 1, Expired: 1}, HasMore: true, Next: &domain.MobileHistoryPosition{TimestampMS: ts.UnixMilli(), RowID: 9007199254740993}}
	return s, op, path, page
}
func assertMobileEmpty(t *testing.T, s *Store, op HistoryOperation) {
	t.Helper()
	assertNoExpiringPage(t, s, op)
	for _, table := range []string{"history_mobile_sources", "peer_first_incoming"} {
		var n int
		if err := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("mobile state escaped rollback", table, n, err)
		}
	}
}
func TestMobileHistoryTransactionWorkCannotExceedBudget(t *testing.T) {
	// Arrange: source work has consumed the whole budget before persistence.
	s, op, _, page := mobileOperation(t)
	defer s.Close()

	// Act: transaction overhead must not publish a page beyond the work limit.
	_, err := s.CommitMobileHistoryPage(context.Background(), op.Status.OperationID, op.Revision, page, MobileHistoryWorkBudget)

	// Assert: all records and checkpoints remain uncommitted.
	if err == nil {
		t.Fatal("transaction exceeded work budget")
	}
	assertMobileEmpty(t, s, op)
}

func TestMobileHistoryAtomicRollbackAtSourceAndOperationCheckpoint(t *testing.T) {
	for _, where := range []string{"source", "operation"} {
		t.Run(where, func(t *testing.T) {
			// Arrange: a valid page and a failure after message/TTL/identity writes.
			s, op, _, page := mobileOperation(t)
			defer s.Close()
			trigger := `CREATE TRIGGER reject_mobile_source BEFORE INSERT ON history_mobile_sources BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`
			if where == "operation" {
				trigger = `CREATE TRIGGER reject_mobile_operation BEFORE UPDATE ON history_operations BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`
			}
			if _, err := s.DB.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			// Act.
			_, err := s.CommitMobileHistoryPage(context.Background(), op.Status.OperationID, op.Revision, page, time.Millisecond)
			// Assert: nothing, including the private source and public raw counters, was committed.
			if err == nil {
				t.Fatal("injected failure reported success")
			}
			assertMobileEmpty(t, s, op)
		})
	}
}
func TestMobileHistoryRestartExactKeysetCountsAndSilentDuplicates(t *testing.T) {
	// Arrange.
	s, op, path, page := mobileOperation(t)
	ctx := context.Background()
	// Act: one page, close/reopen, recover and claim the exact saved source.
	next, err := s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	assertHistoryStatusSchema(t, next)
	if next.Status.RecordsObserved != 3 || next.Status.InsertedCount != 1 || next.Status.State != "running" || next.Status.MobileCoverage.Rejected != 1 || next.Status.MobileCoverage.Expired != 1 {
		t.Fatal("raw counts became retained counts")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = historyOperationStore(t, path, domain.CollectionPolicy{All: true})
	defer s.Close()
	if err = s.RecoverInterruptedHistory(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.HistoryOperation(ctx, op.Status.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := s.MobileHistoryCheckpoint(ctx, op.Status.OperationID)
	if err != nil || checkpoint.Snapshot != page.Snapshot || checkpoint.Next == nil || *checkpoint.Next != *page.Next {
		t.Fatal("source or exact large keyset changed", err)
	}
	encoded, _ := json.Marshal(checkpoint)
	if string(encoded) != "{}" || fmt.Sprintf("%#v", checkpoint) != "mobile history checkpoint [redacted]" {
		t.Fatal("private checkpoint serialized")
	}
	loaded, err = s.ClaimHistoryOperation(ctx, loaded.Status.OperationID, loaded.Revision)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := page.Records[0]
	duplicate.ExpiresAtMS += int64(time.Hour / time.Millisecond)
	final := domain.MobileHistoryPage{Snapshot: page.Snapshot, Counts: domain.MobileHistoryCounts{Examined: 1}, Records: []domain.ExpiringHistoryRecord{duplicate}}
	completed, err := s.CommitMobileHistoryPage(ctx, loaded.Status.OperationID, loaded.Revision, final, time.Millisecond)
	// Assert: examined rows include gaps; a later duplicate never extends TTL or creates Events.
	if err != nil || completed.Status.State != "partial" || completed.Status.StopReason == nil || *completed.Status.StopReason != "source_gaps" || completed.Status.RecordsObserved != 4 || completed.Status.InsertedCount != 1 || completed.Status.DuplicateCount != 1 || completed.Status.MobileCoverage.Examined != 4 {
		t.Fatal("bad final coverage", err)
	}
	assertHistoryStatusSchema(t, completed)
	var ttl int64
	if err = s.DB.QueryRow("SELECT expires_ms FROM history_message_expiry WHERE message_id='13'").Scan(&ttl); err != nil || ttl != page.Records[0].ExpiresAtMS {
		t.Fatal("duplicate changed TTL", err)
	}
	for _, table := range []string{"message_events", "event_deliveries"} {
		var n int
		if err = s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("historical page produced delivery", table, err)
		}
	}
	if _, err = s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, time.Millisecond); !errors.Is(err, ErrHistoryState) {
		t.Fatal("committed page replayed", err)
	}
}
func TestMobileHistoryGapOnlyPageAdvancesAndCountsAgainstBudget(t *testing.T) {
	// Arrange: every source row is skipped, but an exact continuation remains.
	s, op, _, page := mobileOperation(t)
	defer s.Close()
	ctx := context.Background()
	page.Records = nil
	page.Counts = domain.MobileHistoryCounts{Examined: 3, UnsupportedContent: 1, MissingMetadata: 1, InvalidMetadata: 1}
	// Act.
	next, err := s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, time.Millisecond)
	// Assert: no false empty-page stop and no message insertion.
	if err != nil || next.Status.State != "running" || next.Status.RecordsObserved != 3 || next.Status.InsertedCount != 0 {
		t.Fatal("gap-only page did not advance", err)
	}
	checkpoint, err := s.MobileHistoryCheckpoint(ctx, op.Status.OperationID)
	if err != nil || !reflect.DeepEqual(checkpoint.Next, page.Next) {
		t.Fatal("gap-only keyset lost", err)
	}
	final := domain.MobileHistoryPage{Snapshot: page.Snapshot, Counts: domain.MobileHistoryCounts{Examined: 1, Expired: 1}}
	next, err = s.CommitMobileHistoryPage(ctx, next.Status.OperationID, next.Revision, final, time.Millisecond)
	if err != nil || next.Status.RecordsObserved != 4 || next.Status.State != "partial" || *next.Status.StopReason != "source_gaps" {
		t.Fatal("gap-only final page lost", err)
	}
	assertHistoryStatusSchema(t, next)
}
func TestMobileHistoryRejectsInvalidWholePagesAndSourceGates(t *testing.T) {
	for _, mode := range []string{"counts", "negative-gap", "out-of-period", "wal", "controls", "expired-source", "wrong-source", "invalid-ttl", "missing-next", "terminal-next", "zero-continuing"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: valid bound page, then one malformed or ineligible component.
			s, op, _, page := mobileOperation(t)
			defer s.Close()
			switch mode {
			case "counts":
				page.Counts.Examined = 2
			case "negative-gap":
				page.Counts.Rejected = -1
			case "out-of-period":
				page.Records[0].Message.SentAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			case "wal":
				page.Snapshot.WALMode = true
			case "controls":
				page.Snapshot.ControlRows = 1
			case "expired-source":
				page.Snapshot.CreatedMS = time.Now().Add(-16 * time.Minute).UnixMilli()
				page.Snapshot.ExpiresMS = page.Snapshot.CreatedMS + int64(15*time.Minute/time.Millisecond)
			case "wrong-source":
				page.Snapshot.ID = "00000000-0000-4000-8000-000000000099"
			case "invalid-ttl":
				page.Records[0].ExpiresAtMS = -1
			case "missing-next":
				page.Next = nil
			case "terminal-next":
				page.HasMore = false
			case "zero-continuing":
				page.Records = nil
				page.Counts = domain.MobileHistoryCounts{}
			}
			// Act / Assert: no valid prefix survives complete-page rejection.
			if _, err := s.CommitMobileHistoryPage(context.Background(), op.Status.OperationID, op.Revision, page, time.Millisecond); err == nil {
				t.Fatal("invalid mobile page accepted")
			}
			assertMobileEmpty(t, s, op)
		})
	}
}
func TestMobileHistoryImmutableSourceCancellationAndLegacyWorkerBoundary(t *testing.T) {
	// Arrange: public source remains unsupported and legacy worker cannot claim mobile queue entries.
	s, op, _, page := mobileOperation(t)
	defer s.Close()
	ctx := context.Background()
	r := op.Status.HistoryImportRequest
	if _, err := r.Normalize(); err == nil {
		t.Fatal("public request advertised internal source")
	}
	raw, _ := json.Marshal(map[string]any{"source": domain.HistorySourceMobileArchive, "request_id": r.RequestID, "conversation_type": r.ConversationType, "conversation_id": r.ConversationID, "since": r.Since, "until": r.Until})
	var input any
	json.Unmarshal(raw, &input)
	schema, err := contracts.Compile("history_import_request", "input")
	if err != nil || schema.Validate(input) == nil {
		t.Fatal("public schema accepted internal source", err)
	}
	retry, err := s.PrepareMobileHistoryOperation(ctx, r)
	if err != nil || retry.Status.OperationID != op.Status.OperationID {
		t.Fatal("mobile retry changed identity", err)
	}
	if _, err = s.CommitExpiringHistoryOperationPage(ctx, op.Status.OperationID, op.Revision, domain.HistoryPage{}, page.Records, time.Millisecond); !errors.Is(err, ErrHistoryState) {
		t.Fatal("generic commit bypassed mobile proof", err)
	}
	next, err := s.CommitMobileHistoryPage(ctx, op.Status.OperationID, op.Revision, page, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	// Act / Assert: an otherwise valid next page cannot change source identity or lifetime.
	for _, mode := range []string{"digest", "expiry", "coverage", "position"} {
		changed := page
		changed.Records = nil
		changed.Counts = domain.MobileHistoryCounts{Examined: 1, Expired: 1}
		position := *page.Next
		position.RowID++
		changed.Next = &position
		switch mode {
		case "digest":
			changed.Snapshot.Digest[0]++
		case "expiry":
			changed.Snapshot.CreatedMS++
			changed.Snapshot.ExpiresMS++
		case "coverage":
			changed.Snapshot.SourceRows++
		case "position":
			changed.Next = page.Next
		}
		if _, err = s.CommitMobileHistoryPage(ctx, next.Status.OperationID, next.Revision, changed, time.Millisecond); err == nil {
			t.Fatal("saved source/checkpoint replaced", mode)
		}
	}
	if _, err = s.CancelHistoryOperation(ctx, next.Status.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitMobileHistoryPage(ctx, next.Status.OperationID, next.Revision, page, time.Millisecond); !errors.Is(err, ErrHistoryState) {
		t.Fatal("cancelled work persisted", err)
	}
	// Arrange: a fresh queued mobile request after cancellation.
	r.RequestID = "00000000-0000-4000-8000-000000000002"
	queued, err := s.PrepareMobileHistoryOperation(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := s.PendingHistoryOperations(ctx, 10)
	if err != nil || len(legacy) != 0 {
		t.Fatal("legacy worker received mobile source", err)
	}
	mobile, err := s.PendingMobileHistoryOperations(ctx, 10)
	if err != nil || len(mobile) != 1 || mobile[0].Status.OperationID != queued.Status.OperationID {
		t.Fatal("mobile queue lost source", err)
	}
}
func TestMobileHistoryReservationRecoveryAndMigrationRollback(t *testing.T) {
	// Arrange: a mobile reservation may exceed the legacy budget, but remains bounded.
	s, op, _, _ := mobileOperation(t)
	defer s.Close()
	ctx := context.Background()
	reserved, err := s.ReserveMobileHistoryWork(ctx, op.Status.OperationID, op.Revision, 180*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Act: recover interrupted work without a source call or phone dispatch.
	if err = s.RecoverInterruptedHistory(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.HistoryOperation(ctx, reserved.Status.OperationID)
	// Assert: charge the reservation, preserve request and remain within mobile work allowance.
	if err != nil || recovered.Status.State != "queued" || recovered.WorkDuration != 180*time.Second || recovered.ReservedDuration != 0 {
		t.Fatal("mobile work reservation lost", err)
	}
	assertHistoryStatusSchema(t, recovered)
	if _, err = s.DB.Exec(`DROP TABLE history_mobile_sources;DELETE FROM schema_migrations WHERE version=13;CREATE TRIGGER reject_mobile_history_migration BEFORE INSERT ON schema_migrations WHEN new.version=13 BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = s.migrateMobileHistory(ctx); err == nil {
		t.Fatal("migration failure ignored")
	}
	var n int
	if err = s.DB.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='history_mobile_sources'").Scan(&n); err != nil || n != 0 {
		t.Fatal("partial mobile migration retained", err)
	}
	if _, err = s.MobileHistoryCheckpoint(ctx, recovered.Status.OperationID); err == nil || errors.Is(err, sql.ErrNoRows) {
		t.Fatal("missing table was hidden", err)
	}
}

func TestMobileHistoryTerminalSourceCountsAndLimitWithoutContinuation(t *testing.T) {
	for _, mode := range []string{"empty", "invalid-timestamps", "message-limit", "page-limit", "contradictory-exhaustion", "position-outside-source"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: frozen source coverage makes exhaustion and operation limits distinguishable.
			s, op, _, page := mobileOperationRequest(t, func(r *domain.HistoryImportRequest) {
				if mode == "message-limit" {
					r.MaxMessages = 3
				}
				if mode == "page-limit" {
					r.MaxPages = 1
				}
			})
			defer s.Close()
			switch mode {
			case "empty", "invalid-timestamps":
				page.Records = nil
				page.Counts = domain.MobileHistoryCounts{}
				page.HasMore = false
				page.Next = nil
				page.Snapshot.SourceRows = 0
				page.Snapshot.PeriodRows = 0
				page.Snapshot.HasRange = false
				page.Snapshot.EarliestMS = 0
				page.Snapshot.LatestMS = 0
				if mode == "invalid-timestamps" {
					page.Snapshot.SourceRows = 1
					page.Snapshot.InvalidTimestampRows = 1
				}
			case "message-limit", "page-limit":
				page.Next = nil
			case "contradictory-exhaustion":
				page.HasMore = false
				page.Next = nil
			case "position-outside-source":
				position := *page.Next
				position.TimestampMS += int64(time.Minute / time.Millisecond)
				page.Next = &position
			}
			// Act.
			next, err := s.CommitMobileHistoryPage(context.Background(), op.Status.OperationID, op.Revision, page, time.Millisecond)
			// Assert: a limit is partial, an empty available source is not a completeness guarantee.
			if mode == "contradictory-exhaustion" || mode == "position-outside-source" {
				if err == nil {
					t.Fatal("contradictory source evidence accepted")
				}
				assertMobileEmpty(t, s, op)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			expectedState, expectedReason := "completed", "available_source_exhausted"
			if mode == "invalid-timestamps" {
				expectedState, expectedReason = "partial", "source_gaps"
			}
			if mode == "message-limit" {
				expectedState, expectedReason = "partial", "message_limit"
			}
			if mode == "page-limit" {
				expectedState, expectedReason = "partial", "page_limit"
			}
			if next.Status.State != expectedState || *next.Status.StopReason != expectedReason || next.Status.HistoryComplete {
				t.Fatal("source stop misreported")
			}
			assertHistoryStatusSchema(t, next)
		})
	}
}

func TestMobileHistoryPolicyRevocationAndForeignAccountCannotPersist(t *testing.T) {
	for _, mode := range []string{"revoked", "foreign-account"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: a running operation loses access before a page arrives.
			s, op, path, page := mobileOperation(t)
			if mode == "revoked" {
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				s = historyOperationStore(t, path, domain.CollectionPolicy{})
			} else {
				if _, err := s.DB.Exec("UPDATE metadata SET value='foreign-account-binding' WHERE key='account'"); err != nil {
					t.Fatal(err)
				}
			}
			defer s.Close()
			// Act.
			next, err := s.CommitMobileHistoryPage(context.Background(), op.Status.OperationID, op.Revision, page, time.Millisecond)
			// Assert: revoked work cancels; foreign binding rejects; neither commits a record prefix.
			if mode == "revoked" && (err != nil || next.Status.State != "cancelled" || *next.Status.StopReason != "access_revoked") {
				t.Fatal("revoked page not cancelled", err)
			}
			if mode == "foreign-account" && err == nil {
				t.Fatal("foreign account persisted source")
			}
			for _, table := range []string{"messages", "message_identities", "history_mobile_sources", "history_message_expiry", "peer_first_incoming", "message_events", "event_deliveries"} {
				var n int
				if err := s.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
					t.Fatal("unauthorized page escaped", table, n, err)
				}
			}
		})
	}
}

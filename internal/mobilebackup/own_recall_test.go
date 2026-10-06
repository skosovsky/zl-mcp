package mobilebackup

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"testing"
)

func TestOwnRecallClassificationPreservesExactTargetAndSourceGates(t *testing.T) {
	// Arrange: a bounded direct archive control with the live-observed type/status.
	r := selectedFetchRequest()
	normalized, _ := r.Normalize()
	row := candidateSQLiteRow("901", 36)
	row.Status = 3
	row.BinNet = nil
	mapper := identitySourceFunc(func(ctx context.Context, req domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		if len(req.Direct) != 1 || req.Direct[0] != "901" || len(req.Groups) != 0 {
			t.Fatal("wrong mapping scope")
		}
		return []domain.MobileIdentityPair{{Plain: "901", Session: "10"}}, nil
	})
	// Act: classify then inspect the account/request-bound page, retaining source gates.
	prepared, err := PrepareRowPage(context.Background(), []SQLiteRow{row}, r, "10", mapper)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Clear()
	page := PreparedArchivePage{Candidates: prepared, SourceWAL: true, SourceControls: 1, requestID: r.RequestID, fingerprint: normalized.Fingerprint(), account: "10"}
	inspection, err := InspectPreparedArchivePage(context.Background(), page, r, "10", row.TimestampMS+1)
	// Assert: exact large ID, no ordinary message/text, no persistence eligibility.
	if err != nil || len(prepared.Rows) != 0 || prepared.DeferredControls != 1 || len(prepared.OwnRecalls) != 1 || prepared.OwnRecalls[0].MessageID != row.MessageID || inspection.OwnRecallCandidates != 1 || inspection.TextCandidates != 0 || len(inspection.BlockReasons) != 1 || inspection.BlockReasons[0] != "unverified_wal_snapshot" {
		t.Fatal("recall classification or source gate mismatch")
	}
	if result, err := ConvertPreparedArchivePage(context.Background(), page, r, "10", row.TimestampMS+1); err == nil || len(result.Records) != 0 {
		t.Fatal("recall classification authorized import")
	}
	encoded, _ := json.Marshal(prepared)
	if string(encoded) != "{}" {
		t.Fatal("private target exposed")
	}
	ids := prepared.OwnRecalls
	prepared.Clear()
	if len(ids) != 1 || ids[0] != (domain.MobileHistoryRecall{}) {
		t.Fatal("private target not cleared")
	}
}

func TestOwnRecallClassificationScopeAndMappingFailures(t *testing.T) {
	for _, mode := range []string{"incoming", "group", "other-status", "type33", "missing-map", "third-sender", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: controls outside the observed own/direct/status-3 combination.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := selectedFetchRequest()
			row := candidateSQLiteRow("901", 36)
			row.Status = 3
			if mode == "group" {
				r.ConversationType = "group"
			}
			if mode == "other-status" {
				row.Status = 1
			}
			if mode == "type33" {
				row.Type = 33
			}
			calls := 0
			mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
				calls++
				if mode == "missing-map" {
					return nil, nil
				}
				if mode == "cancel" {
					cancel()
				}
				sender := "10"
				if mode == "incoming" {
					sender = r.ConversationID
				}
				if mode == "third-sender" {
					sender = "13"
				}
				return []domain.MobileIdentityPair{{Plain: "901", Session: sender}}, nil
			})
			// Act.
			got, err := PrepareRowPage(ctx, []SQLiteRow{row}, r, "10", mapper)
			defer got.Clear()
			// Assert: no target guessed; mapping/cancellation faults expose no result prefix.
			fault := mode == "missing-map" || mode == "third-sender" || mode == "cancel"
			if fault {
				if !errors.Is(err, ErrSQLite) || got.Examined != 0 || len(got.OwnRecalls) != 0 {
					t.Fatal("partial recall accepted")
				}
				return
			}
			if err != nil || len(got.OwnRecalls) != 0 || got.DeferredControls != 1 || len(got.Rows) != 0 {
				t.Fatal("unsupported control interpreted")
			}
			if (mode == "group" || mode == "other-status" || mode == "type33") && calls != 0 {
				t.Fatal("unsupported control requested mapping")
			}
		})
	}
}

func TestDuplicateOwnRecallTargetsFailWithoutPrefix(t *testing.T) {
	// Arrange: two source rows ambiguously reuse one exact own-recall ID.
	row := candidateSQLiteRow("901", 36)
	row.Status = 3
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		return []domain.MobileIdentityPair{{Plain: "901", Session: "10"}}, nil
	})
	// Act.
	got, err := PrepareRowPage(context.Background(), []SQLiteRow{row, row}, selectedFetchRequest(), "10", mapper)
	// Assert: no partial private target list or counts survive.
	if !errors.Is(err, ErrSQLite) || len(got.OwnRecalls) != 0 || got.Examined != 0 {
		t.Fatal("ambiguous target prefix escaped")
	}
}

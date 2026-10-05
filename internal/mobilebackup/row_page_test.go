package mobilebackup

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func candidateSQLiteRow(sender string, kind int64) SQLiteRow {
	metadata, _ := hex.DecodeString("000000090000000178")
	return SQLiteRow{SenderID: sender, MessageID: "18446744073709551615", ClientID: "9007199254740993", Text: "synthetic payload", TimestampMS: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC).UnixMilli(), TTL: 0, Type: kind, Status: 1, BinNet: metadata}
}
func TestPreparedRowPageTypedKindsDirectionAndCoverage(t *testing.T) {
	// Arrange: known text/photo rows, controls/unknown/missing/malformed metadata.
	rows := []SQLiteRow{candidateSQLiteRow("901", 0), candidateSQLiteRow("902", 3), candidateSQLiteRow("902", 33), candidateSQLiteRow("902", 999), candidateSQLiteRow("902", 0), candidateSQLiteRow("902", 0)}
	rows[0].TTL = 60000
	rows[4].BinNet = nil
	rows[5].BinNet = []byte{1}
	calls := 0
	mapper := identitySourceFunc(func(c context.Context, r domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		calls++
		if len(r.Direct) != 2 || r.Direct[0] != "901" || r.Direct[1] != "902" || len(r.Groups) != 0 {
			t.Fatal("wrong sender scope")
		}
		return []domain.MobileIdentityPair{{Plain: "902", Session: "12"}, {Plain: "901", Session: "10"}}, nil
	})
	// Act.
	got, e := PrepareRowPage(context.Background(), rows, selectedFetchRequest(), "10", mapper)
	// Assert: preserve IDs/kinds/presence and report exclusions, no pretend ordinary text.
	if e != nil || calls != 1 || len(got.Rows) != 2 || got.Examined != 6 || got.DeferredControls != 1 || got.UnsupportedTypes != 1 || got.MissingMetadata != 1 || got.InvalidMetadata != 1 || got.UnsupportedMetadataFields != 2 || got.Rows[0].Row.MessageID != "18446744073709551615" || got.Rows[0].Row.ClientID != "9007199254740993" || !got.Rows[0].ExpiryDeclared || got.Rows[0].ExpiresMS != rows[0].TimestampMS+60000 || got.Rows[1].ExpiryDeclared || got.Rows[0].Direction != "outgoing" || got.Rows[1].Direction != "incoming" || got.Rows[1].Kind != "chat.photo" {
		t.Fatal("row preparation mismatch", e)
	}
	b, _ := json.Marshal(got)
	if string(b) != "{}" || fmt.Sprintf("%#v", got) != "mobile backup prepared row page [redacted]" || fmt.Sprintf("%#v", got.Rows[0]) != "mobile backup prepared row [redacted]" {
		t.Fatal("prepared rows exposed")
	}
	original := append([]byte(nil), rows[0].BinNet...)
	owned := got.Rows[0].Row.BinNet
	got.Clear()
	if got.Rows != nil || !bytes.Equal(owned, make([]byte, len(owned))) || !bytes.Equal(rows[0].BinNet, original) {
		t.Fatal("row ownership broken")
	}
}
func TestPreparedRowPageFailsWholeMappingAndDirectScope(t *testing.T) {
	for _, mode := range []string{"missing", "duplicate-source", "duplicate-target", "third-sender", "group-pair", "private-error", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mapper := identitySourceFunc(func(c context.Context, r domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
				pairs := []domain.MobileIdentityPair{{Plain: "901", Session: "10"}, {Plain: "902", Session: "12"}}
				switch mode {
				case "missing":
					return pairs[:1], nil
				case "duplicate-source":
					pairs[1].Plain = "901"
				case "duplicate-target":
					pairs[1].Session = "10"
				case "third-sender":
					pairs[1].Session = "13"
				case "group-pair":
					pairs[1].Group = true
				case "private-error":
					return nil, errors.New("private marker must not escape")
				case "cancel":
					cancel()
				}
				return pairs, nil
			})
			// Act.
			got, e := PrepareRowPage(ctx, []SQLiteRow{candidateSQLiteRow("901", 0), candidateSQLiteRow("902", 0)}, selectedFetchRequest(), "10", mapper)
			// Assert: no candidate/counters prefix or raw error survives.
			if !errors.Is(e, ErrSQLite) || got.Rows != nil || got.Examined != 0 {
				t.Fatal("partial mapped page accepted")
			}
		})
	}
}
func TestPreparedRowPageValidatesBeforeMappingAndHandlesEmptyPage(t *testing.T) {
	calls := 0
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		calls++
		return nil, nil
	})
	for _, change := range []func(*SQLiteRow){func(r *SQLiteRow) { r.MessageID = "0" }, func(r *SQLiteRow) { r.TimestampMS = 1 }, func(r *SQLiteRow) { r.Status = 0 }, func(r *SQLiteRow) { r.TTL = -1 }, func(r *SQLiteRow) { r.TTL = math.MaxInt64 }} {
		// Arrange.
		row := candidateSQLiteRow("901", 0)
		change(&row)
		// Act / Assert.
		if _, e := PrepareRowPage(context.Background(), []SQLiteRow{candidateSQLiteRow("902", 0), row}, selectedFetchRequest(), "10", mapper); e == nil {
			t.Fatal("invalid whole page accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid row reached mapping")
	}
	got, e := PrepareRowPage(context.Background(), nil, selectedFetchRequest(), "10", mapper)
	if e != nil || len(got.Rows) != 0 || calls != 0 {
		t.Fatal("empty page caused mapping")
	}
}

func TestPreparedRowPageBoundsBeforeSourceWork(t *testing.T) {
	// Arrange: request/aggregate budgets are independent of individual valid rows.
	calls := 0
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		calls++
		return nil, nil
	})
	request := selectedFetchRequest()
	request.MaxMessages = 1
	// Act / Assert.
	if _, e := PrepareRowPage(context.Background(), []SQLiteRow{candidateSQLiteRow("901", 0), candidateSQLiteRow("902", 0)}, request, "10", mapper); e == nil {
		t.Fatal("selection record budget ignored")
	}
	rows := make([]SQLiteRow, 51)
	for i := range rows {
		rows[i] = candidateSQLiteRow("901", 0)
	}
	if _, e := PrepareRowPage(context.Background(), rows, selectedFetchRequest(), "10", mapper); e == nil {
		t.Fatal("page record limit ignored")
	}
	rows = rows[:9]
	for i := range rows {
		rows[i].Text = string(bytes.Repeat([]byte{'x'}, 1<<20))
	}
	if _, e := PrepareRowPage(context.Background(), rows, selectedFetchRequest(), "10", mapper); e == nil {
		t.Fatal("aggregate text budget ignored")
	}
	if calls != 0 {
		t.Fatal("over-budget page reached mapper")
	}
}

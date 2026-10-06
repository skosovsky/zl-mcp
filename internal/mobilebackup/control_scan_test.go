package mobilebackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func controlScanFixture(t *testing.T, count int, modify func(*sql.DB)) (SelectedArchive, domain.MobileBackupRequest) {
	t.Helper()
	r, e := selectedFetchRequest().Normalize()
	if e != nil {
		t.Fatal(e)
	}
	since, _ := time.Parse(time.RFC3339Nano, r.Since)
	stamp := since.Add(-24 * time.Hour).UnixMilli()
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		// An unrelated normal row is intentionally not a control candidate.
		if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "902", "9", "8", "ordinary", stamp, 0, 0, 1, nil); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < count; i++ {
			// Some controls precede the requested period, others follow its first page.
			at := stamp + int64(i)*24*60*60*1000
			if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", "901", fmt.Sprint(100+i), fmt.Sprint(200+i), []byte{255}, at, 0, 36, 3, []byte{255}); err != nil {
				t.Fatal(err)
			}
		}
		if modify != nil {
			modify(db)
		}
	})
	selected := SelectedArchive{requestID: r.RequestID, requestFingerprint: r.Fingerprint(), ref: r.Ref(), File: ArchiveFile{Name: "902.db", Data: data}}
	t.Cleanup(func() { selected.Clear() })
	return selected, r
}

func controlOwnerMapper(counter *int) domain.MobileIdentitySource {
	return identitySourceFunc(func(ctx context.Context, r domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		*counter = *counter + 1
		if len(r.Direct) != 1 || r.Direct[0] != "901" || len(r.Groups) != 0 {
			return nil, errors.New("unexpected synthetic identity request")
		}
		return []domain.MobileIdentityPair{{Plain: "901", Session: "10"}}, nil
	})
}

func TestWholeSourceControlScanIncludesLateAndOutsidePeriodRows(t *testing.T) {
	// Arrange: 52 controls cross both the message interval and a 50-row scan page.
	selected, r := controlScanFixture(t, 52, nil)
	calls := 0
	// Act.
	set, err := ReadOwnDirectRecallSet(context.Background(), selected, r, "10", t.TempDir(), 100, controlOwnerMapper(&calls))
	defer set.Clear()
	// Assert: complete exact target set, one mapping, no private payload returned.
	if err != nil || len(set.Recalls) != 52 || calls != 1 {
		t.Fatal("whole-source scan failed", err, len(set.Recalls), calls)
	}
	if set.Recalls[0].MessageID != "100" || set.Recalls[51].MessageID != "151" || set.Recalls[0].Conversation != r.Ref() || set.Recalls[0].SenderID != "10" {
		t.Fatal("control binding lost")
	}
	since, _ := time.Parse(time.RFC3339Nano, r.Since)
	if !time.UnixMilli(set.Recalls[0].RecordAtMS).Before(since) {
		t.Fatal("outside-period control omitted")
	}
	data, err := json.Marshal(set)
	if err != nil || string(data) != "{}" || !strings.Contains(fmt.Sprintf("%#v", set), "[redacted]") {
		t.Fatal("control set exposed private fields")
	}
	targets := set.Recalls
	set.Clear()
	if targets[0] != (domain.MobileHistoryRecall{}) || len(set.Recalls) != 0 || set.account != "" {
		t.Fatal("control set not cleared")
	}
}

func TestWholeSourceControlScanReturnsNoPrefixOnInvalidSource(t *testing.T) {
	for _, mode := range []string{"type33", "incoming", "status", "duplicate", "invalid-time", "missing-id", "oversized", "wal", "cancelled", "partial-mapping", "foreign-request"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: failure may occur after a complete earlier page of valid controls.
			selected, r := controlScanFixture(t, 52, func(db *sql.DB) {
				query := ""
				switch mode {
				case "type33":
					query = "UPDATE ChatContent SET MsgType=33 WHERE GlbMsgId='151'"
				case "status":
					query = "UPDATE ChatContent SET MsgStatus=1 WHERE GlbMsgId='151'"
				case "incoming":
					query = "UPDATE ChatContent SET SenderId='902' WHERE GlbMsgId='151'"
				case "duplicate":
					query = "UPDATE ChatContent SET GlbMsgId='100' WHERE GlbMsgId='151'"
				case "invalid-time":
					query = "UPDATE ChatContent SET TimeStamp=NULL WHERE GlbMsgId='151'"
				case "missing-id":
					query = "UPDATE ChatContent SET GlbMsgId=NULL WHERE GlbMsgId='151'"
				}
				if query != "" {
					if _, err := db.Exec(query); err != nil {
						t.Fatal(err)
					}
				}
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls, limit := 0, 100
			mapper := controlOwnerMapper(&calls)
			switch mode {
			case "oversized":
				limit = 51
			case "wal":
				selected.File.Data[18] = 2
				selected.File.Data[19] = 2
			case "cancelled":
				cancel()
			case "foreign-request":
				r.ConversationID = "99"
			case "partial-mapping":
				mapper = identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
					return nil, nil
				})
			}
			// Act.
			set, err := ReadOwnDirectRecallSet(ctx, selected, r, "10", t.TempDir(), limit, mapper)
			defer set.Clear()
			// Assert: neither target prefix nor an unbound proof may escape.
			if err == nil || len(set.Recalls) != 0 || set.digest != ([32]byte{}) || set.requestID != "" {
				t.Fatal("invalid source returned proof", err)
			}
			if (mode == "oversized" || mode == "wal" || mode == "foreign-request") && calls != 0 {
				t.Fatal("ineligible source reached mapping")
			}
		})
	}
}

func TestWholeSourceControlScanEmptySetNeedsNoMapping(t *testing.T) {
	// Arrange.
	selected, r := controlScanFixture(t, 0, nil)
	calls := 0
	// Act.
	set, err := ReadOwnDirectRecallSet(context.Background(), selected, r, "10", t.TempDir(), 1, controlOwnerMapper(&calls))
	defer set.Clear()
	// Assert.
	if err != nil || calls != 0 || len(set.Recalls) != 0 || set.digest == ([32]byte{}) || set.requestID != r.RequestID {
		t.Fatal("empty control proof failed", err)
	}
}

func TestWholeSourceControlScanRejectsImageMutationDuringMapping(t *testing.T) {
	// Arrange: the mapper must not be able to rebind proof to different image bytes.
	selected, r := controlScanFixture(t, 1, nil)
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		selected.File.Data[len(selected.File.Data)-1] ^= 1
		return []domain.MobileIdentityPair{{Plain: "901", Session: "10"}}, nil
	})
	// Act.
	set, err := ReadOwnDirectRecallSet(context.Background(), selected, r, "10", t.TempDir(), 1, mapper)
	defer set.Clear()
	// Assert.
	if err == nil || len(set.Recalls) != 0 || set.digest != ([32]byte{}) {
		t.Fatal("image mutation returned proof")
	}
}

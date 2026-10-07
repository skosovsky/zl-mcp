package mobilebackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func snapshotProjectionFixture(t *testing.T, from time.Time) AccountArchive {
	t.Helper()
	metadata := []byte{0, 0, 0, 9, 0, 0, 0, 1, 'x'}
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for i := 1; i <= 9; i++ {
			sender, global, kind, ttl, body := "901", "0", int64(0), int64(0), metadata
			if i == 2 {
				sender = "900"
				global = "2"
			}
			if i == 3 {
				sender = "999"
				global = "3"
			}
			if i == 4 {
				kind = 3
			}
			if i == 5 {
				ttl = 1
			}
			if i == 6 {
				body = []byte{1}
			}
			if i == 7 {
				body = nil
			}
			if i == 8 {
				ttl = -1
			}
			text := fmt.Sprintf("synthetic record %d", i)
			if i == 9 {
				text = ""
			}
			if _, err := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", sender, global, fmt.Sprint(i), text, from.UnixMilli()+int64(i), ttl, kind, 1, body); err != nil {
				t.Fatal(err)
			}
		}
	})
	empty := sqliteFixture(t, backupSQLiteSchema, nil)
	decoded := Format1Archive{Archive: PlainArchive{Files: []ArchiveFile{{Name: "901.db", Data: data}, {Name: "group_901.db", Data: append([]byte(nil), data...)}, {Name: "900.db", Data: empty}, {Name: "group_999.db", Data: append([]byte(nil), empty...)}}}}
	a, err := ownAccountArchive(context.Background(), &decoded, []IdentityPair{{Plain: "901", Session: "12"}, {Plain: "901", Session: "12", Group: true}, {Plain: "900", Session: "10"}, {Plain: "999", Session: "35", Group: true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Clear)
	store, err := NewRetainedArchiveStore(filepath.Join(t.TempDir(), "cache"), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.Save(context.Background(), retainedTestID, "10", a, 0); err != nil {
		t.Fatal(err)
	}
	read, _, err := store.Read(context.Background(), retainedTestID, "10")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(read.Clear)
	return read
}

func TestSnapshotTextProjectionIdentityAuthorsAndGaps(t *testing.T) {
	// Arrange: authenticated complete mapping, tied direct/group IDs and partial payload support.
	from := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	a := snapshotProjectionFixture(t, from)
	ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: "12"}
	// Act: read locally, without any identity-source/network parameter.
	page, err := a.ReadSnapshotPage(context.Background(), retainedTestID, ref, t.TempDir(), from, from.Add(time.Hour), 50, nil, "asc", from.Add(2*time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	defer page.Clear()
	// Assert: only supported unexpired text; unavailable authors are explicit and never group IDs.
	if len(page.Records) != 3 || page.Examined != 9 || page.Rejected != 1 || page.Expired != 1 || page.UnresolvedSenders != 1 || page.UnsupportedMetadataFields != 5 || page.Unsupported["unsupported_content"] != 1 || page.Unsupported["invalid_metadata"] != 1 || page.Unsupported["missing_metadata"] != 1 || page.Unsupported["empty_text_projection"] != 1 {
		t.Fatal("projection lost gaps or rendered unsupported content")
	}
	first, own, unknown := page.Records[0], page.Records[1], page.Records[2]
	if first.ArchiveRowID != "ar:"+retainedTestID+":0:1" || first.GlobalID != "" || first.SenderID != "12" || first.Direction != "incoming" || first.Text != "synthetic record 1" {
		t.Fatal("zero global ID or sender fabricated")
	}
	if own.GlobalID != "2" || own.SenderID != "10" || own.Direction != "outgoing" || unknown.SenderID != "" || unknown.Direction != "unknown" {
		t.Fatal("sender ownership inferred without mapping")
	}
	schema, err := contracts.Compile("archive_snapshot_record", "output")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range page.Records {
		if err = schema.Validate(record.TransportRecord()); err != nil {
			t.Fatal("explicit transport violates schema", err)
		}
		encoded, _ := json.Marshal(record)
		if string(encoded) != "{}" || strings.Contains(fmt.Sprintf("%#v", record), record.Text) {
			t.Fatal("private record implicitly disclosed")
		}
	}
	for _, mode := range []string{"quote-anchor", "fake-global", "invented-author"} {
		view := unknown.TransportRecord()
		switch mode {
		case "quote-anchor":
			view["quote_anchor_eligible"] = true
		case "fake-global":
			view["zalo_message_id"] = "0"
		case "invented-author":
			view["sender_id"] = "35"
			view["direction"] = "incoming"
		}
		if schema.Validate(view) == nil {
			t.Fatal("invalid snapshot semantics admitted by schema")
		}
	}
	group, err := a.ReadSnapshotPage(context.Background(), retainedTestID, domain.ConversationRef{Type: domain.ConversationGroup, ID: "12"}, t.TempDir(), from, from.Add(time.Hour), 50, nil, "asc", from.Add(2*time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	defer group.Clear()
	if group.Records[0].ArchiveRowID == first.ArchiveRowID || group.Records[0].Conversation.Type != domain.ConversationGroup {
		t.Fatal("typed-ID collision lost source file identity")
	}
	if invalid, err := a.ReadSnapshotPage(context.Background(), "00000000-0000-4000-8000-000000000002", ref, t.TempDir(), from, from.Add(time.Hour), 50, nil, "asc", from.UnixMilli()); err == nil {
		invalid.Clear()
		t.Fatal("unauthenticated source label accepted")
	}
}

package mobilebackup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestAccountMetadataRedactionFixedCategoriesAndWholeSourceSelection(t *testing.T) {
	// Arrange: private unknown action/title/URL and supported literal, plus missing/malformed rows.
	payload := append(attachmentField(45, []byte("PRIVATE-ACTION-MARKER")), attachmentField(47, []byte("PRIVATE-TITLE-MARKER"))...)
	payload = append(payload, attachmentField(48, []byte("https://private.example/message"))...)
	rows := []SQLiteRow{{Text: "PRIVATE-TITLE-MARKER", BinNet: attachmentField(6, payload)}, {Text: "present", BinNet: attachmentField(6, attachmentField(45, []byte("rtf")))}, {}, {BinNet: []byte{1}}}
	archive := retainedFixture(t)
	// Act: inspect locally and resolve direct/group identities with the same mapped ID.
	result, e := inspectAccountMetadata(context.Background(), rows)
	direct, e1 := archive.ConversationIndex(context.Background(), domain.ConversationRef{Type: "direct", ID: "12"})
	group, e2 := archive.ConversationIndex(context.Background(), domain.ConversationRef{Type: "group", ID: "12"})
	// Assert: fixed observations only, no raw action/title/URL or fallback to an unrelated file.
	if e != nil || e1 != nil || e2 != nil || direct == group || result.AttachmentCount != 2 || result.ActionClasses["other"] != 1 || result.ActionClasses["rtf"] != 1 || result.InvalidMetadata != 1 || result.MissingMetadata != 1 || result.TitleEqualsText != 1 || len(result.ActionShapes) != 2 {
		t.Fatal("metadata diagnostic mismatch")
	}
	serialized, _ := json.Marshal(result)
	for _, private := range []string{"PRIVATE-ACTION-MARKER", "PRIVATE-TITLE-MARKER", "private.example"} {
		if bytes.Contains(serialized, []byte(private)) {
			t.Fatal("source value exposed")
		}
	}
	if _, e := archive.ConversationIndex(context.Background(), domain.ConversationRef{Type: "direct", ID: "999"}); e == nil {
		t.Fatal("missing identity selected another file")
	}
	// A fixed diagnostic never grants conversion of the unsupported attachment.
	if rows[0].Text != "PRIVATE-TITLE-MARKER" {
		t.Fatal("source changed")
	}
}

func TestAccountMetadataReportsShapeBudgetAndCancellation(t *testing.T) {
	// Arrange: one row can contain more attachment actions than the shape budget.
	var data []byte
	for i := 0; i < 51; i++ {
		data = append(data, attachmentField(6, attachmentField(45, []byte(fmt.Sprintf("private-%d", i))))...)
	}
	// Act.
	result, e := inspectAccountMetadata(context.Background(), []SQLiteRow{{BinNet: data}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled, err := inspectAccountMetadata(ctx, []SQLiteRow{{BinNet: data}})
	// Assert: no silent truncation or partial result after cancellation.
	if e != nil || len(result.ActionShapes) != 50 || !result.ShapesTruncated || result.AttachmentCount != 51 || err == nil || cancelled != nil {
		t.Fatal("diagnostic budget or cancellation failed")
	}
}

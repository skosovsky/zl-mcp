package mobilebackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"unicode/utf8"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// ConversationIndex uses only the authenticated original typed mapping.
func (a AccountArchive) ConversationIndex(ctx context.Context, ref domain.ConversationRef) (int, error) {
	return SelectArchiveIndex(ctx, a.archive, a.pairs, ref)
}

// ConversationRefs returns the authenticated typed file mapping in immutable file order.
func (a AccountArchive) ConversationRefs(ctx context.Context) ([]domain.ConversationRef, error) {
	if ctx == nil || ctx.Err() != nil || a.retainedSourceID == "" {
		return nil, ErrArchive
	}
	byName := map[string]domain.ConversationRef{}
	for _, pair := range a.pairs {
		name := pair.Plain + ".db"
		kind := domain.ConversationDirect
		if pair.Group {
			name = "group_" + name
			kind = domain.ConversationGroup
		}
		byName[name] = domain.ConversationRef{Type: kind, ID: pair.Session}
	}
	result := make([]domain.ConversationRef, 0, len(a.archive.Files))
	for _, file := range a.archive.Files {
		ref, ok := byName[file.Name]
		if !ok {
			return nil, ErrIdentities
		}
		result = append(result, ref)
	}
	return result, nil
}

type AccountActionShape struct {
	SHA256      string `json:"sha256"`
	ByteLength  int    `json:"byte_length"`
	UTF8Valid   bool   `json:"utf8_valid"`
	ContainsNUL bool   `json:"contains_nul"`
}

type AccountMetadataDiagnostics struct {
	RowsWithMetadata      int                  `json:"rows_with_metadata"`
	MissingMetadata       int                  `json:"missing_metadata"`
	InvalidMetadata       int                  `json:"invalid_metadata"`
	AttachmentCount       int                  `json:"attachment_count"`
	UnsupportedFields     int                  `json:"unsupported_metadata_fields"`
	SourceTextPresent     int                  `json:"source_text_present"`
	TitlePresent          int                  `json:"title_present"`
	TitleEqualsText       int                  `json:"title_equals_source_text"`
	TextProjectionClasses map[string]int       `json:"text_projection_classes"`
	ActionClasses         map[string]int       `json:"action_classes"`
	ActionShapes          []AccountActionShape `json:"action_shapes"`
	ShapesTruncated       bool                 `json:"action_shapes_truncated"`
}

// inspectAccountMetadata reports protocol observations only. It never renders
// source text or accepts an attachment action, and retains no raw metadata.
func inspectAccountMetadata(ctx context.Context, rows []SQLiteRow) (*AccountMetadataDiagnostics, error) {
	result := &AccountMetadataDiagnostics{ActionClasses: map[string]int{}, TextProjectionClasses: map[string]int{}, ActionShapes: []AccountActionShape{}}
	seen := map[string]bool{}
	for _, row := range rows {
		if ctx.Err() != nil {
			return nil, ErrArchive
		}
		if row.Text != "" {
			result.SourceTextPresent++
		}
		if len(row.BinNet) == 0 {
			result.MissingMetadata++
			result.TextProjectionClasses["missing_metadata"]++
			continue
		}
		result.RowsWithMetadata++
		meta, err := ParseBinNet(ctx, row.BinNet)
		if ctx.Err() != nil {
			meta.Clear()
			return nil, ErrArchive
		}
		if err != nil {
			meta.Clear()
			result.InvalidMetadata++
			result.TextProjectionClasses["invalid"]++
			continue
		}
		kind, _ := mobilePayloadKind(row.Type)
		_, rich, supported, valid := archiveText(PreparedRow{Row: row, Kind: kind, Metadata: meta})
		projection := "unsupported"
		if !valid {
			projection = "invalid"
		} else if supported && rich {
			projection = "rich"
		} else if supported {
			projection = "plain"
		}
		result.TextProjectionClasses[projection]++
		result.UnsupportedFields += meta.UnsupportedFields
		for _, attachment := range meta.Attachments {
			result.AttachmentCount++
			if attachment.Title.Present {
				result.TitlePresent++
				if string(attachment.Title.Bytes) == row.Text {
					result.TitleEqualsText++
				}
			}
			class := "absent"
			if attachment.Action.Present {
				class = "other"
				switch string(attachment.Action.Bytes) {
				case "":
					class = "empty"
				case "rtf", "ecard", "text", "plain", "webchat":
					class = string(attachment.Action.Bytes)
				}
				digest := sha256.Sum256(attachment.Action.Bytes)
				id := hex.EncodeToString(digest[:])
				if !seen[id] && len(result.ActionShapes) >= 50 {
					result.ShapesTruncated = true
				}
				if !seen[id] && len(result.ActionShapes) < 50 {
					seen[id] = true
					result.ActionShapes = append(result.ActionShapes, AccountActionShape{SHA256: id, ByteLength: len(attachment.Action.Bytes), UTF8Valid: utf8.Valid(attachment.Action.Bytes), ContainsNUL: bytes.IndexByte(attachment.Action.Bytes, 0) >= 0})
				}
			}
			result.ActionClasses[class]++
		}
		meta.Clear()
	}
	return result, nil
}

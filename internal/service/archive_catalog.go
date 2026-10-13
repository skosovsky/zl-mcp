package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type archiveCatalogueCursor struct {
	Version int    `json:"v"`
	Kind    string `json:"kind"`
	Account string `json:"account"`
	Source  string `json:"source"`
	Digest  string `json:"digest"`
	Type    string `json:"type"`
	Query   string `json:"query"`
	Next    int    `json:"next"`
	Members []byte `json:"members"`
}

func (archiveCatalogueCursor) String() string   { return "archive catalogue cursor [redacted]" }
func (archiveCatalogueCursor) GoString() string { return "archive catalogue cursor [redacted]" }

func archiveString(args map[string]any, name string) string {
	value, _ := args[name].(string)
	return value
}
func archiveLimit(args map[string]any) int {
	value, ok := args["limit"].(float64)
	if !ok {
		return defaultArchiveCatalogueLimit
	}
	return int(value)
}
func archiveInvalidCursor() error {
	return domain.Invalid("Archive cursor does not match this source, account or filter.")
}

func (p *membershipPort) archiveConversations(
	ctx context.Context,
	args map[string]any,
	binding string,
) (map[string]any, error) {
	sourceID := archiveString(args, fieldSourceID)
	source, manifest, err := p.library.ReadBound(ctx, sourceID, binding)
	if err != nil {
		return nil, archiveReadUnavailable()
	}
	defer source.Clear()
	receipt, err := p.library.PreservationStatus(ctx, sourceID, binding)
	if err != nil {
		return nil, archiveReadUnavailable()
	}
	refs, err := source.ConversationRefs(ctx)
	if err != nil {
		return nil, archiveReadUnavailable()
	}
	kind, query := archiveString(
		args,
		"conversation_type",
	), strings.ToLower(
		strings.TrimSpace(archiveString(args, "query")),
	)
	//nolint:exhaustruct_v5 // archiveCatalogueCursor initializes only this owner/phase; optional collaborators, buffers and counters keep their zero defaults.
	cursor := archiveCatalogueCursor{
		Version: 1,
		Kind:    "catalogue",
		Account: binding,
		Source:  sourceID,
		Digest:  manifest.Digest,
		Type:    kind,
		Query:   query,
		Members: make([]byte, (len(refs)+membershipBitRoundUp)/membershipByteBits),
	}
	token := archiveString(args, "cursor")
	cursor, err = p.bindCatalogueCursor(ctx, cursor, token, refs, manifest.CapturedAt)
	if err != nil {
		return nil, err
	}
	values := []map[string]any{}
	next := len(refs)
	for index := cursor.Next; index < len(refs); index++ {
		if cursor.Members[index/membershipByteBits]&(1<<uint(index%membershipByteBits)) == 0 ||
			!p.store.AllowsConversation(refs[index]) {
			continue
		}
		if len(values) == archiveLimit(args) {
			next = index
			break
		}
		item, err := p.archiveConversation(ctx, refs[index], manifest.CapturedAt)
		if err != nil {
			return nil, err
		}
		values = append(values, item)
	}
	var nextToken any
	if next < len(refs) {
		cursor.Next = next
		body, err := json.Marshal(cursor)
		if err != nil {
			return nil, archiveInvalidCursor()
		}
		defer clear(body)
		sealed, err := p.library.SealArchiveToken(ctx, "cursor", body)
		if err != nil {
			return nil, archiveInvalidCursor()
		}
		nextToken = sealed
	}
	var emptyReason any
	if len(values) == 0 {
		emptyReason = "no_matching_conversations"
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return map[string]any{
		"conversations":    values,
		fieldHasMore:       next < len(refs),
		"next_cursor":      nextToken,
		"catalog_complete": false,
		"catalog_sources":  []string{fieldRetainedArchive},
		"empty_reason":     emptyReason,
		"source":           archiveDescriptor(receipt),
	}, nil
}

func (p *membershipPort) archiveConversation(
	ctx context.Context,
	ref domain.ConversationRef,
	captured string,
) (map[string]any, error) {
	result, err := p.store.Conversation(ctx, ref)
	if err == nil {
		conversation, ok := result["conversation"].(map[string]any)
		if !ok {
			return nil, errors.New("invalid conversation projection")
		}
		return conversation, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return map[string]any{
		"conversation_type":   ref.Type,
		"conversation_id":     ref.ID,
		"name":                nil,
		"metadata_source":     fieldRetainedArchive,
		"availability":        stateUnknownText,
		"first_discovered_at": captured,
		"updated_at":          captured,
		"collection_enabled":  true,
		"aliases":             []string{},
		"friendship":          stateUnknownText,
		"has_stored_messages": false,
		"metadata_sources":    []string{fieldRetainedArchive},
	}, nil
}

func archiveNameMatches(item map[string]any, query string) bool {
	if query == "" {
		return true
	}
	var name string
	switch value := item["name"].(type) {
	case string:
		name = value
	case *string:
		if value != nil {
			name = *value
		}
	}
	if strings.Contains(strings.ToLower(name), query) {
		return true
	}
	aliases, _ := item["aliases"].([]string)
	for _, alias := range aliases {
		if strings.Contains(strings.ToLower(alias), query) {
			return true
		}
	}
	return false
}

func (p *membershipPort) bindCatalogueCursor(
	ctx context.Context,
	cursor archiveCatalogueCursor,
	token string,
	refs []domain.ConversationRef,
	captured string,
) (archiveCatalogueCursor, error) {
	if token == "" {
		for index, ref := range refs {
			if !p.store.AllowsConversation(ref) || cursor.Type != "" && cursor.Type != ref.Type {
				continue
			}
			item, err := p.archiveConversation(ctx, ref, captured)
			if err != nil {
				return archiveCatalogueCursor{}, err
			}
			if archiveNameMatches(item, cursor.Query) {
				cursor.Members[index/membershipByteBits] |= 1 << uint(index%membershipByteBits)
			}
		}

		return cursor, nil
	}
	body, err := p.library.OpenArchiveToken(ctx, "cursor", token)
	if err != nil {
		return archiveCatalogueCursor{}, archiveInvalidCursor()
	}
	defer clear(body)
	var saved archiveCatalogueCursor
	if json.Unmarshal(body, &saved) != nil || saved.Version != cursor.Version || saved.Kind != cursor.Kind ||
		saved.Account != cursor.Account ||
		saved.Source != cursor.Source ||
		saved.Digest != cursor.Digest ||
		saved.Type != cursor.Type ||
		saved.Query != cursor.Query ||
		saved.Next < 0 ||
		saved.Next >= len(refs) ||
		len(saved.Members) != len(cursor.Members) {
		return archiveCatalogueCursor{}, archiveInvalidCursor()
	}
	cursor = saved
	return cursor, nil
}

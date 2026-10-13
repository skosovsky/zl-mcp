package zalo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type conversationPreloadReader interface {
	GetConversationPreload(context.Context) (*api.ConversationPreloadPage, error)
}

func (c *Client) ConversationPreload(ctx context.Context) (domain.PreloadSnapshot, error) {
	source, ok := c.api.(conversationPreloadReader)
	if !ok {
		return domain.PreloadSnapshot{}, domain.ErrConversationPreloadUnsupported
	}
	return readConversationPreload(ctx, source, c.AccountID())
}

func readConversationPreload(
	ctx context.Context,
	source conversationPreloadReader,
	own string,
) (domain.PreloadSnapshot, error) {
	if own == "" || own == "0" {
		return domain.PreloadSnapshot{}, invalidHistoryPage("missing preload account binding")
	}
	raw, err := source.GetConversationPreload(ctx)
	if err != nil {
		if errors.Is(err, api.ErrConversationPreloadUnavailable) {
			return domain.PreloadSnapshot{}, domain.ErrConversationPreloadUnsupported
		}
		if errors.Is(err, errs.ErrAuthenticationRequired) {
			return domain.PreloadSnapshot{}, domain.ErrAuthenticationRequired
		}
		return domain.PreloadSnapshot{}, safeHistorySourceFailure(err)
	}
	if boundsErr := validatePreloadBounds(raw); boundsErr != nil {
		return domain.PreloadSnapshot{}, boundsErr
	}
	result := domain.PreloadSnapshot{
		Entries:                 []domain.PreloadEntry{},
		Messages:                []domain.Message{},
		DirectMessagesAvailable: raw.DirectMessages != nil,
		GroupMessagesAvailable:  raw.GroupMessages != nil,
		UnsupportedMessageCount: raw.UnsupportedMessageCount,
	}
	seen := map[domain.ConversationRef]bool{}
	for _, item := range raw.Metadata {
		entry, entryErr := parsePreloadEntry(item, own, seen)
		if entryErr != nil {
			return domain.PreloadSnapshot{}, entryErr
		}
		result.Entries = append(result.Entries, entry)
	}
	for _, batch := range []struct {
		kind    string
		records []json.RawMessage
	}{{domain.ConversationDirect, raw.DirectMessages}, {domain.ConversationGroup, raw.GroupMessages}} {
		for _, item := range batch.records {
			msg, messageErr := parsePreloadMessage(item, batch.kind, own)
			if messageErr != nil {
				return domain.PreloadSnapshot{}, messageErr
			}
			result.Messages = append(result.Messages, msg)
		}
	}
	return result, nil
}

func preloadID(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	var value string
	if len(raw) > 0 && raw[0] == '"' {
		if json.Unmarshal(raw, &value) != nil {
			return "", invalidHistoryPage("invalid preload identifier")
		}
	} else {
		value = string(raw)
		if !historyNumber(value) {
			return "", invalidHistoryPage("invalid preload numeric identifier")
		}
	}
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return "", invalidHistoryPage("invalid preload identifier")
	}
	return value, nil
}

func normalizePreloadMessage(raw json.RawMessage) ([]byte, error) {
	if len(raw) > 1<<20 {
		return nil, invalidHistoryPage("preload record exceeds bounds")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, invalidHistoryPage("invalid preload message shape")
	}
	for _, key := range []string{nativeActionIDField, "msgId", nativeClientMessageIDField, "uidFrom", "idTo", "ts", nativeUserIDField, nativeRealMessageIDField} {
		value := bytes.TrimSpace(fields[key])
		if len(value) == 0 || bytes.Equal(value, []byte("null")) {
			continue
		}
		// Optional SDK fields may be absent or an empty string. They are not
		// message/peer identity and must not reject an otherwise valid record.
		if bytes.Equal(value, []byte(`""`)) &&
			(key == nativeActionIDField || key == nativeClientMessageIDField || key == nativeUserIDField || key == nativeRealMessageIDField) {
			continue
		}
		id, err := preloadID(value)
		if err != nil {
			return nil, invalidHistoryPage("invalid preload message identifier " + key)
		}
		fields[key], _ = json.Marshal(id)
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, invalidHistoryPage("invalid preload message")
	}
	return data, nil
}

func validatePreloadBounds(raw *api.ConversationPreloadPage) error {
	if raw == nil || raw.Metadata == nil || len(raw.Metadata) > 5000 || raw.UnsupportedMessageCount < 0 ||
		len(raw.DirectMessages)+len(raw.GroupMessages)+raw.UnsupportedMessageCount > 1000 {
		return invalidHistoryPage("invalid preload page bounds")
	}
	totalBytes := 0
	for _, records := range [][]json.RawMessage{raw.Metadata, raw.DirectMessages, raw.GroupMessages} {
		for _, record := range records {
			totalBytes += len(record)
			if totalBytes > 8<<20 {
				return invalidHistoryPage("preload normalization exceeds bounds")
			}
		}
	}
	return nil
}

func parsePreloadEntry(
	item json.RawMessage,
	own string,
	seen map[domain.ConversationRef]bool,
) (domain.PreloadEntry, error) {
	var wire struct {
		ID    json.RawMessage `json:"idTo"`
		Group json.RawMessage `json:"isGroup"`
		Name  *string         `json:"userName"`
		Last  json.RawMessage `json:"lastMsgId"`
	}
	if json.Unmarshal(item, &wire) != nil {
		return domain.PreloadEntry{}, invalidHistoryPage("invalid preload metadata")
	}
	id, err := preloadID(wire.ID)
	if err != nil || id == own || id == "0" || (wire.Name != nil && len(*wire.Name) > 4096) {
		return domain.PreloadEntry{}, invalidHistoryPage("invalid preload dialogue identity")
	}
	kind := domain.ConversationDirect
	switch string(bytes.TrimSpace(wire.Group)) {
	case "0":
	case "1":
		kind = domain.ConversationGroup
	default:
		return domain.PreloadEntry{}, invalidHistoryPage("unknown preload dialogue classification")
	}
	ref := domain.ConversationRef{Type: kind, ID: id}
	if seen[ref] {
		return domain.PreloadEntry{}, invalidHistoryPage("duplicate preload dialogue identity")
	}
	seen[ref] = true
	var last *string
	if len(wire.Last) > 0 && string(wire.Last) != "null" {
		v, e := preloadID(wire.Last)
		if e != nil {
			return domain.PreloadEntry{}, invalidHistoryPage("invalid preload last message identifier")
		}
		last = &v
	}
	return domain.PreloadEntry{Conversation: ref, Name: wire.Name, LastMessageID: last}, nil
}

func parsePreloadMessage(item json.RawMessage, kind, own string) (domain.Message, error) {
	normalized, err := normalizePreloadMessage(item)
	if err != nil {
		return domain.Message{}, err
	}
	var wire model.TGroupMessage
	if json.Unmarshal(normalized, &wire) != nil {
		return domain.Message{}, invalidHistoryPage("invalid preload message fields")
	}
	sender, to := wire.UIDFrom, wire.IDTo
	outgoing := sender == own || sender == "0"
	if sender == "0" {
		wire.UIDFrom = own
	}
	id, peerErr := preloadPeer(kind, own, sender, to, outgoing)
	if peerErr != nil {
		return domain.Message{}, peerErr
	}
	msg, e := convertMessage(wire.TMessage, domain.ConversationRef{Type: kind, ID: id}, "")
	if e != nil || len(msg.Text) > 1<<20 {
		return domain.Message{}, invalidHistoryPage("invalid preload message metadata")
	}
	msg.Direction = directionIncoming
	if outgoing {
		msg.Direction = directionOutgoing
	}
	if kind == domain.ConversationDirect && wire.Content.String != nil && wire.MsgType == "webchat" &&
		wire.CliMsgID != "" {
		msg.QuoteMetadata = &domain.QuoteMetadata{
			ClientMessageID: wire.CliMsgID,
			MessageType:     wire.MsgType,
			Timestamp:       wire.TS,
			TTL:             wire.TTL,
		}
	}
	return msg, nil
}

func preloadPeer(kind, own, sender, to string, outgoing bool) (string, error) {
	id := to
	if kind == domain.ConversationDirect {
		var peerErr error
		id, peerErr = directPreloadPeer(own, sender, to, outgoing)
		if peerErr != nil {
			return "", peerErr
		}
	}
	if id == "" || id == "0" || id == own || len(id) > 256 {
		return "", invalidHistoryPage("invalid preload message conversation")
	}
	return id, nil
}

func directPreloadPeer(own, sender, to string, outgoing bool) (string, error) {
	id := to

	if outgoing {
		if to == "" || to == "0" || to == own {
			return "", invalidHistoryPage("ambiguous preload outgoing peer")
		}
	} else {
		if to != own && to != "0" {
			return "", invalidHistoryPage(
				"preload message belongs to a foreign account",
			)
		}
		id = sender
	}

	return id, nil
}

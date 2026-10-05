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

func readConversationPreload(ctx context.Context, source conversationPreloadReader, own string) (domain.PreloadSnapshot, error) {
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
	if raw == nil || raw.Metadata == nil || len(raw.Metadata) > 5000 || raw.UnsupportedMessageCount < 0 || len(raw.DirectMessages)+len(raw.GroupMessages)+raw.UnsupportedMessageCount > 1000 {
		return domain.PreloadSnapshot{}, invalidHistoryPage("invalid preload page bounds")
	}
	totalBytes := 0
	for _, records := range [][]json.RawMessage{raw.Metadata, raw.DirectMessages, raw.GroupMessages} {
		for _, record := range records {
			totalBytes += len(record)
			if totalBytes > 8<<20 {
				return domain.PreloadSnapshot{}, invalidHistoryPage("preload normalization exceeds bounds")
			}
		}
	}
	result := domain.PreloadSnapshot{Entries: []domain.PreloadEntry{}, Messages: []domain.Message{}, DirectMessagesAvailable: raw.DirectMessages != nil, GroupMessagesAvailable: raw.GroupMessages != nil, UnsupportedMessageCount: raw.UnsupportedMessageCount}
	seen := map[domain.ConversationRef]bool{}
	for _, item := range raw.Metadata {
		var wire struct {
			ID    json.RawMessage `json:"idTo"`
			Group json.RawMessage `json:"isGroup"`
			Name  *string         `json:"userName"`
			Last  json.RawMessage `json:"lastMsgId"`
		}
		if json.Unmarshal(item, &wire) != nil {
			return domain.PreloadSnapshot{}, invalidHistoryPage("invalid preload metadata")
		}
		id, err := preloadID(wire.ID)
		if err != nil || id == own || id == "0" || (wire.Name != nil && len(*wire.Name) > 4096) {
			return domain.PreloadSnapshot{}, invalidHistoryPage("invalid preload dialogue identity")
		}
		kind := domain.ConversationDirect
		switch string(bytes.TrimSpace(wire.Group)) {
		case "0":
		case "1":
			kind = domain.ConversationGroup
		default:
			return domain.PreloadSnapshot{}, invalidHistoryPage("unknown preload dialogue classification")
		}
		ref := domain.ConversationRef{Type: kind, ID: id}
		if seen[ref] {
			return domain.PreloadSnapshot{}, invalidHistoryPage("duplicate preload dialogue identity")
		}
		seen[ref] = true
		var last *string
		if len(wire.Last) > 0 && string(wire.Last) != "null" {
			v, e := preloadID(wire.Last)
			if e != nil {
				return domain.PreloadSnapshot{}, invalidHistoryPage("invalid preload last message identifier")
			}
			last = &v
		}
		result.Entries = append(result.Entries, domain.PreloadEntry{Conversation: ref, Name: wire.Name, LastMessageID: last})
	}
	for _, batch := range []struct {
		kind    string
		records []json.RawMessage
	}{{domain.ConversationDirect, raw.DirectMessages}, {domain.ConversationGroup, raw.GroupMessages}} {
		for _, item := range batch.records {
			normalized, err := normalizePreloadMessage(item)
			if err != nil {
				return domain.PreloadSnapshot{}, err
			}
			var wire model.TGroupMessage
			if json.Unmarshal(normalized, &wire) != nil {
				return domain.PreloadSnapshot{}, invalidHistoryPage("invalid preload message fields")
			}
			sender, to := wire.UIDFrom, wire.IDTo
			outgoing := sender == own || sender == "0"
			if sender == "0" {
				wire.UIDFrom = own
			}
			id := to
			if batch.kind == domain.ConversationDirect {
				if outgoing {
					if to == "" || to == "0" || to == own {
						return domain.PreloadSnapshot{}, invalidHistoryPage("ambiguous preload outgoing peer")
					}
				} else {
					if to != own && to != "0" {
						return domain.PreloadSnapshot{}, invalidHistoryPage("preload message belongs to a foreign account")
					}
					id = sender
				}
			}
			if id == "" || id == "0" || id == own || len(id) > 256 {
				return domain.PreloadSnapshot{}, invalidHistoryPage("invalid preload message conversation")
			}
			msg, e := convertMessage(wire.TMessage, domain.ConversationRef{Type: batch.kind, ID: id}, "")
			if e != nil || len(msg.Text) > 1<<20 {
				return domain.PreloadSnapshot{}, invalidHistoryPage("invalid preload message metadata")
			}
			msg.Direction = "incoming"
			if outgoing {
				msg.Direction = "outgoing"
			}
			if batch.kind == domain.ConversationDirect && wire.Content.String != nil && wire.MsgType == "webchat" && wire.CliMsgID != "" {
				msg.QuoteMetadata = &domain.QuoteMetadata{ClientMessageID: wire.CliMsgID, MessageType: wire.MsgType, Timestamp: wire.TS, TTL: wire.TTL}
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
	for _, key := range []string{"actionId", "msgId", "cliMsgId", "uidFrom", "idTo", "ts", "userId", "realMsgId"} {
		value := bytes.TrimSpace(fields[key])
		if len(value) == 0 || bytes.Equal(value, []byte("null")) {
			continue
		}
		// Optional SDK fields may be absent or an empty string. They are not
		// message/peer identity and must not reject an otherwise valid record.
		if bytes.Equal(value, []byte(`""`)) && (key == "actionId" || key == "cliMsgId" || key == "userId" || key == "realMsgId") {
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

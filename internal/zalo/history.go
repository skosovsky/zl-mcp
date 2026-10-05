package zalo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/model"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type groupHistoryReader interface {
	GetGroupHistoryPage(context.Context, string, string, int) (*api.GroupHistoryPage, error)
}

type phasedGroupHistoryReader interface {
	GetGroupHistoryPageWithPhase(context.Context, string, string, bool, int) (*api.GroupHistoryPage, error)
}

func (c *Client) HistoryPhaseRequired() bool { return true }

type groupHistoryReadFunc func(context.Context, string, string, int) (*api.GroupHistoryPage, error)

func (f groupHistoryReadFunc) GetGroupHistoryPage(ctx context.Context, id, cursor string, limit int) (*api.GroupHistoryPage, error) {
	return f(ctx, id, cursor, limit)
}

func (c *Client) HistoryPageWithPhase(ctx context.Context, ref domain.ConversationRef, cursor string, old *bool, limit int) (domain.HistoryPage, error) {
	if !ref.Valid() || len(ref.ID) > 256 || limit < 1 || limit > 50 || !historyNumber(cursor) {
		return domain.HistoryPage{}, domain.Invalid("Invalid history page request.")
	}
	if ref.Type != domain.ConversationGroup {
		return domain.HistoryPage{}, domain.ErrHistoryUnsupported
	}
	reader, ok := c.api.(phasedGroupHistoryReader)
	if !ok {
		return domain.HistoryPage{}, domain.ErrHistoryUnsupported
	}
	page, err := readGroupHistoryPage(ctx, groupHistoryReadFunc(func(ctx context.Context, id, cursor string, count int) (*api.GroupHistoryPage, error) {
		return reader.GetGroupHistoryPageWithPhase(ctx, id, cursor, old != nil && *old, count)
	}), c.AccountID(), ref, cursor, limit)
	page.PhaseRequired = true
	return page, err
}

func (c *Client) HistoryPage(ctx context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
	if !ref.Valid() || len(ref.ID) > 256 || limit < 1 || limit > 50 || !historyNumber(cursor) {
		return domain.HistoryPage{}, domain.Invalid("Invalid history page request.")
	}
	if ref.Type != domain.ConversationGroup {
		return domain.HistoryPage{}, domain.ErrHistoryUnsupported
	}
	reader, ok := c.api.(groupHistoryReader)
	if !ok {
		return domain.HistoryPage{}, domain.ErrHistoryUnsupported
	}
	return readGroupHistoryPage(ctx, reader, c.AccountID(), ref, cursor, limit)
}

func readGroupHistoryPage(ctx context.Context, reader groupHistoryReader, own string, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
	raw, err := reader.GetGroupHistoryPage(ctx, ref.ID, cursor, limit)
	if err != nil {
		var malformed *api.GroupHistoryPageDecodeError
		if errors.As(err, &malformed) {
			return domain.HistoryPage{}, invalidHistoryPage("invalid group history " + malformed.Field)
		}
		if errors.Is(err, api.ErrGroupHistorySourceUnavailable) {
			return domain.HistoryPage{}, domain.ErrHistoryUnsupported
		}
		if errors.Is(err, errs.ErrAuthenticationRequired) {
			return domain.HistoryPage{}, domain.ErrAuthenticationRequired
		}

		return domain.HistoryPage{}, safeHistorySourceFailure(err)
	}
	if raw == nil || len(raw.Records) > limit {
		return domain.HistoryPage{}, invalidHistoryPage("invalid history page records")
	}
	if raw.InternalError != nil && *raw.InternalError != 0 {
		return domain.HistoryPage{}, invalidHistoryPage("history source returned internal error")
	}
	joined, err := optionalHistoryDecimal(raw.JoinTimestamp)
	if err != nil {
		return domain.HistoryPage{}, invalidHistoryPage("invalid history join timestamp")
	}
	page := domain.HistoryPage{Messages: []domain.Message{}, HasMore: raw.HasMore, Cursor: raw.LastMessageID, IsFiltered: raw.IsFiltered, IsFilteredByPhase: raw.IsFilteredByPhase, IsFilteredByTimeJoin: raw.IsFilteredByTimeJoin, IsOld: raw.IsOld}
	page.JoinTimestampMillis = joined
	total := 0
	for _, record := range raw.Records {
		total += len(record)
		if total > 1<<20 {
			return domain.HistoryPage{}, invalidHistoryPage("history page exceeds normalization budget")
		}
		msg, err := decodeHistoryRecord(record, own, ref)
		if err != nil {
			return domain.HistoryPage{}, err
		}
		page.Messages = append(page.Messages, msg)
	}
	return page, nil
}

func optionalHistoryDecimal(raw json.RawMessage) (*string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	value := string(raw)
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
	}
	if !historyNumber(value) {
		return nil, invalidHistoryPage("invalid decimal metadata")
	}
	return &value, nil
}

func historyNumber(value string) bool {
	if len(value) == 0 || len(value) > 128 || (len(value) > 1 && value[0] == '0') {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func decodeHistoryRecord(raw []byte, own string, ref domain.ConversationRef) (domain.Message, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return domain.Message{}, invalidHistoryPage("invalid history message shape")
	}
	// Convert only identifier/time fields whose pinned model is string based.
	for _, key := range []string{"actionId", "msgId", "cliMsgId", "uidFrom", "idTo", "ts", "userId", "realMsgId"} {
		value := bytes.TrimSpace(fields[key])
		if len(value) == 0 || bytes.Equal(value, []byte("null")) || value[0] == '"' {
			continue
		}
		if !historyNumber(string(value)) {
			return domain.Message{}, invalidHistoryPage("invalid history numeric identifier")
		}
		fields[key], _ = json.Marshal(string(value))
	}
	normalized, err := json.Marshal(fields)
	if err != nil {
		return domain.Message{}, invalidHistoryPage("invalid history message")
	}
	var wire model.TGroupMessage
	if json.Unmarshal(normalized, &wire) != nil {
		return domain.Message{}, invalidHistoryPage("unsupported history message fields")
	}
	if wire.IDTo != ref.ID {
		return domain.Message{}, invalidHistoryPage("history record does not match requested conversation")
	}
	msg, err := Convert(model.NewGroupMessage(own, wire), "")
	if err != nil {
		return domain.Message{}, invalidHistoryPage("unsupported history message metadata")
	}
	if len(msg.Text) > 1<<20 {
		return domain.Message{}, invalidHistoryPage("history text exceeds supported size")
	}
	if msg.SenderID == own {
		msg.Direction = "outgoing"
	}
	return msg, nil
}

func invalidHistoryPage(message string) error {
	return domain.NewHistoryPageFailure(message)
}

func safeHistorySourceFailure(err error) error {
	var value errs.ZaloAPIError
	var pointer *errs.ZaloAPIError
	var code *errs.ZaloErrorCode
	if errors.As(err, &value) {
		code = value.Code
	} else if errors.As(err, &pointer) && pointer != nil {
		code = pointer.Code
	} else {
		return err
	}
	var safeCode *int
	if code != nil {
		value := int(*code)
		safeCode = &value
	}
	return domain.NewHistorySourceFailure(safeCode, err)
}

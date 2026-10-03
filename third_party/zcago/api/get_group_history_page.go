package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/amrakk/zcago/internal/httpx"
	"github.com/amrakk/zcago/internal/jsonx"
	"github.com/amrakk/zcago/session"
)

var ErrGroupHistorySourceUnavailable = errors.New("group cloud history source unavailable")

// GroupHistoryPage is raw protocol evidence. Reading it does not import messages.
// Nullable fields distinguish missing metadata from false or zero.
type GroupHistoryPage struct {
	Records              []json.RawMessage
	HasMore              *bool
	LastMessageID        *string
	IsFiltered           *bool
	IsFilteredByPhase    *bool
	IsFilteredByTimeJoin *bool
	IsOld                *bool
	JoinTimestamp        json.RawMessage
	InternalError        *int
}

func historyDecimal(raw json.RawMessage) (*string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	value := string(raw)
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("invalid history cursor")
		}
	}
	if !validHistoryCursor(value) {
		return nil, fmt.Errorf("invalid history cursor")
	}
	return &value, nil
}
func validHistoryCursor(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	// JSON numeric request cursors must not have leading zeros.
	return len(value) == 1 || value[0] != '0'
}

func (p *GroupHistoryPage) UnmarshalJSON(raw []byte) error {
	*p = GroupHistoryPage{}
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return err
		}
		raw = []byte(encoded)
	}
	var wire struct {
		Messages             *[]json.RawMessage `json:"groupMsgs"`
		HasMore              *bool              `json:"hasMore"`
		LastID               json.RawMessage    `json:"lastMsgId"`
		IsFiltered           *bool              `json:"isFiltered"`
		IsFilteredByPhase    *bool              `json:"isFilteredByPhase"`
		IsFilteredByTimeJoin *bool              `json:"isFilteredByTimeJoin"`
		IsOld                *bool              `json:"isOld"`
		JoinTimestamp        json.RawMessage    `json:"tsJoinGroup"`
		Error                *int               `json:"error"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return fmt.Errorf("invalid history response metadata")
	}
	if wire.Messages == nil || len(*wire.Messages) > 50 {
		return fmt.Errorf("missing or oversized history records")
	}
	cursor, err := historyDecimal(wire.LastID)
	if err != nil {
		return err
	}
	*p = GroupHistoryPage{Records: *wire.Messages, HasMore: wire.HasMore, LastMessageID: cursor, IsFiltered: wire.IsFiltered, IsFilteredByPhase: wire.IsFilteredByPhase, IsFilteredByTimeJoin: wire.IsFilteredByTimeJoin, IsOld: wire.IsOld, JoinTimestamp: wire.JoinTimestamp, InternalError: wire.Error}
	return nil
}

type GroupHistoryPageFn func(context.Context, string, string, int) (*GroupHistoryPage, error)

// GetGroupHistoryPage is an optional concrete API extension. The SDK's existing
// public API interface remains compatible. It uses the already authenticated API.
func (a *api) GetGroupHistoryPage(ctx context.Context, groupID, cursor string, count int) (*GroupHistoryPage, error) {
	fn, err := groupHistoryPageFactory(a.sc, a)
	if err != nil {
		return nil, err
	}
	return fn(ctx, groupID, cursor, count)
}

var groupHistoryPageFactory = apiFactory[*GroupHistoryPage, GroupHistoryPageFn]()(
	func(a *api, sc session.Context, u factoryUtils[*GroupHistoryPage]) (GroupHistoryPageFn, error) {
		base := jsonx.FirstOr(sc.GetZpwService("group_cloud_message"), "")
		if base == "" {
			return nil, ErrGroupHistorySourceUnavailable
		}
		serviceURL := u.MakeURL(strings.TrimSuffix(base, "/")+"/api/cm/getrecentv2", nil, true)
		return func(ctx context.Context, groupID, cursor string, count int) (*GroupHistoryPage, error) {
			if groupID == "" || len(groupID) > 256 || !validHistoryCursor(cursor) || count < 1 || count > 50 {
				return nil, fmt.Errorf("invalid group history page request")
			}
			payload := map[string]any{"groupId": groupID, "globalMsgId": json.Number(cursor), "count": count, "msgIds": []string{}, "imei": sc.IMEI(), "src": 3}
			encrypted, err := u.EncodeAES(jsonx.Stringify(payload))
			if err != nil {
				return nil, fmt.Errorf("history request encoding failed")
			}
			url := u.MakeURL(serviceURL, map[string]any{"params": encrypted, "nretry": 0}, true)
			response, err := u.Request(ctx, url, &httpx.RequestOptions{Method: http.MethodGet})
			if err != nil {
				return nil, err
			}
			defer response.Body.Close()
			result, err := u.Resolve(response, true)
			if err != nil {
				return nil, err
			}
			if result == nil || len(result.Records) > count {
				return nil, fmt.Errorf("invalid group history page size")
			}
			if result.InternalError != nil && *result.InternalError != 0 {
				return nil, fmt.Errorf("group history returned internal error")
			}
			return result, nil
		}, nil
	},
)

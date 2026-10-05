package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/amrakk/zcago/internal/httpx"
	"github.com/amrakk/zcago/internal/jsonx"
	"github.com/amrakk/zcago/session"
)

type GroupHistoryPageDecodeError struct{ Field string }

func (e *GroupHistoryPageDecodeError) Error() string { return "invalid group history page" }

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
		HasMore              json.RawMessage    `json:"hasMore"`
		LastID               json.RawMessage    `json:"lastMsgId"`
		IsFiltered           json.RawMessage    `json:"isFiltered"`
		IsFilteredByPhase    json.RawMessage    `json:"isFilteredByPhase"`
		IsFilteredByTimeJoin json.RawMessage    `json:"isFilteredByTimeJoin"`
		IsOld                json.RawMessage    `json:"isOld"`
		JoinTimestamp        json.RawMessage    `json:"tsJoinGroup"`
		Error                *int               `json:"error"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return &GroupHistoryPageDecodeError{Field: "shape"}
	}
	if wire.Messages == nil || len(*wire.Messages) > 50 {
		return &GroupHistoryPageDecodeError{Field: "records"}
	}
	flags := make([]*bool, 5)
	for i, item := range []struct {
		name  string
		value json.RawMessage
	}{{"hasMore", wire.HasMore}, {"isFiltered", wire.IsFiltered}, {"isFilteredByPhase", wire.IsFilteredByPhase}, {"isFilteredByTimeJoin", wire.IsFilteredByTimeJoin}, {"isOld", wire.IsOld}} {
		if len(item.value) == 0 || bytes.Equal(bytes.TrimSpace(item.value), []byte("null")) {
			continue
		}
		var value bool
		switch string(bytes.TrimSpace(item.value)) {
		case "0":
			flags[i] = &value
			continue
		case "1":
			value = true
			flags[i] = &value
			continue
		}
		if err := json.Unmarshal(item.value, &value); err != nil {
			return &GroupHistoryPageDecodeError{Field: item.name}
		}
		flags[i] = &value
	}
	cursor, err := historyDecimal(wire.LastID)
	if err != nil {
		return err
	}
	*p = GroupHistoryPage{Records: *wire.Messages, HasMore: flags[0], LastMessageID: cursor, IsFiltered: flags[1], IsFilteredByPhase: flags[2], IsFilteredByTimeJoin: flags[3], IsOld: flags[4], JoinTimestamp: wire.JoinTimestamp, InternalError: wire.Error}
	return nil
}

type GroupHistoryPageFn func(context.Context, string, string, int) (*GroupHistoryPage, error)

// GetGroupHistoryPage is an optional concrete API extension. The SDK's existing
// public API interface remains compatible. It uses the already authenticated API.
func (a *api) GetGroupHistoryPage(ctx context.Context, groupID, cursor string, count int) (*GroupHistoryPage, error) {
	return a.GetGroupHistoryPageWithPhase(ctx, groupID, cursor, false, count)
}

func (a *api) GetGroupHistoryPageWithPhase(ctx context.Context, groupID, cursor string, old bool, count int) (*GroupHistoryPage, error) {
	if old {
		fn, err := oldGroupHistoryPageFactory(a.sc, a)
		if err != nil {
			return nil, err
		}
		return fn(ctx, groupID, cursor, count)
	}
	fn, err := groupHistoryPageFactory(a.sc, a)
	if err != nil {
		return nil, err
	}
	return fn(ctx, groupID, cursor, count)
}

var groupHistoryPageFactory = newGroupHistoryPageFactory("/api/cm/getrecentv2")
var oldGroupHistoryPageFactory = newGroupHistoryPageFactory("/api/cm/getoldv2")

func newGroupHistoryPageFactory(path string) endpointFactory[*GroupHistoryPage, GroupHistoryPageFn] {
	return apiFactory[*GroupHistoryPage, GroupHistoryPageFn]()(
		func(a *api, sc session.Context, u factoryUtils[*GroupHistoryPage]) (GroupHistoryPageFn, error) {
			base := jsonx.FirstOr(sc.GetZpwService("group_cloud_message"), "")
			if base == "" {
				return nil, ErrGroupHistorySourceUnavailable
			}
			serviceURL := u.MakeURL(strings.TrimSuffix(base, "/")+path, nil, true)
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
				// Bound transport bytes and expanded JSON independently: compressed
				// input must not allocate an unbounded encrypted envelope.
				const responseLimit = 8 << 20
				wire, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
				if err != nil || len(wire) > responseLimit {
					return nil, fmt.Errorf("history response exceeds wire budget or cannot be read")
				}
				response.Body = io.NopCloser(bytes.NewReader(wire))
				decoded, err := httpx.DecodeResponse(response)
				if err != nil {
					return nil, fmt.Errorf("history response decoding failed")
				}
				defer decoded.Close()
				expanded, err := io.ReadAll(io.LimitReader(decoded, responseLimit+1))
				if err != nil || len(expanded) > responseLimit {
					return nil, fmt.Errorf("history response exceeds expanded budget or cannot be read")
				}
				response.Body = io.NopCloser(bytes.NewReader(expanded))
				response.Header.Del("Content-Encoding")
				raw, err := resolveResponse[json.RawMessage](sc, response, true)
				if err != nil {
					return nil, err
				}
				var result GroupHistoryPage
				if err := json.Unmarshal(raw, &result); err != nil {
					return nil, err
				}
				if len(result.Records) > count {
					return nil, fmt.Errorf("invalid group history page size")
				}
				if result.InternalError != nil && *result.InternalError != 0 {
					return nil, fmt.Errorf("group history returned internal error")
				}
				return &result, nil
			}, nil
		},
	)
}

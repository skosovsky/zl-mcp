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

var ErrConversationPreloadUnavailable = errors.New("conversation preload service unavailable")

// ConversationPreloadPage is raw, bounded evidence, not a complete inbox.
// Nil message slices mean absent/null arrays; empty slices mean observed empty.
type ConversationPreloadPage struct {
	Metadata                []json.RawMessage
	DirectMessages          []json.RawMessage
	GroupMessages           []json.RawMessage
	UnsupportedMessageCount int
}

func (p *ConversationPreloadPage) UnmarshalJSON(raw []byte) error {
	*p = ConversationPreloadPage{}
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return fmt.Errorf("invalid preload page encoding")
		}
		raw = []byte(encoded)
	}
	var wire struct {
		Metadata *[]json.RawMessage `json:"clearUnreads"`
		Direct   []json.RawMessage  `json:"msgs"`
		Groups   []json.RawMessage  `json:"groupMsgs"`
		Pages    []json.RawMessage  `json:"pageMsgs"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil || wire.Metadata == nil {
		return fmt.Errorf("missing or invalid preload metadata")
	}
	if len(*wire.Metadata) > 5000 || len(wire.Direct)+len(wire.Groups)+len(wire.Pages) > 1000 {
		return fmt.Errorf("preload page exceeds bounds")
	}
	for _, records := range [][]json.RawMessage{*wire.Metadata, wire.Direct, wire.Groups, wire.Pages} {
		for _, record := range records {
			record = bytes.TrimSpace(record)
			if len(record) == 0 || record[0] != '{' {
				return fmt.Errorf("invalid preload record shape")
			}
		}
	}
	*p = ConversationPreloadPage{Metadata: *wire.Metadata, DirectMessages: wire.Direct, GroupMessages: wire.Groups, UnsupportedMessageCount: len(wire.Pages)}
	return nil
}

type ConversationPreloadFn func(context.Context) (*ConversationPreloadPage, error)

// GetConversationPreload is an optional read through the existing API session.
func (a *api) GetConversationPreload(ctx context.Context) (*ConversationPreloadPage, error) {
	fn, err := conversationPreloadFactory(a.sc, a)
	if err != nil {
		return nil, err
	}
	return fn(ctx)
}

var conversationPreloadFactory = apiFactory[*ConversationPreloadPage, ConversationPreloadFn]()(
	func(a *api, sc session.Context, u factoryUtils[*ConversationPreloadPage]) (ConversationPreloadFn, error) {
		base := jsonx.FirstOr(sc.GetZpwService("conversation"), "")
		if base == "" {
			return nil, ErrConversationPreloadUnavailable
		}
		serviceURL := u.MakeURL(strings.TrimSuffix(base, "/")+"/api/preloadconvers/get-last-msgs", nil, true)
		return func(ctx context.Context) (*ConversationPreloadPage, error) {
			encrypted, err := u.EncodeAES(jsonx.Stringify(map[string]any{"threadIdLocalMsgId": "{}", "imei": sc.IMEI()}))
			if err != nil {
				return nil, fmt.Errorf("preload request encoding failed")
			}
			response, err := u.Request(ctx, u.MakeURL(serviceURL, map[string]any{"params": encrypted, "nretry": 0}, true), &httpx.RequestOptions{Method: http.MethodGet})
			if err != nil {
				return nil, err
			}
			defer response.Body.Close()
			// Bound both wire bytes and expanded JSON before Resolve allocates
			// encrypted payloads; compressed input must not bypass the limit.
			const responseLimit = 8 << 20
			wire, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
			if err != nil || len(wire) > responseLimit {
				return nil, fmt.Errorf("preload response exceeds wire budget or cannot be read")
			}
			response.Body = io.NopCloser(bytes.NewReader(wire))
			decoded, err := httpx.DecodeResponse(response)
			if err != nil {
				return nil, fmt.Errorf("preload response decoding failed")
			}
			defer decoded.Close()
			expanded, err := io.ReadAll(io.LimitReader(decoded, responseLimit+1))
			if err != nil || len(expanded) > responseLimit {
				return nil, fmt.Errorf("preload response exceeds expanded budget or cannot be read")
			}
			response.Body = io.NopCloser(bytes.NewReader(expanded))
			response.Header.Del("Content-Encoding")
			result, err := u.Resolve(response, true)
			if err != nil {
				return nil, err
			}
			if result == nil {
				return nil, fmt.Errorf("missing preload page")
			}
			return result, nil
		}, nil
	},
)

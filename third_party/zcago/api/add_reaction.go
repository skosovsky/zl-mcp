package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/internal/httpx"
	"github.com/amrakk/zcago/internal/jsonx"
	"github.com/amrakk/zcago/model"
	"github.com/amrakk/zcago/session"
)

var ErrInvalidReaction = errs.NewZCA("invalid reaction data", "api.AddReaction")

type (
	AddReactionData struct {
		MsgID    string
		CliMsgID string
	}
	AddReactionDestination struct {
		ThreadID string
		Type     model.ThreadType
		Data     AddReactionData
	}

	AddReactionResponse struct {
		MsgIDs []int `json:"msgIds"`
	}
	AddReactionFn = func(ctx context.Context, dest AddReactionDestination, reaction model.ReactionData) (*AddReactionResponse, error)
)

func (a *api) AddReaction(ctx context.Context, dest AddReactionDestination, reaction model.ReactionData) (*AddReactionResponse, error) {
	return a.e.AddReaction(ctx, dest, reaction)
}

var addReactionFactory = apiFactory[*AddReactionResponse, AddReactionFn]()(
	func(a *api, sc session.Context, u factoryUtils[*AddReactionResponse]) (AddReactionFn, error) {
		base := jsonx.FirstOr(sc.GetZpwService("reaction"), "")
		serviceURLs := map[model.ThreadType]string{
			model.ThreadTypeUser:  u.MakeURL(base+"/api/message/reaction", nil, true),
			model.ThreadTypeGroup: u.MakeURL(base+"/api/group/reaction", nil, true),
		}

		return func(ctx context.Context, dest AddReactionDestination, reaction model.ReactionData) (*AddReactionResponse, error) {
			if (dest.Type != model.ThreadTypeUser && dest.Type != model.ThreadTypeGroup) || dest.ThreadID == "" || !reaction.IsValid() {
				return nil, ErrInvalidReaction
			}

			gMsgID, err := positiveActionMessageID(dest.Data.MsgID)
			if err != nil {
				return nil, err
			}
			cMsgID, err := positiveActionMessageID(dest.Data.CliMsgID)
			if err != nil {
				return nil, err
			}

			msg := map[string]any{
				"rMsg": []map[string]any{{
					"gMsgID":  gMsgID,
					"cMsgID":  cMsgID,
					"msgType": 1,
				}},
				"rIcon":  reaction.RIcon,
				"rType":  reaction.RType,
				"source": reaction.Source,
			}

			reactList := []map[string]any{{
				"message":  jsonx.Stringify(msg),
				"clientId": time.Now().UnixMilli(),
			}}

			payload := map[string]any{
				"react_list": reactList,
			}

			if dest.Type == model.ThreadTypeUser {
				payload["toid"] = dest.ThreadID
			} else {
				payload["grid"] = dest.ThreadID
				payload["imei"] = sc.IMEI()
			}

			enc, err := u.EncodeAES(jsonx.Stringify(payload))
			if err != nil {
				return nil, errs.WrapZCA("failed to encrypt params", "api.AddReaction", err)
			}

			body := httpx.BuildFormBody(map[string]string{"params": enc})
			resp, err := u.Request(ctx, serviceURLs[dest.Type], &httpx.RequestOptions{
				Method: http.MethodPost,
				Body:   body,
			})
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			return u.Resolve(resp, true)
		}, nil
	},
)

func (r *AddReactionResponse) UnmarshalJSON(data []byte) error {
	var aux struct {
		MsgIDs json.RawMessage `json:"msgIds"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	ids, shape, err := decodeReactionMessageIDs(aux.MsgIDs)
	// Closed shape/outcome only: never log native bodies, IDs or error text.
	slog.Info("zalo_reaction_response_shape", "msg_ids_shape", shape,
		"decode_ok", err == nil, "msg_ids_count", min(len(ids), 1024))
	if err != nil {
		return err
	}
	r.MsgIDs = ids
	return nil
}

func decodeReactionMessageIDs(encoded json.RawMessage) ([]int, string, error) {
	if len(encoded) == 0 {
		return []int{}, "missing", nil
	}
	if string(encoded) == "null" {
		return []int{}, "null", nil
	}
	// Upstream accepts an array or an array encoded inside a string. Preserve
	// integer precision by decoding directly, without a float64 intermediate.
	shape := "other"
	if encoded[0] == '[' {
		shape = "array"
	}
	if encoded[0] == '"' {
		shape = "string"
		var text string
		if err := json.Unmarshal(encoded, &text); err != nil {
			return nil, shape, err
		}
		if text == "" {
			return []int{}, shape, nil
		}
		encoded = []byte(text)
	}
	var ids []int
	if err := json.Unmarshal(encoded, &ids); err != nil {
		return nil, shape, err
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, shape, ErrInvalidReaction
		}
	}
	return ids, shape, nil
}

// positiveActionMessageID rejects malformed identifiers before constructing a write.
// Numeric JSON values are int64; never parse through float64.
func positiveActionMessageID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		return 0, ErrInvalidReaction
	}
	return id, nil
}

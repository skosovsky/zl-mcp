package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/amrakk/zcago/internal/cryptox"
	"github.com/amrakk/zcago/model"
	"github.com/amrakk/zcago/session"
)

func TestReactionWriteWireAndInvalidDestinations(t *testing.T) {
	for _, kind := range []model.ThreadType{model.ThreadTypeUser, model.ThreadTypeGroup} {
		for _, icon := range []model.ReactionIcon{model.ReactionHeart, model.ReactionTearsOfJoy, model.ReactionNone} {
			t.Run(string(icon)+string(rune('0'+kind)), func(t *testing.T) {
				// Arrange: decrypt only synthetic requests on a loopback test server.
				key := []byte(strings.Repeat("k", 32))
				calls := 0
				receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					path := "/api/message/reaction"
					if kind == model.ThreadTypeGroup {
						path = "/api/group/reaction"
					}
					if r.Method != "POST" || r.URL.Path != path {
						t.Error("incorrect route")
					}
					if err := r.ParseForm(); err != nil {
						t.Error(err)
						return
					}
					plain, err := cryptox.DecodeAESCBC(key, r.Form.Get("params"))
					if err != nil {
						t.Error(err)
						return
					}
					var envelope struct {
						To    string `json:"toid"`
						Group string `json:"grid"`
						IMEI  string `json:"imei"`
						List  []struct {
							Message string `json:"message"`
						} `json:"react_list"`
					}
					if err = json.Unmarshal(plain, &envelope); err != nil || len(envelope.List) != 1 {
						t.Error("invalid envelope")
						return
					}
					var payload map[string]any
					decoder := json.NewDecoder(bytes.NewBufferString(envelope.List[0].Message))
					decoder.UseNumber()
					if err = decoder.Decode(&payload); err != nil {
						t.Error(err)
						return
					}
					refs := payload["rMsg"].([]any)
					ref := refs[0].(map[string]any)
					want := model.NewReactionData(icon)
					if ref["gMsgID"] != json.Number("9007199254740993") || ref["cMsgID"] != json.Number("9007199254740995") || payload["rIcon"] != string(icon) || payload["rType"] != json.Number(strconv.Itoa(want.RType)) || payload["source"] != json.Number("6") {
						t.Error("reaction IDs or chosen icon changed")
					}
					if kind == model.ThreadTypeUser && (envelope.To != "peer" || envelope.Group != "" || envelope.IMEI != "") {
						t.Error("wrong direct destination")
					}
					if kind == model.ThreadTypeGroup && (envelope.Group != "peer" || envelope.To != "" || envelope.IMEI != "synthetic-imei") {
						t.Error("wrong group destination")
					}
					reply := `{"error_code":0,"data":{"msgIds":"[1]"}}`
					if kind == model.ThreadTypeGroup {
						reply = `{"error_code":0,"data":{"msgIds":[1]}}`
					}
					encoded, err := cryptox.EncodeAESCBC(key, reply, cryptox.EncryptTypeBase64)
					if err != nil {
						t.Error(err)
						return
					}
					if err = json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": encoded}); err != nil {
						t.Error(err)
					}
				}))
				defer receiver.Close()
				sc := session.NewContext(session.WithHTTPClient(receiver.Client()), session.WithLogging(false))
				sc.SealLogin(session.Seal{UID: "owner", UserAgent: "synthetic", IMEI: "synthetic-imei", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString(key)), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{Reaction: []string{receiver.URL}}}})
				fn, err := addReactionFactory(sc, &api{sc: sc})
				if err != nil {
					t.Fatal(err)
				}
				dest := AddReactionDestination{ThreadID: "peer", Type: kind, Data: AddReactionData{MsgID: "9007199254740993", CliMsgID: "9007199254740995"}}
				// Act.
				result, err := fn(context.Background(), dest, model.NewReactionData(icon))
				// Assert: preserve integers above float64's exact range and the removal sentinel.
				if err != nil || result == nil || len(result.MsgIDs) != 1 || calls != 1 {
					t.Fatal("valid request failed", err)
				}
				for _, bad := range []string{"", "0", "-1", "+1", "01", "1.5", "1e3", "9223372036854775808", "private-marker"} {
					for _, client := range []bool{false, true} {
						invalid := dest
						if client {
							invalid.Data.CliMsgID = bad
						} else {
							invalid.Data.MsgID = bad
						}
						if _, err = fn(context.Background(), invalid, model.NewReactionData(icon)); err == nil {
							t.Fatal("invalid ID accepted")
						}
					}
				}
				for _, invalid := range []AddReactionDestination{{ThreadID: "peer", Type: model.ThreadType(99), Data: dest.Data}, {Type: kind, Data: dest.Data}} {
					if _, err = fn(context.Background(), invalid, model.NewReactionData(icon)); err == nil {
						t.Fatal("invalid destination accepted")
					}
				}
				if _, err = fn(context.Background(), dest, model.ReactionData{}); err == nil {
					t.Fatal("zero value removed reaction")
				}
				if calls != 1 {
					t.Fatal("invalid request contacted upstream")
				}
			})
		}
	}
}

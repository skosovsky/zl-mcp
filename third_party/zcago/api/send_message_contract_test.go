package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amrakk/zcago/internal/cryptox"
	"github.com/amrakk/zcago/model"
	"github.com/amrakk/zcago/session"
)

func TestDirectTextAndQuoteWireContract(t *testing.T) {
	for _, acknowledgement := range []struct{ name, payload, want string }{
		{"string", `{"error_code":0,"data":{"msgId":"accepted"}}`, "accepted"},
		{"number", `{"error_code":0,"data":{"msgId":9007199254740993}}`, "9007199254740993"},
	} {
		for _, quoted := range []bool{false, true} {
			name := "plain"
			if quoted {
				name = "quote"
			}
			t.Run(name+"/"+acknowledgement.name, func(t *testing.T) {
				// Arrange: only local HTTP and a fixed synthetic encryption key.
				key := []byte(strings.Repeat("k", 32))
				seen := make(chan map[string]any, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := "/api/message/sms"
					if quoted {
						path = "/api/message/quote"
					}
					if r.Method != "POST" || r.URL.Path != path || r.URL.Query().Get("nretry") != "0" {
						t.Error("wrong send route")
						w.WriteHeader(400)
						return
					}
					if err := r.ParseForm(); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					plain, err := cryptox.DecodeAESCBC(key, r.Form.Get("params"))
					if err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					var payload map[string]any
					if err = json.Unmarshal(plain, &payload); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					seen <- payload
					encoded, err := cryptox.EncodeAESCBC(key, acknowledgement.payload, cryptox.EncryptTypeBase64)
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": encoded})
				}))
				defer server.Close()
				sc := session.NewContext(session.WithHTTPClient(server.Client()), session.WithLogging(false))
				sc.SealLogin(session.Seal{UID: "owner", UserAgent: "synthetic-test-client", IMEI: "synthetic-imei", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString(key)), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{Chat: []string{server.URL}}}})
				fn, err := sendMessageFactory(sc, &api{sc: sc})
				if err != nil {
					t.Fatal(err)
				}
				content := MessageContent{Msg: "hello 界"}
				if quoted {
					text := "original text"
					content.Quote = &SendMessageQuote{MsgID: "9007199254740993", CliMsgID: "9007199254740995", UIDFrom: "peer", MsgType: "webchat", Content: model.Content{String: &text}, TS: "1791028800000", TTL: 7}
				}
				// Act: exercise the actual upstream factory, form encoding and response.
				result, err := fn(context.Background(), "peer", model.ThreadTypeUser, content)
				// Assert: exact IDs stay strings, Unicode text and quote fields survive.
				if err != nil || result == nil || result.Message == nil || result.Message.MsgID != acknowledgement.want {
					t.Fatal("send response failed", err)
				}
				payload := <-seen
				if payload["toid"] != "peer" || payload["imei"] != "synthetic-imei" || payload["message"] != content.Msg || payload["clientId"] == nil || payload["grid"] != nil {
					t.Fatal("incorrect direct payload")
				}
				if quoted {
					if payload["qmsgId"] != "9007199254740993" || payload["qmsgCliId"] != "9007199254740995" || payload["qmsgOwner"] != "peer" || payload["qmsg"] != "original text" || payload["qmsgType"] != float64(1) || payload["qmsgTs"] != "1791028800000" || payload["qmsgTTL"] != float64(7) {
						t.Fatal("quote wire metadata changed")
					}
				} else if payload["qmsgId"] != nil {
					t.Fatal("plain send introduced quote")
				}
			})
		}
	}
}

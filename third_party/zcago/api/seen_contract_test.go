package api

import (
	"bytes"
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

func TestSeenWireKeepsNativeCountersAndTypedRecipients(t *testing.T) {
	for _, kind := range []model.ThreadType{model.ThreadTypeUser, model.ThreadTypeGroup} {
		t.Run(map[model.ThreadType]string{model.ThreadTypeUser: "direct", model.ThreadTypeGroup: "group"}[kind], func(t *testing.T) {
			// Arrange: production factory, synthetic encryption/session, no Zalo network.
			key := []byte(strings.Repeat("k", 32))
			calls := 0
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				path := "/api/message/seenv2"
				if kind == model.ThreadTypeGroup {
					path = "/api/group/seenv2"
				}
				if r.Method != "POST" || r.URL.Path != path || r.URL.Query().Get("nretry") != "0" {
					t.Error("incorrect seen route")
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
					Infos string `json:"msgInfos"`
					IMEI  string `json:"imei"`
				}
				if err = json.Unmarshal(plain, &envelope); err != nil {
					t.Error(err)
					return
				}
				var payload struct {
					Sender string           `json:"senderId"`
					Group  string           `json:"grid"`
					Data   []map[string]any `json:"data"`
				}
				decoder := json.NewDecoder(bytes.NewBufferString(envelope.Infos))
				decoder.UseNumber()
				if err = decoder.Decode(&payload); err != nil || len(payload.Data) != 1 {
					t.Error("invalid payload")
					return
				}
				got := payload.Data[0]
				recipient := "0"
				if kind == model.ThreadTypeGroup {
					recipient = "group"
				}
				if got["cmi"] != "9007199254740995" || got["gmi"] != "9007199254740993" || got["si"] != "peer" || got["di"] != recipient || got["mt"] != "chat.photo" || got["st"] != json.Number("3") || got["at"] != json.Number("0") || got["cmd"] != json.Number("501") || got["ts"] != json.Number("1791028800000") {
					t.Error("native fields changed")
				}
				if kind == model.ThreadTypeUser && (payload.Sender != "peer" || payload.Group != "" || envelope.IMEI != "") {
					t.Error("wrong direct envelope")
				}
				if kind == model.ThreadTypeGroup && (payload.Group != "group" || payload.Sender != "" || envelope.IMEI != "synthetic-imei") {
					t.Error("wrong group envelope")
				}
				encrypted, err := cryptox.EncodeAESCBC(key, `{"error_code":0,"data":{"status":1}}`, cryptox.EncryptTypeBase64)
				if err != nil {
					t.Error(err)
					return
				}
				if err = json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": encrypted}); err != nil {
					t.Error(err)
				}
			}))
			defer receiver.Close()
			sc := session.NewContext(session.WithHTTPClient(receiver.Client()), session.WithLogging(false))
			sc.SealLogin(session.Seal{UID: "owner", UserAgent: "synthetic", IMEI: "synthetic-imei", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString(key)), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{Chat: []string{receiver.URL}, Group: []string{receiver.URL}}}})
			fn, err := sendSeenEventFactory(sc, &api{sc: sc})
			if err != nil {
				t.Fatal(err)
			}
			message := model.OutboundMessage{MsgID: "9007199254740993", CliMsgID: "9007199254740995", UIDFrom: "peer", IDTo: "owner", MsgType: "chat.photo", ST: 3, AT: 0, CMD: 501, TS: 1791028800000}
			if kind == model.ThreadTypeGroup {
				message.IDTo = "group"
			}
			// Act.
			response, err := fn(context.Background(), []model.OutboundMessage{message}, kind)
			// Assert.
			if err != nil || response == nil || response.Status != 1 || calls != 1 {
				t.Fatal("valid seen failed", err)
			}
			for _, mutate := range []func(*model.OutboundMessage){
				func(m *model.OutboundMessage) { m.MsgID = "invalid" }, func(m *model.OutboundMessage) { m.CliMsgID = "0" },
				func(m *model.OutboundMessage) { m.UIDFrom = "" }, func(m *model.OutboundMessage) { m.IDTo = "" },
				func(m *model.OutboundMessage) { m.MsgType = "" }, func(m *model.OutboundMessage) { m.TS = 0 },
			} {
				bad := message
				mutate(&bad)
				if _, err = fn(context.Background(), []model.OutboundMessage{bad}, kind); err == nil {
					t.Fatal("invalid evidence accepted")
				}
			}
			other := message
			if kind == model.ThreadTypeUser {
				other.UIDFrom = "another"
			} else {
				other.IDTo = "another"
			}
			for _, batch := range [][]model.OutboundMessage{nil, make([]model.OutboundMessage, 51), {message, other}} {
				if _, err = fn(context.Background(), batch, kind); err == nil {
					t.Fatal("invalid batch accepted")
				}
			}
			if _, err = fn(context.Background(), []model.OutboundMessage{message}, model.ThreadType(99)); err == nil {
				t.Fatal("invalid thread type accepted")
			}
			if calls != 1 {
				t.Fatal("invalid requests contacted source")
			}
		})
	}
}

func TestSeenStatusPreservesPresenceIncludingZero(t *testing.T) {
	for _, tc := range []struct {
		body    string
		present bool
		status  int
	}{
		{`{}`, false, 0}, {`{"status":null}`, false, 0}, {`{"status":0}`, true, 0}, {`{"status":7}`, true, 7},
	} {
		// Arrange: reuse an object so a missing status cannot inherit earlier evidence.
		got := SendSeenEventResponse{Status: 99, HasStatus: true}
		// Act.
		err := json.Unmarshal([]byte(tc.body), &got)
		// Assert: no value is interpreted as success by this codec.
		if err != nil || got.HasStatus != tc.present || got.Status != tc.status {
			t.Fatal("status presence lost", err)
		}
	}
}

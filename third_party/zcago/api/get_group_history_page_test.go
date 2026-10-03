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
	"github.com/amrakk/zcago/session"
)

func TestGroupHistoryPageWirePreservesNumericCursorAndFlags(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(map[bool]string{false: "object", true: "nested_string"}[wrapped], func(t *testing.T) {
			// Arrange
			key := []byte(strings.Repeat("k", 32))
			seen := 0
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen++
				if r.Method != "GET" || r.URL.Path != "/api/cm/getrecentv2" || r.URL.Query().Get("nretry") != "0" {
					t.Error("wrong history route")
				}
				plain, err := cryptox.DecodeAESCBC(key, r.URL.Query().Get("params"))
				if err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				decoder := json.NewDecoder(bytes.NewReader(plain))
				decoder.UseNumber()
				var payload map[string]any
				if err = decoder.Decode(&payload); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if payload["groupId"] != "synthetic-group" || payload["globalMsgId"] != json.Number("9007199254740993") || payload["count"] != json.Number("2") || payload["src"] != json.Number("3") || payload["imei"] != "synthetic-imei" || len(payload["msgIds"].([]any)) != 0 {
					t.Errorf("wrong request: %#v", payload)
				}
				data := json.RawMessage(`{"groupMsgs":[{"msgId":9007199254740995}],"lastMsgId":9007199254740997,"hasMore":true,"isFiltered":false,"isFilteredByTimeJoin":true,"error":0}`)
				var inner any = data
				if wrapped {
					inner = string(data)
				}
				body, err := json.Marshal(map[string]any{"error_code": 0, "data": inner})
				if err != nil {
					t.Error(err)
					return
				}
				encrypted, err := cryptox.EncodeAESCBC(key, string(body), cryptox.EncryptTypeBase64)
				if err != nil {
					t.Error(err)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": encrypted})
			}))
			defer receiver.Close()
			sc := session.NewContext(session.WithHTTPClient(receiver.Client()), session.WithLogging(false))
			sc.SealLogin(session.Seal{UID: "owner", UserAgent: "synthetic", IMEI: "synthetic-imei", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString(key)), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{GroupCloudMessage: []string{receiver.URL}}}})
			fn, err := groupHistoryPageFactory(sc, &api{sc: sc})
			if err != nil {
				t.Fatal(err)
			}
			// Act
			page, err := fn(context.Background(), "synthetic-group", "9007199254740993", 2)
			// Assert
			if err != nil {
				t.Fatal(err)
			}
			if seen != 1 || page.LastMessageID == nil || *page.LastMessageID != "9007199254740997" || page.HasMore == nil || !*page.HasMore || page.IsFiltered == nil || *page.IsFiltered || page.IsFilteredByTimeJoin == nil || !*page.IsFilteredByTimeJoin || len(page.Records) != 1 || !bytes.Contains(page.Records[0], []byte("9007199254740995")) {
				t.Fatal("precision or filtering metadata lost")
			}
			// Act / Assert: reject malformed requests before contacting the source.
			for _, cursor := range []string{"", "-1", "1.2", "1e3", "01", strings.Repeat("1", 129)} {
				if _, err = fn(context.Background(), "synthetic-group", cursor, 2); err == nil {
					t.Fatal("invalid cursor accepted")
				}
			}
			for _, count := range []int{0, 51} {
				if _, err = fn(context.Background(), "synthetic-group", "0", count); err == nil {
					t.Fatal("invalid count accepted")
				}
			}
			if seen != 1 {
				t.Fatal("invalid requests contacted source")
			}
		})
	}
}

func TestGroupHistoryPageRejectsInvalidEvidence(t *testing.T) {
	for _, body := range []string{`{}`, `{"groupMsgs":null}`, `{"groupMsgs":[],"hasMore":1}`, `{"groupMsgs":[],"lastMsgId":1.5}`, `{"groupMsgs":[],"lastMsgId":1e5}`, `{"groupMsgs":[],"lastMsgId":{}}`} {
		// Arrange / Act
		var page GroupHistoryPage
		err := json.Unmarshal([]byte(body), &page)
		// Assert
		if err == nil {
			t.Fatalf("invalid metadata accepted: %s", body)
		}
	}
	// Arrange / Act: omission differs from terminal false and numeric zero.
	var omitted, terminal GroupHistoryPage
	if err := json.Unmarshal([]byte(`{"groupMsgs":[]}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"groupMsgs":[],"lastMsgId":"0","hasMore":false}`), &terminal); err != nil {
		t.Fatal(err)
	}
	// Assert
	if omitted.HasMore != nil || omitted.LastMessageID != nil || terminal.HasMore == nil || *terminal.HasMore || terminal.LastMessageID == nil || *terminal.LastMessageID != "0" {
		t.Fatal("nullable continuation metadata lost")
	}
}

func TestGroupHistoryPageLimitsAndMissingSource(t *testing.T) {
	// Arrange: a reusable result must not retain successful data after an error.
	page := GroupHistoryPage{Records: []json.RawMessage{json.RawMessage(`{"msgId":"old"}`)}}
	records := make([]json.RawMessage, 51)
	for i := range records {
		records[i] = json.RawMessage(`{}`)
	}
	body, err := json.Marshal(map[string]any{"groupMsgs": records})
	if err != nil {
		t.Fatal(err)
	}
	// Act
	err = json.Unmarshal(body, &page)
	// Assert
	if err == nil || len(page.Records) != 0 {
		t.Fatal("oversized response retained stale records")
	}
	// Arrange / Act: missing advertised service is not replaced by a guessed URL.
	sc := session.NewContext(session.WithLogging(false))
	sc.SealLogin(session.Seal{UID: "owner", LoginInfo: &session.LoginInfo{}})
	_, err = groupHistoryPageFactory(sc, &api{sc: sc})
	// Assert
	if err == nil {
		t.Fatal("missing history source accepted")
	}
}

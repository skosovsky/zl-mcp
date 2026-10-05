package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
				data := json.RawMessage(`{"groupMsgs":[{"msgId":9007199254740995}],"lastMsgId":9007199254740997,"hasMore":1,"isFiltered":false,"isFilteredByTimeJoin":true,"error":0}`)
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
	for _, body := range []string{`{}`, `{"groupMsgs":null}`, `{"groupMsgs":[],"hasMore":2}`, `{"groupMsgs":[],"lastMsgId":1.5}`, `{"groupMsgs":[],"lastMsgId":1e5}`, `{"groupMsgs":[],"lastMsgId":{}}`} {
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

func TestGroupHistoryInvalidFlagHasSafeFieldEvidence(t *testing.T) {
	// Arrange / Act: invalid private text is never retained as a decode reason.
	for _, field := range []string{"hasMore", "isOld", "isFiltered", "isFilteredByPhase", "isFilteredByTimeJoin"} {
		var page GroupHistoryPage
		err := json.Unmarshal([]byte(`{"groupMsgs":[],"`+field+`":"private-body-marker"}`), &page)
		// Assert
		var invalid *GroupHistoryPageDecodeError
		if !errors.As(err, &invalid) || invalid.Field != field || strings.Contains(err.Error(), "private-") {
			t.Fatal("decode reason lost or unsafe")
		}
	}
}

func TestGroupHistoryNormalizesOnlyBinaryIntegerFlags(t *testing.T) {
	// Arrange / Act
	var page GroupHistoryPage
	err := json.Unmarshal([]byte(`{"groupMsgs":[],"hasMore":1,"isOld":0,"isFiltered":true,"isFilteredByPhase":false}`), &page)
	// Assert
	if err != nil || page.HasMore == nil || !*page.HasMore || page.IsOld == nil || *page.IsOld || page.IsFiltered == nil || !*page.IsFiltered || page.IsFilteredByPhase == nil || *page.IsFilteredByPhase || page.IsFilteredByTimeJoin != nil {
		t.Fatal("binary flag normalization lost unknown/false")
	}
	for _, raw := range []string{`2`, `-1`, `1.0`, `"1"`, `"true"`, `{}`, `[]`} {
		err = json.Unmarshal([]byte(`{"groupMsgs":[],"hasMore":`+raw+`}`), &page)
		if err == nil {
			t.Fatal("noncanonical source flag accepted")
		}
	}
}

func TestGroupHistoryBoundsWireAndExpandedJSON(t *testing.T) {
	for _, padding := range []bool{false, true} {
		for _, compressed := range []bool{false, true} {
			t.Run(map[bool]string{false: "oversized_envelope/", true: "valid_prefix_padding/"}[padding]+map[bool]string{false: "plain", true: "gzip"}[compressed], func(t *testing.T) {
				// Arrange: an oversized envelope must not reach decrypted-page parsing.
				raw := []byte(`{"error_code":0,"data":"` + strings.Repeat("x", 9<<20) + `"}`)
				if padding {
					key := []byte(strings.Repeat("k", 32))
					encrypted, err := cryptox.EncodeAESCBC(key, `{"error_code":0,"data":{"groupMsgs":[],"hasMore":false}}`, cryptox.EncryptTypeBase64)
					if err != nil {
						t.Fatal(err)
					}
					raw, err = json.Marshal(map[string]any{"error_code": 0, "data": encrypted})
					if err != nil {
						t.Fatal(err)
					}
					raw = append(raw, bytes.Repeat([]byte(" "), 9<<20)...)
				}
				header := http.Header{}
				if compressed {
					var packed bytes.Buffer
					writer := gzip.NewWriter(&packed)
					if _, err := writer.Write(raw); err != nil {
						t.Fatal(err)
					}
					if err := writer.Close(); err != nil {
						t.Fatal(err)
					}
					raw = packed.Bytes()
					header.Set("Content-Encoding", "gzip")
				}
				body := &preloadCountedBody{Reader: bytes.NewReader(raw)}
				client := &http.Client{Transport: preloadTransport(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: header, Body: body, Request: r}, nil
				})}
				sc := session.NewContext(session.WithHTTPClient(client), session.WithLogging(false))
				sc.SealLogin(session.Seal{UID: "owner", UserAgent: "synthetic", IMEI: "synthetic-imei", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{GroupCloudMessage: []string{"https://synthetic.invalid"}}}})
				fn, err := groupHistoryPageFactory(sc, &api{sc: sc})
				if err != nil {
					t.Fatal(err)
				}
				// Act
				page, err := fn(context.Background(), "synthetic-group", "0", 1)
				// Assert: truncation is not source exhaustion or a usable page.
				if err == nil || page != nil || body.read > (8<<20)+1 {
					t.Fatal("history response bound bypassed")
				}
			})
		}
	}
}

func TestGroupHistoryExplicitPhaseSelectsOldRoute(t *testing.T) {
	// Arrange
	key := []byte(strings.Repeat("k", 32))
	paths := []string{}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		plain, err := cryptox.DecodeAESCBC(key, r.URL.Query().Get("params"))
		if err != nil {
			t.Error(err)
			return
		}
		var payload map[string]json.RawMessage
		if err = json.Unmarshal(plain, &payload); err != nil {
			t.Error(err)
			return
		}
		if string(payload["globalMsgId"]) != "9007199254740993" {
			t.Error("cursor precision lost")
		}
		encrypted, err := cryptox.EncodeAESCBC(key, `{"error_code":0,"data":{"groupMsgs":[],"hasMore":0,"isOld":1}}`, cryptox.EncryptTypeBase64)
		if err != nil {
			t.Error(err)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": encrypted})
	}))
	defer receiver.Close()
	sc := session.NewContext(session.WithHTTPClient(receiver.Client()), session.WithLogging(false))
	sc.SealLogin(session.Seal{UID: "owner", UserAgent: "synthetic", IMEI: "synthetic-imei", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString(key)), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{GroupCloudMessage: []string{receiver.URL}}}})
	client := &api{sc: sc}
	// Act: a nonzero cursor alone stays recent; explicit phase changes the route.
	_, err := client.GetGroupHistoryPage(context.Background(), "synthetic-group", "9007199254740993", 1)
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.GetGroupHistoryPageWithPhase(context.Background(), "synthetic-group", "9007199254740993", true, 1)
	// Assert
	if err != nil || len(paths) != 2 || paths[0] != "/api/cm/getrecentv2" || paths[1] != "/api/cm/getoldv2" || page.IsOld == nil || !*page.IsOld {
		t.Fatal("phase routing failed", err)
	}
}

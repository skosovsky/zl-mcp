package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amrakk/zcago/internal/cryptox"
	"github.com/amrakk/zcago/session"
)

func TestConversationPreloadUsesAdvertisedSessionAndPreservesExactRecords(t *testing.T) {
	// Arrange
	key := []byte(strings.Repeat("k", 32))
	calls := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/preloadconvers/get-last-msgs" || r.URL.Query().Get("nretry") != "0" {
			t.Error("wrong preload route")
		}
		plain, err := cryptox.DecodeAESCBC(key, r.URL.Query().Get("params"))
		if err != nil {
			t.Error(err)
			return
		}
		var params map[string]any
		if err := json.Unmarshal(plain, &params); err != nil {
			t.Error(err)
			return
		}
		if len(params) != 2 || params["threadIdLocalMsgId"] != "{}" || params["imei"] != "synthetic-imei" {
			t.Error("unexpected preload parameters")
		}
		data := json.RawMessage(`{"clearUnreads":[{"idTo":9007199254740993,"isGroup":0,"lastMsgId":9007199254740995}],"msgs":[{"msgId":9007199254740997}],"groupMsgs":[]}`)
		body, _ := json.Marshal(map[string]any{"error_code": 0, "data": data})
		encrypted, err := cryptox.EncodeAESCBC(key, string(body), cryptox.EncryptTypeBase64)
		if err != nil {
			t.Error(err)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": encrypted})
	}))
	defer receiver.Close()
	sc := session.NewContext(session.WithHTTPClient(receiver.Client()), session.WithLogging(false))
	sc.SealLogin(session.Seal{UID: "owner", UserAgent: "synthetic", IMEI: "synthetic-imei", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString(key)), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{Conversation: []string{receiver.URL}}}})
	fn, err := conversationPreloadFactory(sc, &api{sc: sc})
	if err != nil {
		t.Fatal(err)
	}
	// Act
	page, err := fn(context.Background())
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(page.Metadata) != 1 || len(page.DirectMessages) != 1 || page.GroupMessages == nil || string(page.DirectMessages[0]) != `{"msgId":9007199254740997}` || !strings.Contains(string(page.Metadata[0]), "9007199254740993") {
		t.Fatal("preload precision/category evidence lost")
	}
}

func TestConversationPreloadRejectsMalformedAndOversizedWholePages(t *testing.T) {
	for _, body := range []string{`{}`, `{"clearUnreads":null}`, `{"clearUnreads":[null]}`, `{"clearUnreads":[],"msgs":[[]]}`, `{"clearUnreads":[],"msgs":false}`, `{"clearUnreads":[` + strings.TrimSuffix(strings.Repeat(`{},`, 5001), ",") + `]}`, `{"clearUnreads":[],"groupMsgs":[` + strings.TrimSuffix(strings.Repeat(`{},`, 1001), ",") + `]}`} {
		// Arrange: stale successful contents must be cleared on failure.
		page := ConversationPreloadPage{Metadata: []json.RawMessage{json.RawMessage(`{}`)}}
		// Act
		err := json.Unmarshal([]byte(body), &page)
		// Assert
		if err == nil || page.Metadata != nil {
			t.Fatal("invalid preload page returned stale/partial records")
		}
	}
	// Arrange / Act: absent differs from an observed empty message category.
	var absent, empty ConversationPreloadPage
	if err := json.Unmarshal([]byte(`{"clearUnreads":[]}`), &absent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"clearUnreads":[],"msgs":[]}`), &empty); err != nil {
		t.Fatal(err)
	}
	// Assert
	if absent.DirectMessages != nil || empty.DirectMessages == nil {
		t.Fatal("message availability lost")
	}
}

func TestConversationPreloadMissingServiceDoesNotGuessDomain(t *testing.T) {
	// Arrange / Act
	sc := session.NewContext(session.WithLogging(false))
	_, err := conversationPreloadFactory(sc, &api{sc: sc})
	// Assert
	if !errors.Is(err, ErrConversationPreloadUnavailable) {
		t.Fatal("missing service not explicitly rejected")
	}
}

type preloadTransport func(*http.Request) (*http.Response, error)

func (f preloadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type preloadCountedBody struct {
	io.Reader
	read int
}

func (b *preloadCountedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *preloadCountedBody) Close() error { return nil }

func TestConversationPreloadBoundsWireAndExpandedJSON(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "gzip"}[compressed], func(t *testing.T) {
			// Arrange: a large JSON string requires full decoding unless bounded first.
			raw := []byte(`{"error_code":0,"data":"` + strings.Repeat("x", 9<<20) + `"}`)
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
			sc.SealLogin(session.Seal{UID: "owner", UserAgent: "synthetic", IMEI: "synthetic-imei", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{Conversation: []string{"https://synthetic.invalid"}}}})
			fn, err := conversationPreloadFactory(sc, &api{sc: sc})
			if err != nil {
				t.Fatal(err)
			}
			// Act
			page, err := fn(context.Background())
			// Assert: no truncated valid-looking page or oversized raw read is accepted.
			if err == nil || page != nil || body.read > (8<<20)+1 {
				t.Fatal("preload response bound bypassed")
			}
		})
	}
}

func TestConversationPreloadRejectsOversizedValidPrefix(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(map[bool]string{false: "wire", true: "expanded"}[compressed], func(t *testing.T) {
			// Arrange: an otherwise valid encrypted page plus JSON whitespace past the budget.
			key := []byte(strings.Repeat("k", 32))
			leaf := `{"error_code":0,"data":{"clearUnreads":[],"msgs":[]}}`
			encrypted, err := cryptox.EncodeAESCBC(key, leaf, cryptox.EncryptTypeBase64)
			if err != nil {
				t.Fatal(err)
			}
			prefix, err := json.Marshal(map[string]any{"error_code": 0, "data": encrypted})
			if err != nil {
				t.Fatal(err)
			}
			raw := append(prefix, bytes.Repeat([]byte(" "), (8<<20)+1)...)
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
			sc.SealLogin(session.Seal{UID: "owner", UserAgent: "synthetic", IMEI: "synthetic-imei", SecretKey: session.SecretKey(base64.StdEncoding.EncodeToString(key)), Settings: &session.Settings{}, LoginInfo: &session.LoginInfo{ZpwServiceMapV3: session.ZpwServiceMapV3{Conversation: []string{"https://synthetic.invalid"}}}})
			fn, err := conversationPreloadFactory(sc, &api{sc: sc})
			if err != nil {
				t.Fatal(err)
			}
			// Act.
			page, err := fn(context.Background())
			// Assert: whole-page failure, with at most one overflow sentinel byte.
			if err == nil || page != nil || body.read > (8<<20)+1 {
				t.Fatal("oversized valid prefix accepted")
			}
		})
	}
}

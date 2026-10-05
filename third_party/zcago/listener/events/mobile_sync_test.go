package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func mobileControl(t *testing.T, action, payload string, quoted bool) ControlContent {
	t.Helper()
	var data any = json.RawMessage(payload)
	if quoted {
		data = payload
	}
	b, err := json.Marshal(map[string]any{"act_type": "syncmsgmb", "act": action, "data": data})
	if err != nil {
		t.Fatal(err)
	}
	var content ControlContent
	if err := json.Unmarshal(b, &content); err != nil {
		t.Fatal(err)
	}
	return content
}

func TestMobileSyncExactIDsAndRedaction(t *testing.T) {
	for _, quoted := range []bool{false, true} {
		// Arrange: invented IDs beyond floating-point precision and synthetic transport data.
		body := `{"uid":9007199254740993,"from_seq_id":9007199254740995,"public_key":"synthetic-public","url":"https://synthetic.invalid/backup","encrypted_key":"synthetic-ciphertext","file_size":16,"db_info":"{\"backup_db\":{}}"}`
		// Act.
		content := mobileControl(t, "syncmsg_info", body, quoted)
		e := content.Data.MobileSync
		// Assert.
		if e == nil || e.UID != "9007199254740993" || e.FromSequence != "9007199254740995" || e.FileSize != 16 {
			t.Fatal("metadata precision lost")
		}
		serialized, err := json.Marshal(e)
		if err != nil || string(serialized) != "{}" {
			t.Fatal("JSON transport data leaked")
		}
		for _, s := range []string{fmt.Sprintf("%v", e), fmt.Sprintf("%+v", e), fmt.Sprintf("%#v", e)} {
			if strings.Contains(s, "synthetic-") || !strings.Contains(s, "redacted") {
				t.Fatal("formatted transport data leaked")
			}
		}
	}
}

func TestMobileSyncConfirmationRetainsActions(t *testing.T) {
	for n := 0; n <= 3; n++ {
		// Arrange.
		body := fmt.Sprintf(`{"public_key":"synthetic-public","pc_name":"Web","user_action":%d}`, n)
		// Act.
		e := mobileControl(t, "user_confirm", body, false).Data.MobileSync
		// Assert.
		if e == nil || e.UserAction == nil || *e.UserAction != n {
			t.Fatal("confirmation semantics collapsed")
		}
	}
}

func TestMobileSyncMalformedDataFailsClosed(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"public_key":"k","pc_name":"Web","user_action":4}`,
		`{"public_key":"k","user_action":1}`,
		`{"public_key":"k","pc_name":"Web","user_action":1.5}`,
		`{"public_key":"k","pc_name":"Web","user_action":1,"noise":"` + strings.Repeat("x", 65<<10) + `"}`,
	} {
		// Arrange.
		b, _ := json.Marshal(map[string]any{"act_type": "syncmsgmb", "act": "user_confirm", "data": json.RawMessage(body)})
		var content ControlContent
		// Act.
		err := json.Unmarshal(b, &content)
		// Assert.
		if err != nil || !content.Data.MobileSyncInvalid || content.Data.MobileSync != nil {
			t.Fatal("invalid control accepted or data leaked")
		}
	}
}

func TestMobileSyncUnknownActionIgnored(t *testing.T) {
	// Arrange / Act: unknown data is not interpreted as usable backup metadata.
	c := mobileControl(t, "future_action", `{"public_key":"synthetic-public"}`, false)
	// Assert.
	if c.Data.MobileSync != nil {
		t.Fatal("unknown action emitted")
	}
}

func TestMobileSyncReuseClearsSensitivePriorData(t *testing.T) {
	// Arrange: previously valid control in a reused destination.
	c := mobileControl(t, "user_confirm", `{"public_key":"synthetic-public","pc_name":"Web","user_action":1}`, false)
	// Act.
	err := json.Unmarshal([]byte(`{"act_type":"syncmsgmb","act":"user_confirm","data":{}}`), &c)
	// Assert.
	if err != nil || !c.Data.MobileSyncInvalid || c.Data.MobileSync != nil {
		t.Fatal("stale sensitive control retained")
	}
}

func TestMalformedMobileControlPreservesOtherControls(t *testing.T) {
	// Arrange: malformed sync data followed by a valid control in the same frame.
	b := []byte(`{"controls":[{"content":{"act_type":"syncmsgmb","act":"user_confirm","data":{}}},{"content":{"act_type":"syncmsgmb","act":"user_confirm","data":{"public_key":"synthetic-public","pc_name":"Web","user_action":1}}}]}`)
	var page ControlEventData
	// Act.
	err := json.Unmarshal(b, &page)
	// Assert.
	if err != nil || len(page.Controls) != 2 || !page.Controls[0].Content.Data.MobileSyncInvalid || page.Controls[1].Content.Data.MobileSync == nil {
		t.Fatal("unrelated control lost")
	}
}

func TestMobileSyncMetadataBounds(t *testing.T) {
	base := `{"uid":9007199254740993,"from_seq_id":0,"public_key":"synthetic-public","url":"https://synthetic.invalid/backup","encrypted_key":"synthetic-key","file_size":16,"db_info":{}}`
	for _, tc := range []struct{ from, to string }{
		{`"uid":9007199254740993`, `"uid":9007199254740993.0`},
		{`"uid":9007199254740993`, `"uid":"09007199254740993"`},
		{`"from_seq_id":0`, `"from_seq_id":18446744073709551616`},
		{`"from_seq_id":0`, `"from_seq_id":-1`},
		{`"file_size":16`, `"file_size":536870913`},
		{`"file_size":16`, `"file_size":0`},
		{`https://synthetic.invalid/backup`, `http://synthetic.invalid/backup`},
		{`https://synthetic.invalid/backup`, `https://user@synthetic.invalid/backup`},
		{`https://synthetic.invalid/backup`, `https://synthetic.invalid/backup#fragment`},
		{`"db_info":{}`, `"db_info":"null"`},
	} {
		// Arrange.
		body := strings.Replace(base, tc.from, tc.to, 1)
		// Act.
		data, err := decodeMobileSync("syncmsg_info", []byte(body))
		// Assert: never return usable partial metadata or underlying input details.
		if err != ErrMobileSyncControl || data != nil {
			t.Fatal("invalid metadata accepted")
		}
	}
}

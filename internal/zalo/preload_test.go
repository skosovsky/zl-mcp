package zalo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type fakePreloadReader struct {
	page  *api.ConversationPreloadPage
	err   error
	calls int
}

func (f *fakePreloadReader) GetConversationPreload(context.Context) (*api.ConversationPreloadPage, error) {
	f.calls++
	return f.page, f.err
}

func TestConversationPreloadNormalizesTypedIdentitiesAndAvailableRecords(t *testing.T) {
	// Arrange: namespace collision, exact numeric IDs and both self encodings.
	source := &fakePreloadReader{page: &api.ConversationPreloadPage{
		Metadata: []json.RawMessage{json.RawMessage(`{"idTo":9007199254740993,"isGroup":0,"userName":"Synthetic peer","lastMsgId":9007199254740995}`), json.RawMessage(`{"idTo":9007199254740993,"isGroup":1}`)},
		DirectMessages: []json.RawMessage{
			json.RawMessage(`{"uidFrom":9007199254740993,"idTo":0,"msgId":9007199254740995,"ts":1791028800000,"content":"Synthetic incoming"}`),
			json.RawMessage(`{"uidFrom":"owner","idTo":9007199254740993,"msgId":9007199254740996,"cliMsgId":9007199254740998,"ts":1791028800001,"content":"Synthetic outgoing","msgType":"webchat"}`),
		},
		GroupMessages:           []json.RawMessage{json.RawMessage(`{"uidFrom":0,"idTo":9007199254740993,"msgId":9007199254740997,"ts":1791028800002,"content":"Synthetic group"}`)},
		UnsupportedMessageCount: 2,
	}}
	// Act
	result, err := readConversationPreload(context.Background(), source, "owner")
	// Assert: no storage provenance or novelty is silently assigned.
	if err != nil {
		t.Fatal(err)
	}
	if source.calls != 1 || len(result.Entries) != 2 || result.Entries[0].Conversation.Type != "direct" || result.Entries[1].Conversation.Type != "group" || result.Entries[0].Conversation.ID != "9007199254740993" || *result.Entries[0].LastMessageID != "9007199254740995" || len(result.Messages) != 3 || !result.DirectMessagesAvailable || !result.GroupMessagesAvailable || result.UnsupportedMessageCount != 2 {
		t.Fatal("preload source evidence lost")
	}
	if result.Messages[0].Direction != "incoming" || result.Messages[1].Direction != "outgoing" || result.Messages[1].SenderID != "owner" || result.Messages[1].QuoteMetadata == nil || result.Messages[1].QuoteMetadata.ClientMessageID != "9007199254740998" || result.Messages[2].Direction != "outgoing" || result.Messages[2].Ref().Type != "group" {
		t.Fatal("preload message ownership/direction lost")
	}
	for _, message := range result.Messages {
		if message.Source != "" || message.FirstIncoming != nil {
			t.Fatal("preload source chose ingestion policy")
		}
	}
}

func TestConversationPreloadRejectsAmbiguousMetadataAndForeignRecords(t *testing.T) {
	for _, metadata := range []string{`null`, `[]`, `{}`, `{"idTo":"peer"}`, `{"idTo":"peer","isGroup":true}`, `{"idTo":"peer","isGroup":2}`, `{"idTo":1e20,"isGroup":0}`, `{"idTo":"owner","isGroup":0}`} {
		// Arrange / Act
		source := &fakePreloadReader{page: &api.ConversationPreloadPage{Metadata: []json.RawMessage{json.RawMessage(metadata)}}}
		result, err := readConversationPreload(context.Background(), source, "owner")
		// Assert
		if !errors.Is(err, domain.ErrHistoryInvalidPage) || result.Entries != nil || result.Messages != nil {
			t.Fatal("ambiguous metadata returned a partial snapshot")
		}
	}
	for _, record := range []string{
		`{"uidFrom":"peer","idTo":"foreign-owner","msgId":"message","ts":"1791028800000"}`,
		`{"uidFrom":0,"idTo":0,"msgId":"message","ts":"1791028800000"}`,
		`{"uidFrom":"peer","idTo":0,"msgId":1e20,"ts":"1791028800000"}`,
		`{"uidFrom":"peer","idTo":0,"msgId":"message","ts":"invalid"}`,
	} {
		// Arrange: a valid first record cannot escape an invalid whole-page result.
		source := &fakePreloadReader{page: &api.ConversationPreloadPage{Metadata: []json.RawMessage{}, DirectMessages: []json.RawMessage{json.RawMessage(`{"uidFrom":"peer","idTo":0,"msgId":"valid","ts":"1791028800000"}`), json.RawMessage(record)}}}
		// Act
		result, err := readConversationPreload(context.Background(), source, "owner")
		// Assert
		if !errors.Is(err, domain.ErrHistoryInvalidPage) || result.Messages != nil {
			t.Fatal("foreign/invalid record returned a partial snapshot")
		}
	}
}

func TestConversationPreloadRequiresAccountAndPreservesSourceFailures(t *testing.T) {
	// Arrange / Act: reject absent ownership before an upstream call.
	source := &fakePreloadReader{}
	_, err := readConversationPreload(context.Background(), source, "")
	// Assert
	if source.calls != 0 || !errors.Is(err, domain.ErrHistoryInvalidPage) {
		t.Fatal("unbound preload source was contacted")
	}
	for _, tc := range []struct {
		source   error
		expected error
	}{{api.ErrConversationPreloadUnavailable, domain.ErrConversationPreloadUnsupported}, {errs.ErrAuthenticationRequired, domain.ErrAuthenticationRequired}} {
		source.err = tc.source
		_, err = readConversationPreload(context.Background(), source, "owner")
		if !errors.Is(err, tc.expected) {
			t.Fatal("typed preload source failure lost")
		}
	}
	// A client without this optional SDK method never invents another source.
	client := &Client{}
	_, err = client.ConversationPreload(context.Background())
	if !errors.Is(err, domain.ErrConversationPreloadUnsupported) {
		t.Fatal("absent optional source not explicit")
	}
}

func TestPreloadInvalidIdentifierReportsFieldWithoutValue(t *testing.T) {
	// Arrange: private identifiers are never diagnostic payload.
	for _, tc := range []struct{ metadata, record, reason string }{
		{`{"idTo":"peer","isGroup":0,"lastMsgId":""}`, "", "invalid_preload_last_message_identifier"},
		{`{"idTo":"peer","isGroup":0}`, `{"uidFrom":"peer","idTo":0,"msgId":"message","ts":"1791028800000","userId":" "}`, "invalid_preload_message_identifier_userId"},
	} {
		source := &fakePreloadReader{page: &api.ConversationPreloadPage{Metadata: []json.RawMessage{json.RawMessage(tc.metadata)}}}
		if tc.record != "" {
			source.page.DirectMessages = []json.RawMessage{json.RawMessage(tc.record)}
		}
		// Act
		_, err := readConversationPreload(context.Background(), source, "owner")
		// Assert
		var failure *domain.HistoryPageFailure
		if !errors.As(err, &failure) || failure.Reason() != tc.reason || !errors.Is(err, domain.ErrHistoryInvalidPage) {
			t.Fatal("safe field diagnostic missing")
		}
	}
}

func TestPreloadAllowsEmptyOptionalFieldsButRequiresMessageIdentity(t *testing.T) {
	// Arrange
	raw := json.RawMessage(`{"uidFrom":"peer","idTo":0,"msgId":"message","ts":"1791028800000","actionId":"","cliMsgId":"","userId":"","realMsgId":""}`)
	source := &fakePreloadReader{page: &api.ConversationPreloadPage{Metadata: []json.RawMessage{}, DirectMessages: []json.RawMessage{raw}}}
	// Act
	page, err := readConversationPreload(context.Background(), source, "owner")
	// Assert
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != "message" {
		t.Fatal("optional empty fields rejected valid record")
	}
	for _, field := range []string{"msgId", "uidFrom", "idTo", "ts"} {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		fields[field] = json.RawMessage(`""`)
		invalid, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		source.page.DirectMessages = []json.RawMessage{invalid}
		_, err = readConversationPreload(context.Background(), source, "owner")
		if !errors.Is(err, domain.ErrHistoryInvalidPage) {
			t.Fatal("required identity/timestamp accepted empty value")
		}
	}
}

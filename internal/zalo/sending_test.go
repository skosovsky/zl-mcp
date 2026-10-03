package zalo

import (
	"context"
	"errors"
	"testing"

	"github.com/amrakk/zcago"
	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/model"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/messaging"
)

func TestSendAdapterDoesNotTreatHTTPOrDecodeFailureAsConfirmedRejection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     *errs.ZaloErrorCode
		rejected bool
	}{
		{"missing", nil, false}, {"decode", codePointer(0), false},
		{"HTTP timeout", codePointer(408), false}, {"HTTP server error", codePointer(500), false},
		{"HTTP redirect", codePointer(302), false},
		{"protocol invalid params", codePointer(114), true},
		{"protocol denial", codePointer(-10), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, pointer := range []bool{false, true} {
				// Arrange: exercise both error shapes exposed by the dependency.
				client := Client{api: sendAPI{send: func(string, model.ThreadType, api.MessageContent) (*api.SendMessageResponse, error) {
					e := errs.NewZaloAPIError("synthetic private failure", tc.code)
					if pointer {
						return nil, &e
					}
					return nil, e
				}}}
				// Act.
				id, err := client.SendDirect(context.Background(), "peer", "synthetic", nil)
				// Assert.
				if id != "" || err == nil || errors.Is(err, messaging.ErrRejected) != tc.rejected {
					t.Fatal("send failure classification is unsafe")
				}
			}
		})
	}
}

func codePointer(n errs.ZaloErrorCode) *errs.ZaloErrorCode { return &n }

type sendAPI struct {
	zcago.API
	send func(string, model.ThreadType, api.MessageContent) (*api.SendMessageResponse, error)
}

func (s sendAPI) SendMessage(_ context.Context, id string, typ model.ThreadType, c api.MessageContent) (*api.SendMessageResponse, error) {
	return s.send(id, typ, c)
}

func TestDirectSendAdapterPreservesQuoteAndPeer(t *testing.T) {
	// Arrange.
	q := &domain.SendQuote{MessageID: "m", SenderID: "author", Text: "source", Metadata: domain.QuoteMetadata{ClientMessageID: "c", MessageType: "webchat", Timestamp: "1791029400000", TTL: 0}}
	client := Client{api: sendAPI{send: func(peer string, typ model.ThreadType, c api.MessageContent) (*api.SendMessageResponse, error) {
		// Assert wire adapter arguments, with no account or network.
		if peer != "peer" || typ != model.ThreadTypeUser || c.Msg != "reply" || c.Quote == nil || c.Quote.MsgID != "m" || c.Quote.CliMsgID != "c" || c.Quote.UIDFrom != "author" || c.Quote.Content.String == nil || *c.Quote.Content.String != "source" {
			t.Fatal("incorrect direct quote request")
		}
		return &api.SendMessageResponse{Message: &api.SendMessageResult{MsgID: "accepted"}}, nil
	}}}
	// Act.
	id, err := client.SendDirect(context.Background(), "peer", "reply", q)
	// Assert.
	if err != nil || id != "accepted" {
		t.Fatal("upstream acceptance lost", err)
	}
}

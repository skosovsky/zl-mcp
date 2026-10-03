package zalo

import (
	"context"
	"errors"

	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/model"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/messaging"
)

func (c *Client) SendDirect(ctx context.Context, peer, text string, q *domain.SendQuote) (string, error) {
	content := api.MessageContent{Msg: text}
	if q != nil {
		content.Quote = &api.SendMessageQuote{MsgID: q.MessageID, CliMsgID: q.Metadata.ClientMessageID, MsgType: q.Metadata.MessageType, UIDFrom: q.SenderID, Content: model.Content{String: &q.Text}, TS: q.Metadata.Timestamp, TTL: q.Metadata.TTL}
	}
	r, err := c.api.SendMessage(ctx, peer, model.ThreadTypeUser, content)
	if err != nil {
		var value errs.ZaloAPIError
		var pointer *errs.ZaloAPIError
		if errors.As(err, &value) {
			if confirmedRejection(value.Code) {
				return "", messaging.ErrRejected
			}
		} else if errors.As(err, &pointer) && pointer != nil {
			if confirmedRejection(pointer.Code) {
				return "", messaging.ErrRejected
			}
		}
		return "", errors.New("upstream send result is ambiguous")
	}
	if r == nil || r.Message == nil || r.Message.MsgID == "" {
		return "", errors.New("upstream send result is ambiguous")
	}
	return r.Message.MsgID, nil
}

// zcago collapses response parsing failures (code 0) and HTTP statuses into
// ZaloAPIError. Only protocol codes distinguishable from HTTP are evidence of
// rejection; invalid parameters is the documented protocol code exception.
func confirmedRejection(code *errs.ZaloErrorCode) bool {
	return code != nil && *code != 0 && (*code == errs.ZaloErrorCodeInvalidParams || *code < 100 || *code > 599)
}

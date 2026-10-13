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
	//nolint:exhaustruct_v5 // MessageContent preserves optional upstream fields absent from this request/projection.
	content := api.MessageContent{Msg: text}
	if q != nil {
		//nolint:exhaustruct_v5 // SendMessageQuote preserves optional upstream fields absent from this request/projection.
		content.Quote = &api.SendMessageQuote{
			MsgID:    q.MessageID,
			CliMsgID: q.Metadata.ClientMessageID,
			MsgType:  q.Metadata.MessageType,
			UIDFrom:  q.SenderID,
			//nolint:exhaustruct_v5 // Content preserves optional upstream fields absent from this request/projection.
			Content: model.Content{String: &q.Text},
			TS:      q.Metadata.Timestamp,
			TTL:     q.Metadata.TTL,
		}
	}
	r, err := c.api.SendMessage(ctx, peer, model.ThreadTypeUser, content)
	if err != nil {
		if confirmedSendFailure(err) {
			return "", messaging.ErrRejected
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

func confirmedSendFailure(err error) bool {
	if value, ok := errors.AsType[errs.ZaloAPIError](err); ok {
		return confirmedRejection(value.Code)
	}
	var pointer *errs.ZaloAPIError
	return errors.As(err, &pointer) && pointer != nil && confirmedRejection(pointer.Code)
}

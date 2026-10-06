package zalo

import (
	"context"
	"errors"
	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/model"
)

// UndoDiagnosticDirect uses the existing API session; the owner control validates
// the exact permitted diagnostic operation before invoking this adapter.
func (c *Client) UndoDiagnosticDirect(ctx context.Context, peer, message, client string) (int, error) {
	result, err := c.api.UndoMessage(ctx, peer, model.ThreadTypeUser, api.UndoMessageData{MsgID: message, CliMsgID: client})
	if err != nil || result == nil {
		return 0, errors.New("recall result is ambiguous")
	}
	return result.Status, nil
}

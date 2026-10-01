package zalo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/listener"
	"github.com/amrakk/zcago/model"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"log/slog"
	"strconv"
)

// Listen never retries membership mutations; reconnect policy belongs to collector.
func (c *Client) Listen(ctx context.Context, onMessage func(domain.Message) error, onDelete func(string, string) error, onConnected func() error) error {
	ln := c.api.Listener()
	if e := ln.Start(ctx, false); e != nil {
		return listenerFailure(e, "listener connection failed")
	}
	defer ln.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ln.Connected():
			if e := onConnected(); e != nil {
				return e
			}
			if e := ln.RequestOldMessages(ctx, model.ThreadTypeGroup, nil); e != nil {
				return listenerFailure(e, "replay request failed")
			}
		case info := <-ln.Disconnected():
			slog.Warn("zalo_listener_disconnected", "close_code", info.Code)
			return listenerCloseFailure(info.Code, "listener disconnected")
		case info := <-ln.Closed():
			slog.Warn("zalo_listener_closed", "close_code", info.Code)
			return listenerCloseFailure(info.Code, "listener closed")
		case err := <-ln.Error():
			logListenerError(err)
			return listenerFailure(err, "listener reported an error")
		case v := <-ln.Message():
			if g, ok := v.(model.GroupMessage); ok {
				m, e := Convert(g, "live")
				if e != nil {
					return e
				}
				if e = onMessage(m); e != nil {
					return e
				}
			}
		case old := <-ln.OldMessages():
			for _, v := range old.Messages {
				if g, ok := v.(model.GroupMessage); ok {
					m, e := Convert(g, "replay")
					if e != nil {
						return e
					}
					if e = onMessage(m); e != nil {
						return e
					}
				}
			}
		case undo := <-ln.Undo():
			if undo.IsGroup {
				if e := onDelete(undo.ThreadID, strconv.FormatInt(undo.Data.Content.GlobalMsgID, 10)); e != nil {
					return e
				}
			}
		case <-ln.Group():
		case <-ln.Friend():
		case <-ln.Reaction():
		case <-ln.OldReactions():
		case <-ln.Typing():
		case <-ln.DeliveredMessages():
		case <-ln.SeenMessages():
		case <-ln.UploadAttachment():
		case <-ln.CipherKey():
		}
	}
}

// 3003 is a server kick, reproduced live after the user revoked all sessions.
// A fresh local login is required; it is distinct from duplicate connection 3000.
func listenerCloseFailure(code int, message string) error {
	if code == listener.ZaloKickConnection {
		return domain.ErrAuthenticationRequired
	}
	return errors.New(message)
}

// Diagnostics expose error classes and schema fields, never upstream messages or payloads.
func logListenerError(err error) {
	for depth := 0; err != nil && depth < 6; depth++ {
		fields := []any{"error_type", fmt.Sprintf("%T", err)}
		switch e := err.(type) {
		case errs.ZCAError:
			switch e.Op {
			case "listener.handleGroupMessages", "listener.handleOldMessages", "listener.handleActions", "listener.handleControls":
				fields = append(fields, "operation", e.Op)
			}
		case *json.UnmarshalTypeError:
			fields = append(fields, "json_field", e.Field, "expected_type", e.Type.String())
		}
		slog.Warn("zalo_listener_error", fields...)
		err = errors.Unwrap(err)
	}
}

// Preserve only an explicit authentication signal; upstream error text stays private.
func listenerFailure(err error, message string) error {
	if errors.Is(err, errs.ErrAuthenticationRequired) {
		return domain.ErrAuthenticationRequired
	}
	return errors.New(message)
}

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
	"time"
)

// Listen never retries membership mutations; reconnect policy belongs to collector.
func (c *Client) Listen(ctx context.Context, onMessage func(domain.Message) error, onDelete func(string, string) error, onConnected func() error) error {
	return c.listenConversations(ctx, func(m domain.Message) error {
		if m.Ref().Type == domain.ConversationGroup {
			return onMessage(m)
		}
		return nil
	}, func(ref domain.ConversationRef, id string) error {
		if ref.Type == domain.ConversationGroup {
			return onDelete(ref.ID, id)
		}
		return nil
	}, onConnected, false)
}

func (c *Client) ListenConversations(ctx context.Context, onMessage func(domain.Message) error, onDelete func(domain.ConversationRef, string) error, onConnected func() error) error {
	return c.listenConversations(ctx, onMessage, onDelete, onConnected, true)
}

func (c *Client) listenConversations(ctx context.Context, onMessage func(domain.Message) error, onDelete func(domain.ConversationRef, string) error, onConnected func() error, includeDirect bool) error {
	ln := c.api.Listener()
	if e := ln.Start(ctx, false); e != nil {
		return listenerFailure(e, "listener connection failed")
	}
	defer ln.Stop()
	diagnostics := time.NewTicker(15 * time.Second)
	defer diagnostics.Stop()
	pager := newReplayPager(includeDirect)
	for {
		select {
		case <-diagnostics.C:
			logIngestionDiagnostics(ln)
		case <-ctx.Done():
			return ctx.Err()
		case <-ln.Connected():
			if e := onConnected(); e != nil {
				return e
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
			m, e := convertIncoming(v, "live")
			if e != nil {
				return e
			}
			if e = onMessage(m); e != nil {
				return e
			}
		case old := <-ln.OldMessages():
			for _, v := range old.Messages {
				m, e := convertIncoming(v, "replay")
				if e != nil {
					return e
				}
				if e = onMessage(m); e != nil {
					return e
				}
			}
			if e := pager.Continue(ctx, ln, old.Replay); e != nil {
				return listenerFailure(e, "replay continuation failed")
			}
		case undo := <-ln.Undo():
			kind := domain.ConversationDirect
			if undo.IsGroup {
				kind = domain.ConversationGroup
			}
			if e := onDelete(domain.ConversationRef{Type: kind, ID: undo.ThreadID}, strconv.FormatInt(undo.Data.Content.GlobalMsgID, 10)); e != nil {
				return e
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
			pager = newReplayPager(includeDirect)
			types := []model.ThreadType{model.ThreadTypeGroup}
			if includeDirect {
				types = append(types, model.ThreadTypeUser)
			}
			for _, kind := range types {
				if e := ln.RequestOldMessages(ctx, kind, nil); e != nil {
					return listenerFailure(e, "replay request failed")
				}
			}
		}
	}
}

func convertIncoming(v model.Message, source string) (domain.Message, error) {
	switch m := v.(type) {
	case model.GroupMessage:
		return Convert(m, source)
	case model.UserMessage:
		return ConvertDirect(m, source)
	default:
		return domain.Message{}, errors.New("unsupported upstream conversation message")
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

// logIngestionDiagnostics uses an optional dependency extension; it does not
// change the public MCP contract or include message contents or account data.
func logIngestionDiagnostics(ln listener.Listener) {
	source, ok := ln.(interface{ Diagnostics() listener.Diagnostics })
	if !ok {
		return
	}
	d := source.Diagnostics()
	slog.Info("zalo_ingestion_pressure", "direct_frames", d.DirectFrames, "decoded_direct", d.DecodedDirect, "emitted_direct", d.EmittedDirect, "direct_errors", d.DirectErrors, "group_errors", d.GroupErrors, "replay_errors", d.ReplayErrors, "backpressure", d.Backpressure, "cancelled_emissions", d.CancelledEmissions)
	slog.Info("zalo_ingestion_diagnostics", "frames", d.Frames, "non_binary", d.NonBinary, "unhandled", d.Unhandled, "cipher_keys", d.CipherKeys, "group_frames", d.GroupFrames, "replay_frames", d.ReplayFrames, "decoded_groups", d.DecodedGroups, "emitted_groups", d.EmittedGroups, "errors", d.Errors, "last_version", d.LastVersion, "last_command", d.LastCommand, "last_subcommand", d.LastSubcommand, "last_unhandled_command", d.LastUnhandledCommand, "replay_users", d.ReplayUsers, "replay_code", d.ReplayCode, "replay_shape", d.ReplayShape)
}

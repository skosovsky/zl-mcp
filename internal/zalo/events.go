package zalo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/listener"
	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// Listen never retries membership mutations; reconnect policy belongs to collector.
func (c *Client) Listen(
	ctx context.Context,
	onMessage func(domain.Message) error,
	onDelete func(string, string) error,
	onConnected func() error,
) error {
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
	}, onConnected, false, nil)
}

func (c *Client) ListenConversations(
	ctx context.Context,
	onMessage func(domain.Message) error,
	onDelete func(domain.ConversationRef, string) error,
	onConnected func() error,
) error {
	return c.listenConversations(ctx, onMessage, onDelete, onConnected, true, nil)
}

// ListenConversationActivities shares the same upstream listener and session.
func (c *Client) ListenConversationActivities(
	ctx context.Context,
	onMessage func(domain.Message) error,
	onDelete func(domain.ConversationRef, string) error,
	onConnected func() error,
	onActivity func(domain.Activity) error,
) error {
	return c.listenConversations(ctx, onMessage, onDelete, onConnected, true, onActivity)
}

//nolint:cyclop,gocognit,gocyclo // The closed native channel dispatch keeps terminal events and callbacks within one Start/Stop lifetime; message normalization, replay batches and initial requests are separate handlers.
func (c *Client) listenConversations(
	ctx context.Context,
	onMessage func(domain.Message) error,
	onDelete func(domain.ConversationRef, string) error,
	onConnected func() error,
	includeDirect bool,
	onActivity func(domain.Activity) error,
) error {
	ln := c.api.Listener()
	if e := ln.Start(ctx, false); e != nil {
		return listenerFailure(e, "listener connection failed")
	}
	defer ln.Stop()
	diagnostics := time.NewTicker(listenerDiagnosticsInterval)
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
			//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
			slog.WarnContext(ctx, "zalo_listener_disconnected", "close_code", info.Code)
			return listenerCloseFailure(info.Code, "listener disconnected")
		case info := <-ln.Closed():
			//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
			slog.WarnContext(ctx, "zalo_listener_closed", "close_code", info.Code)
			return listenerCloseFailure(info.Code, "listener closed")
		case err := <-ln.Error():
			logListenerError(err)
			return listenerFailure(err, "listener reported an error")
		case v := <-ln.Message():
			if deliveryErr := deliverIncomingActivity(v, "live", onMessage, onActivity); deliveryErr != nil {
				return deliveryErr
			}
		case old := <-ln.OldMessages():
			for _, message := range old.Messages {
				if replayErr := deliverIncomingActivity(message, "replay", onMessage, onActivity); replayErr != nil {
					return replayErr
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
			if e := onDelete(
				domain.ConversationRef{Type: kind, ID: undo.ThreadID},
				strconv.FormatInt(undo.Data.Content.GlobalMsgID, 10),
			); e != nil {
				return e
			}
		case <-ln.Group():
		case <-ln.Friend():
		case reaction := <-ln.Reaction():
			if err := deliverReaction(reaction, "live", onActivity); err != nil {
				return err
			}
		case reactions := <-ln.OldReactions():
			for _, reaction := range reactions.Reactions {
				if err := deliverReaction(reaction, "replay", onActivity); err != nil {
					return err
				}
			}

		case <-ln.Typing():
		case <-ln.DeliveredMessages():
		case <-ln.SeenMessages():
		case <-ln.UploadAttachment():
		case <-ln.CipherKey():
			pager = newReplayPager(includeDirect)
			if requestErr := requestInitialReplay(ctx, ln, includeDirect); requestErr != nil {
				return requestErr
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
		//nolint:errorlint // Inspect only this error's concrete schema; errors.As can invoke upstream callbacks.
		switch e := err.(type) {
		case errs.ZCAError:
			switch e.Op {
			case "listener.handleGroupMessages", "listener.handleOldMessages",
				"listener.handleActions", "listener.handleControls":
				fields = append(fields, "operation", e.Op)
			}
		case *json.UnmarshalTypeError:
			fields = append(fields, "json_field", e.Field, "expected_type", e.Type.String())
		}
		//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
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
	//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
	slog.Info(
		"zalo_ingestion_pressure",
		"direct_frames",
		d.DirectFrames,
		"decoded_direct",
		d.DecodedDirect,
		"emitted_direct",
		d.EmittedDirect,
		"direct_errors",
		d.DirectErrors,
		"group_errors",
		d.GroupErrors,
		"replay_errors",
		d.ReplayErrors,
		"backpressure",
		d.Backpressure,
		"cancelled_emissions",
		d.CancelledEmissions,
		"ignored_actions", d.IgnoredActions,
		"ignored_controls", d.IgnoredControls,
		"unhandled_header_overflow", d.UnhandledHeaderOverflow,
	)
	for _, header := range d.UnhandledHeaders {
		//nolint:sloglint // Numeric protocol headers only; no body, identities or arbitrary action strings.
		slog.Info("zalo_unhandled_header", "version", header.Version, "command", header.Command,
			"subcommand", header.Subcommand, "count", header.Count)
	}
	//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
	slog.Info(
		"zalo_ingestion_diagnostics",
		"frames",
		d.Frames,
		"non_binary",
		d.NonBinary,
		"unhandled",
		d.Unhandled,
		"cipher_keys",
		d.CipherKeys,
		"group_frames",
		d.GroupFrames,
		"replay_frames",
		d.ReplayFrames,
		"decoded_groups",
		d.DecodedGroups,
		"emitted_groups",
		d.EmittedGroups,
		"errors",
		d.Errors,
		"last_version",
		d.LastVersion,
		"last_command",
		d.LastCommand,
		"last_subcommand",
		d.LastSubcommand,
		"last_unhandled_command",
		d.LastUnhandledCommand,
		"replay_users",
		d.ReplayUsers,
		"replay_code",
		d.ReplayCode,
		"replay_shape",
		d.ReplayShape,
	)
}

func deliverIncoming(message model.Message, source string, onMessage func(domain.Message) error) error {
	normalized, err := convertIncoming(message, source)
	if isNonMessageRecord(err) {
		logSkippedNonMessage(source, err)
		return nil
	}
	if err != nil {
		return err
	}
	return onMessage(normalized)
}

func deliverReplay(messages []model.Message, onMessage func(domain.Message) error) error {
	for _, message := range messages {
		if err := deliverIncoming(message, "replay", onMessage); err != nil {
			return err
		}
	}
	return nil
}

type oldMessageRequester interface {
	RequestOldMessages(context.Context, model.ThreadType, *string) error
}

func requestInitialReplay(ctx context.Context, ln oldMessageRequester, includeDirect bool) error {
	types := []model.ThreadType{model.ThreadTypeGroup}
	if includeDirect {
		types = append(types, model.ThreadTypeUser)
	}
	for _, kind := range types {
		if e := ln.RequestOldMessages(ctx, kind, nil); e != nil {
			return listenerFailure(e, "replay request failed")
		}
	}
	return nil
}

package listener

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/internal/websocketx"
	"github.com/amrakk/zcago/model"
)

// ----------------------------------------
// WebSocket sending utilities
// ----------------------------------------

type WSPayload struct {
	Version uint8
	CMD     uint16
	SubCMD  uint8
	Data    map[string]any
}

func (ln *listener) SendWS(ctx context.Context, p WSPayload, requireID bool) error {
	if err := ln.validateSendRequest(ctx); err != nil {
		return err
	}

	client := ln.getClient()
	if client == nil {
		return errs.NewZCA("listener not started", "listener.SendWS")
	}

	if requireID {
		ln.addRequestID(&p)
	}

	frame, err := encodeFrame(p)
	if err != nil {
		return errs.WrapZCA("failed to encode frame", "listener.SendWS", err)
	}

	return client.Write(ctx, websocketx.BinaryMessage, frame)
}

func (ln *listener) RequestOldMessages(ctx context.Context, tt model.ThreadType, lastMsgID *string) error {
	return ln.RequestReplayPage(ctx, tt, true, lastMsgID)
}

// RequestReplayPage continues the existing offline queue on the same socket.
// The lastId cursor is the response lastActionId, not a global message ID.
func (ln *listener) RequestReplayPage(ctx context.Context, tt model.ThreadType, first bool, lastActionID *string) error {
	cmd := uint16(510)
	if tt == model.ThreadTypeGroup {
		cmd = 511
	}
	data := map[string]any{
		"first":  first,
		"lastId": lastActionID,
		"preIds": []string{},
	}

	return ln.SendWS(ctx, WSPayload{
		Version: 1,
		CMD:     cmd,
		SubCMD:  1,
		Data:    data,
	}, true)
}

func (ln *listener) RequestOldReactions(ctx context.Context, tt model.ThreadType, lastMsgID *string) error {
	cmd := uint16(610)
	if tt == model.ThreadTypeGroup {
		cmd = 611
	}
	data := map[string]any{
		"first":  true,
		"lastId": lastMsgID,
		"preIds": []string{},
	}

	return ln.SendWS(ctx, WSPayload{
		Version: 1,
		CMD:     cmd,
		SubCMD:  1,
		Data:    data,
	}, true)
}

func (ln *listener) validateSendRequest(ctx context.Context) error {
	if ln == nil {
		return errs.NewZCA("listener is nil", "listener.validateSendRequest")
	}
	if ctx == nil {
		return errs.NewZCA("context is nil", "listener.validateSendRequest")
	}
	if ctx.Err() != nil {
		err := ctx.Err()
		return errs.WrapZCA("context cancelled", "listener.validateSendRequest", err)
	}
	return nil
}

func (ln *listener) addRequestID(p *WSPayload) {
	ln.mu.Lock()
	defer ln.mu.Unlock()

	if p.Data == nil {
		p.Data = map[string]any{}
	}
	p.Data["req_id"] = "req_" + fmt.Sprint(ln.reqID)
	ln.reqID++
}

// ----------------------------------------
// Websocket reading utilities
// ----------------------------------------

type WSMessage[T any] struct {
	// DecodeShape is a safe classification of the decoded JSON envelope.
	DecodeShape  uint    `json:"-"`
	Key          *string `json:"key"`
	Encrypt      uint    `json:"encrypt"`
	ErrorCode    int     `json:"error_code"`
	ErrorMessage string  `json:"error_message"`
	Data         T       `json:"data"`
}

type BaseWSMessage = WSMessage[string]

func (ln *listener) handleWebSocketMessage(ctx context.Context, msg websocketx.Message) {
	ln.diagnostics.frames.Add(1)
	if msg.Type != websocketx.BinaryMessage {
		ln.diagnostics.nonBinary.Add(1)
		return
	}

	version, cmd, subCMD, data, err := parseWebSocketMessage(msg.Data)
	if err != nil {
		ln.emitError(ctx, err)
		return
	}

	ln.diagnostics.lastVersion.Store(uint64(version))
	ln.diagnostics.lastCommand.Store(uint64(cmd))
	ln.diagnostics.lastSubcommand.Store(uint64(subCMD))
	var parsed BaseWSMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		ln.emitError(ctx, errs.WrapZCA("failed to parse message JSON", "listener.handleWebSocketMessage", err))
		return
	}

	ln.router(ctx, uint(version), uint(cmd), uint(subCMD), parsed)
}

func parseWebSocketMessage(data []byte) (byte, uint16, byte, []byte, error) {
	if len(data) < 4 {
		return 0, 0, 0, nil, errs.NewZCA("message too short", "listener.parseWebSocketMessage")
	}

	header := make([]byte, 4)
	copy(header, data[:4])

	version, cmd, subCMD, err := getMessageHeader(header)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	msgData := data[4:]
	if len(msgData) == 0 {
		return 0, 0, 0, nil, errs.NewZCA("empty message data", "listener.parseWebSocketMessage")
	}

	return version, cmd, subCMD, msgData, nil
}

func getMessageHeader(buffer []byte) (byte, uint16, byte, error) {
	if len(buffer) < 4 {
		return 0, 0, 0, errs.NewZCA("invalid header", "listener.getHeader")
	}

	version := buffer[0]
	cmd := binary.LittleEndian.Uint16(buffer[1:3])
	subCMD := buffer[3]

	return version, cmd, subCMD, nil
}

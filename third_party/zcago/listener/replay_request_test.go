package listener

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/amrakk/zcago/internal/websocketx"
	"github.com/amrakk/zcago/model"
	"github.com/coder/websocket"
)

type replayWireCapture struct {
	websocketx.Client
	frames [][]byte
}

func (s *replayWireCapture) Write(_ context.Context, _ websocket.MessageType, body []byte) error {
	s.frames = append(s.frames, append([]byte(nil), body...))
	return nil
}

func TestReplayRequestsPreserveFirstCompatibilityAndUseActionCursor(t *testing.T) {
	// Arrange
	socket := &replayWireCapture{}
	ln := &listener{client: socket}
	ctx := context.Background()
	id := "90071992547409931234"
	// Act
	if err := ln.RequestOldMessages(ctx, model.ThreadTypeUser, nil); err != nil {
		t.Fatal(err)
	}
	if err := ln.RequestReplayPage(ctx, model.ThreadTypeUser, false, &id); err != nil {
		t.Fatal(err)
	}
	if err := ln.RequestReplayPage(ctx, model.ThreadTypeGroup, false, &id); err != nil {
		t.Fatal(err)
	}
	// Assert
	for i, frame := range socket.frames {
		var data map[string]any
		if err := json.Unmarshal(frame[4:], &data); err != nil {
			t.Fatal(err)
		}
		cmd := uint16(510)
		if i == 2 {
			cmd = 511
		}
		if frame[0] != 1 || binary.LittleEndian.Uint16(frame[1:3]) != cmd || frame[3] != 1 || data["first"] != (i == 0) {
			t.Fatal("wrong queue or first flag")
		}
		if (i == 0 && data["lastId"] != nil) || (i > 0 && data["lastId"] != id) {
			t.Fatal("cursor changed or interpreted as a number")
		}
		if data["req_id"] == nil || len(data["preIds"].([]any)) != 0 {
			t.Fatal("missing request ID or changed preIds contract")
		}
	}
	if len(socket.frames) != 3 || ln.reqID != 3 {
		t.Fatal("request ID allocator not used")
	}
}

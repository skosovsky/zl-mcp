package listener

import (
	"context"
	"github.com/amrakk/zcago/internal/websocketx"
	"github.com/amrakk/zcago/session"
	"testing"
)

func TestDiagnosticsRecognizeIgnoredFramesWithoutPayload(t *testing.T) {
	// Arrange
	ln := &listener{}
	// Act: text frames and unknown binary commands previously disappeared silently.
	ln.handleWebSocketMessage(context.Background(), websocketx.Message{Type: websocketx.TextMessage, Data: []byte("SYNTHETIC_SECRET")})
	ln.handleWebSocketMessage(context.Background(), websocketx.Message{Type: websocketx.BinaryMessage, Data: []byte{1, 0xff, 0xff, 9, '{', '}'}})
	d := ln.Diagnostics()
	// Assert
	if d.Frames != 2 || d.NonBinary != 1 || d.Unhandled != 1 || d.LastCommand != 65535 || d.LastSubcommand != 9 {
		t.Fatalf("incorrect counters: %+v", d)
	}
}

func TestDiagnosticsSeparateDirectAndGroupMessages(t *testing.T) {
	// Arrange: use both live protocol categories with synthetic identities.
	ln := &listener{sc: session.NewContext(), ch: initializeChannels()}
	// Act.
	ln.handleMessages(context.Background(), BaseWSMessage{Data: `{"data":{"msgs":[{"msgId":"d","uidFrom":"peer","idTo":"0","content":"direct"}]}}`})
	ln.handleGroupMessages(context.Background(), BaseWSMessage{Data: `{"data":{"groupMsgs":[{"msgId":"g","uidFrom":"peer","idTo":"group","content":"group"}]}}`})
	d := ln.Diagnostics()
	// Assert: direct events cannot inflate the group counter.
	if d.DirectFrames != 1 || d.GroupFrames != 1 || d.DecodedDirect != 1 || d.DecodedGroups != 1 || d.EmittedDirect != 1 || d.EmittedGroups != 1 || d.Errors != 0 {
		t.Fatalf("incorrect category counters: %+v", d)
	}
}

func TestDiagnosticsAttributeKnownDecodeFailures(t *testing.T) {
	// Arrange: invalid bodies for known live categories and an untyped replay.
	ln := &listener{sc: session.NewContext(), ch: initializeChannels()}
	body := BaseWSMessage{Data: "invalid synthetic envelope"}
	// Act.
	ln.handleMessages(context.Background(), body)
	ln.handleGroupMessages(context.Background(), body)
	ln.handleOldMessages(context.Background(), body)
	d := ln.Diagnostics()
	// Assert: malformed mixed replay is not falsely assigned to a live type.
	if d.DirectErrors != 1 || d.GroupErrors != 1 || d.ReplayErrors != 1 || d.Errors != 3 {
		t.Fatalf("incorrect decode attribution: %+v", d)
	}
}

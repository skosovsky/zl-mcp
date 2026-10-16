package listener

import (
	"context"
	"encoding/json"
	"github.com/amrakk/zcago/internal/websocketx"
	"github.com/amrakk/zcago/session"
	"strings"
	"sync"
	"testing"
)

func TestUnhandledHeaderDiagnosticsAreBoundedAndDetached(t *testing.T) {
	// Arrange: arbitrary body is never decoded or retained by header diagnostics.
	ln := &listener{}
	// Act: exceed the distinct-header budget, then repeat an admitted header.
	for i := uint(0); i < 20; i++ {
		ln.router(context.Background(), 1, 800+i, 7, BaseWSMessage{Data: "SYNTHETIC_PRIVATE_BODY"})
	}
	ln.router(context.Background(), 1, 800, 7, BaseWSMessage{})
	d := ln.Diagnostics()
	encoded, err := json.Marshal(d)
	// Assert: fixed memory budget, overflow explicit, no body and no shared slice.
	if err != nil || len(d.UnhandledHeaders) != unhandledHeaderLimit || d.UnhandledHeaderOverflow != 4 || d.UnhandledHeaders[0].Count != 2 || strings.Contains(string(encoded), "SYNTHETIC_PRIVATE_BODY") {
		t.Fatal("unbounded or sensitive header diagnostics", err)
	}
	d.UnhandledHeaders[0].Count = 999
	if ln.Diagnostics().UnhandledHeaders[0].Count != 2 {
		t.Fatal("snapshot aliases mutable counters")
	}
}

func TestUnhandledHeaderDiagnosticsConcurrentSnapshot(t *testing.T) {
	// Arrange.
	ln := &listener{}
	var workers sync.WaitGroup
	// Act: writer and snapshot operations must share a safe bounded buffer.
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); ln.diagnostics.recordUnhandled(1, 800, 0); _ = ln.Diagnostics() }()
	}
	workers.Wait()
	// Assert.
	d := ln.Diagnostics()
	if len(d.UnhandledHeaders) != 1 || d.UnhandledHeaders[0].Count != 20 {
		t.Fatal("concurrent header counts lost")
	}
}

func TestUnknownActionsAndControlsAreCountedWithoutTheirValues(t *testing.T) {
	// Arrange.
	ln := &listener{sc: session.NewContext(), ch: initializeChannels()}
	// Act: ignored data is not labelled a call or published as a message.
	ln.handleActions(context.Background(), BaseWSMessage{Data: `{"data":{"actions":[{"act_type":"SYNTHETIC_PRIVATE_ACTION","data":{}}]}}`})
	ln.handleControls(context.Background(), BaseWSMessage{Data: `{"data":{"controls":[{"content":{"act_type":"SYNTHETIC_PRIVATE_CONTROL","data":{}}}]}}`})
	d := ln.Diagnostics()
	encoded, err := json.Marshal(d)
	// Assert.
	if err != nil || d.IgnoredActions != 1 || d.IgnoredControls != 1 || strings.Contains(string(encoded), "SYNTHETIC_PRIVATE") {
		t.Fatal("ignored records not safely counted", err)
	}
}

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

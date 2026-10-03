package listener

import (
	"context"
	"github.com/amrakk/zcago/model"
	"github.com/amrakk/zcago/session"
	"testing"
)

func TestMixedReplayPreservesBothKinds(t *testing.T) {
	// Arrange
	ln := &listener{sc: session.NewContext(), ch: initializeChannels()}
	body := BaseWSMessage{Data: `{"data":{"msgs":[{"msgId":"d","uidFrom":"peer","idTo":"0","content":"direct"}],"groupMsgs":[{"msgId":"g","uidFrom":"peer","idTo":"group","content":"group"}]}}`}
	// Act
	ln.handleOldMessages(context.Background(), body)
	// Assert
	seen := map[model.ThreadType]int{}
	for i := 0; i < 2; i++ {
		select {
		case batch := <-ln.OldMessages():
			for _, msg := range batch.Messages {
				if msg.Type() != batch.ThreadType {
					t.Fatal("mixed batch type")
				}
				seen[msg.Type()]++
			}
		default:
			t.Fatal("replay batch discarded")
		}
	}
	if seen[model.ThreadTypeUser] != 1 || seen[model.ThreadTypeGroup] != 1 {
		t.Fatal("both kinds were not preserved")
	}
}

func TestReplayMetadataEmitsOnceAfterMixedPageAndPreservesEmptyQueue(t *testing.T) {
	// Arrange
	ln := &listener{sc: session.NewContext(), ch: initializeChannels()}
	body := BaseWSMessage{Data: `{"data":{"more":1,"lastActionId":90071992547409931234,"msgs":[{"msgId":"d","uidFrom":"peer","idTo":"0","content":"direct"}],"groupMsgs":[{"msgId":"g","uidFrom":"peer","idTo":"group","content":"group"}]}}`}
	// Act
	ln.handleOldMessagesFor(context.Background(), body, model.ThreadTypeGroup)
	first := <-ln.OldMessages()
	second := <-ln.OldMessages()
	// Assert
	if first.Replay != nil || second.Replay == nil || second.Replay.Queue != model.ThreadTypeGroup || second.Replay.LastActionID != "90071992547409931234" || second.Replay.More == nil || !*second.Replay.More || second.Replay.MessageCount != 2 {
		t.Fatal("continuation duplicated, lost or assigned to message type instead of queue")
	}
	// Act / Assert: a response with no messages still preserves its group queue.
	ln.handleOldMessagesFor(context.Background(), BaseWSMessage{Data: `{"data":{"more":0,"lastActionId":"12","groupMsgs":[]}}`}, model.ThreadTypeGroup)
	empty := <-ln.OldMessages()
	if len(empty.Messages) != 0 || empty.ThreadType != model.ThreadTypeGroup || empty.Replay == nil || empty.Replay.More == nil || *empty.Replay.More {
		t.Fatal("empty queue metadata lost")
	}
	// Act / Assert: invalid continuation cannot discard otherwise valid records.
	ln.handleOldMessages(context.Background(), BaseWSMessage{Data: `{"data":{"more":"bad","msgs":[{"msgId":"d2","uidFrom":"peer","content":"valid"}]}}`})
	invalid := <-ln.OldMessages()
	if len(invalid.Messages) != 1 || invalid.Replay.Valid {
		t.Fatal("metadata failure swallowed valid records")
	}
}

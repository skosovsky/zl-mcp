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

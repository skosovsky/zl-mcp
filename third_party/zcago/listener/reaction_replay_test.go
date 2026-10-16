package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/amrakk/zcago/model"
	"github.com/amrakk/zcago/session"
)

func TestMixedReactionReplayPreservesBothNamespaces(t *testing.T) {
	// Arrange: one replay envelope can contain both direct and group reactions.
	ln := &listener{sc: session.NewContext(), ch: initializeChannels(), selfListen: true}
	body := BaseWSMessage{Data: `{"data":{"reacts":[{"msgId":"direct","uidFrom":"peer","idTo":"0","content":""}],"reactGroups":[{"msgId":"group","uidFrom":"peer","idTo":"group","content":""}]}}`}
	// Act.
	ln.handleOldReactions(context.Background(), body)
	// Assert: independent homogeneous batches, neither branch displaces the other.
	seen := map[model.ThreadType]int{}
	for range 2 {
		select {
		case batch := <-ln.OldReactions():
			for _, reaction := range batch.Reactions {
				if reaction.Type != batch.ThreadType {
					t.Fatal("reaction namespace mixed")
				}
				seen[reaction.Type]++
			}
		default:
			t.Fatal("reaction batch dropped")
		}
	}
	if seen[model.ThreadTypeUser] != 1 || seen[model.ThreadTypeGroup] != 1 {
		t.Fatal("mixed replay lost a namespace")
	}
}

func TestReactionBurstBackpressureDoesNotEvictEarlierEvents(t *testing.T) {
	// Arrange: a deliberately tiny buffer and a larger native batch.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln := &listener{sc: session.NewContext(), ch: initializeChannels(), selfListen: true}
	ln.ch.Reaction = make(chan model.Reaction, 1)
	data := []map[string]string{}
	for i := range 100 {
		data = append(data, map[string]string{"msgId": fmt.Sprint(i), "uidFrom": "peer", "idTo": "0", "content": ""})
	}
	wire, err := json.Marshal(map[string]any{"data": map[string]any{"reacts": data}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	// Act.
	go func() { defer close(done); ln.handleReactions(ctx, BaseWSMessage{Data: string(wire)}) }()
	// Assert: every ID arrives in order, cancellation releases a blocked producer.
	for i := range 100 {
		select {
		case reaction := <-ln.Reaction():
			if reaction.Data.MsgID != fmt.Sprint(i) {
				t.Fatal("reaction evicted or reordered")
			}
		case <-ctx.Done():
			t.Fatal("reaction burst stalled")
		}
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("reaction producer did not exit")
	}
}

func TestEmptyReactionReplayStillSignalsResponse(t *testing.T) {
	// Arrange / Act: preserve the prior empty-response signal.
	ln := &listener{sc: session.NewContext(), ch: initializeChannels()}
	ln.handleOldReactions(context.Background(), BaseWSMessage{Data: `{"data":{"reacts":[],"reactGroups":[]}}`})
	// Assert.
	select {
	case batch := <-ln.OldReactions():
		if len(batch.Reactions) != 0 {
			t.Fatal("empty replay invented reactions")
		}
	default:
		t.Fatal("empty replay response disappeared")
	}
}

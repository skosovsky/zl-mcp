package listener

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/amrakk/zcago/session"
)

func mobileListener(t *testing.T) *listener {
	t.Helper()
	sc := session.NewContext(session.WithLogging(false))
	sc.SealLogin(session.Seal{UID: "9007199254740993"})
	return &listener{sc: sc, ch: initializeChannels()}
}

func routeMobile(t *testing.T, ln *listener, act string, data map[string]any) {
	t.Helper()
	b, err := json.Marshal(map[string]any{"data": map[string]any{"controls": []any{map[string]any{"content": map[string]any{"act_type": "syncmsgmb", "act": act, "data": data}}}}})
	if err != nil {
		t.Fatal(err)
	}
	ln.router(context.Background(), 1, 601, 0, BaseWSMessage{Data: string(b)})
}

func TestMobileSyncExistingRouterCorrelation(t *testing.T) {
	// Arrange: optional receiver on the existing authenticated listener.
	ln := mobileListener(t)
	sub, stop, err := ln.SubscribeMobileSync("synthetic-public", "Web")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	// Act: foreign key and host cannot reach the receiver.
	routeMobile(t, ln, "user_confirm", map[string]any{"public_key": "foreign", "pc_name": "Web", "user_action": 1})
	routeMobile(t, ln, "user_confirm", map[string]any{"public_key": "synthetic-public", "pc_name": "Other", "user_action": 1})
	// Assert.
	select {
	case <-sub.Events:
		t.Fatal("foreign control delivered")
	default:
	}
	// Act: plain mobile owner differs from the current session noise ID.
	routeMobile(t, ln, "syncmsg_info", map[string]any{"public_key": "synthetic-public", "uid": "123", "from_seq_id": 0, "file_size": 16, "url": "https://synthetic.invalid/backup", "encrypted_key": "synthetic-key", "db_info": map[string]any{}})
	// Assert: the operation receives this correlated offer for authenticated mapping.
	select {
	case e := <-sub.Events:
		if e.UID != "123" || e.Action != "syncmsg_info" {
			t.Fatal("plain identity lost")
		}
	default:
		t.Fatal("plain identity rejected as a noise ID")
	}
	// Act: matching confirmation retains restoring, not synthetic success.
	routeMobile(t, ln, "user_confirm", map[string]any{"public_key": "synthetic-public", "pc_name": "Web", "user_action": 2})
	// Assert.
	select {
	case e := <-sub.Events:
		if e.UserAction == nil || *e.UserAction != 2 {
			t.Fatal("restoring lost")
		}
	default:
		t.Fatal("matching control missing")
	}
	select {
	case <-ln.Error():
		t.Fatal("valid controls caused collector error")
	default:
	}
}

func TestMobileSyncOverflowDoesNotBlockOrOverwrite(t *testing.T) {
	// Arrange.
	ln := mobileListener(t)
	sub, stop, err := ln.SubscribeMobileSync("synthetic-public", "Web")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	// Act: more controls than the receiver capacity, with no consumer.
	for n := 0; n < 20; n++ {
		routeMobile(t, ln, "user_confirm", map[string]any{"public_key": "synthetic-public", "pc_name": "Web", "user_action": n % 4})
	}
	// Assert: queued controls remain in order, overflow is explicit and emitted once.
	select {
	case err := <-sub.Errors:
		if err != ErrMobileSyncOverflow {
			t.Fatal("wrong failure")
		}
	default:
		t.Fatal("overflow hidden")
	}
	for n := 0; n < 8; n++ {
		select {
		case e := <-sub.Events:
			if e.UserAction == nil || *e.UserAction != n%4 {
				t.Fatal("queued control overwritten")
			}
		default:
			t.Fatal("queued control missing")
		}
	}
	select {
	case <-sub.Events:
		t.Fatal("failed receiver accepted further controls")
	default:
	}
	select {
	case <-sub.Errors:
		t.Fatal("overflow repeated")
	default:
	}
}

func TestMobileSyncUnsubscribeRaceAndSingleReceiver(t *testing.T) {
	// Arrange.
	ln := mobileListener(t)
	_, stop, err := ln.SubscribeMobileSync("synthetic-public", "Web")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ln.SubscribeMobileSync("other", "Web"); err != ErrMobileSyncReceiver {
		t.Fatal("parallel receiver allowed")
	}
	var wg sync.WaitGroup
	// Act: unsubscribing races with control delivery but never closes a sender's channel.
	wg.Add(2)
	go func() {
		defer wg.Done()
		for n := 0; n < 20; n++ {
			routeMobile(t, ln, "user_confirm", map[string]any{"public_key": "synthetic-public", "pc_name": "Web", "user_action": 1})
		}
	}()
	go func() { defer wg.Done(); stop(); stop() }()
	wg.Wait()
	// Assert: a fresh operation can register after release.
	_, stop2, err := ln.SubscribeMobileSync("second", "Web")
	if err != nil {
		t.Fatal(err)
	}
	stop2()
	routeMobile(t, ln, "user_confirm", map[string]any{"public_key": "second", "pc_name": "Web", "user_action": 1})
}

func TestMalformedMobileControlDoesNotFailCollector(t *testing.T) {
	// Arrange: inactive receiver first, then an active transfer wait.
	ln := mobileListener(t)
	routeMobile(t, ln, "user_confirm", map[string]any{})
	select {
	case <-ln.Error():
		t.Fatal("unrequested malformed sync failed collector")
	default:
	}
	sub, stop, err := ln.SubscribeMobileSync("synthetic-public", "Web")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	// Act.
	routeMobile(t, ln, "user_confirm", map[string]any{})
	// Assert: only the operation fails, no secret or general collector error.
	select {
	case err := <-sub.Errors:
		if err.Error() != "invalid mobile sync control" {
			t.Fatal("unclosed operation failure")
		}
	default:
		t.Fatal("malformed control ignored by active receiver")
	}
	select {
	case <-ln.Error():
		t.Fatal("operation failure leaked to collector")
	default:
	}
}

package listener

import (
	"context"
	"errors"
	"log/slog"

	"github.com/amrakk/zcago/listener/events"
	"github.com/amrakk/zcago/model"
)

var ErrMobileSyncReceiver = errors.New("mobile sync receiver unavailable")
var ErrMobileSyncOverflow = errors.New("mobile sync receiver overflow")

type MobileSyncSubscription struct {
	Events <-chan model.MobileSyncEvent
	Errors <-chan error
}

type mobileSyncReceiver struct {
	key, host, owner string
	events           chan model.MobileSyncEvent
	errors           chan error
	failed           bool
}

// SubscribeMobileSync is an optional concrete extension, not part of Listener.
// Register before requesting transfer; the caller owns the wait context.
func (ln *listener) SubscribeMobileSync(publicKey, pcName string) (MobileSyncSubscription, func(), error) {
	ln.mobileMu.Lock()
	defer ln.mobileMu.Unlock()
	if ln.mobileReceiver != nil || publicKey == "" || len(publicKey) > 4096 || pcName == "" || len(pcName) > 128 || ln.sc == nil || ln.sc.UID() == "" {
		return MobileSyncSubscription{}, nil, ErrMobileSyncReceiver
	}
	r := &mobileSyncReceiver{key: publicKey, host: pcName, owner: ln.sc.UID(), events: make(chan model.MobileSyncEvent, 8), errors: make(chan error, 1)}
	ln.mobileReceiver = r
	unsubscribe := func() {
		ln.mobileMu.Lock()
		defer ln.mobileMu.Unlock()
		if ln.mobileReceiver != r {
			return
		}
		ln.mobileReceiver = nil
		for {
			select {
			case <-r.events:
			default:
				return
			}
		}
	}
	return MobileSyncSubscription{Events: r.events, Errors: r.errors}, unsubscribe, nil
}

func (ln *listener) handleMobileSync(ctx context.Context, content events.ControlContent) {
	if ctx.Err() != nil {
		return
	}
	ln.mobileMu.Lock()
	defer ln.mobileMu.Unlock()
	r := ln.mobileReceiver
	if r == nil || r.failed || ln.sc.UID() != r.owner {
		return
	}
	if content.Data.MobileSyncInvalid {
		r.failed = true
		r.errors <- events.ErrMobileSyncControl
		return
	}
	if content.Data.MobileSync == nil {
		return
	}
	e := *content.Data.MobileSync
	if e.PublicKey != r.key {
		slog.Info("mobile_backup_control_ignored", "reason", "CORRELATION_KEY")
		return
	}
	// syncmsg_info.uid is a plain mobile ID. The operation must map it to
	// the session owner; direct equality here compares different namespaces.
	if e.Action != "syncmsg_info" && e.PCName != r.host {
		slog.Info("mobile_backup_control_ignored", "reason", "HOST_MISMATCH")
		return
	}
	select {
	case r.events <- e:
	default:
		r.failed = true
		r.errors <- ErrMobileSyncOverflow
	}
}

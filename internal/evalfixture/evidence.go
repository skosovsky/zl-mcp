package evalfixture

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/skosovsky/zl-mcp/internal/historyimport"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func (f *Fixture) EventCount(ctx context.Context) (int, error) {
	var n int
	err := f.Store.DB.QueryRowContext(ctx, "SELECT count(*) FROM message_events").Scan(&n)
	return n, err
}

// PrepareSyntheticEvents uses real activation/fanout/delivery journal owners;
// there is no callback network transport in this deterministic read/ACK fixture.
func (f *Fixture) PrepareSyntheticEvents(ctx context.Context) error {
	at := time.Now().UTC()
	//nolint:exhaustruct_v5 // EventSubscription preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
	sub := storage.EventSubscription{
		ID:        "synthetic-sub",
		Principal: "local:" + strconv.Itoa(os.Getuid()),
		Profile:   domain.ConversationMessageCreatedV2,
		Scope:     "all",
		Callback:  "https://receiver.example",
		Secret:    "synthetic",
	}
	revision, err := f.Store.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		return err
	}
	if _, err = f.Store.ActivateSubscription(ctx, sub, revision, at); err != nil {
		return err
	}
	for _, id := range []string{"event-one", "event-two"} {
		if err = f.Store.Put(
			ctx,
			//nolint:exhaustruct_v5 // Message preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
			domain.Message{
				Conversation: domain.ConversationRef{Type: domain.ConversationDirect, ID: fixturePeerID},
				ID:           id,
				SenderID:     fixturePeerID,
				Direction:    "incoming",
				Text:         id,
				SentAt:       at,
				Source:       wireLive,
			},
		); err != nil {
			return err
		}
	}
	encoder, err := events.NewMessageEncoder()
	if err != nil {
		return err
	}
	if _, err = f.Store.FanoutProfileEvents(
		ctx,
		at,
		storage.DefaultDeliveryPolicy(),
		encoder.EncodeProfile,
	); err != nil {
		return err
	}
	for {
		d, err := f.Store.ClaimDelivery(ctx, at, storage.DefaultDeliveryPolicy())
		if err != nil {
			return err
		}
		if d == nil {
			break
		}
		if err = f.Store.FinishDelivery(
			ctx,
			*d,
			//nolint:exhaustruct_v5 // DeliveryOutcome preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
			storage.DeliveryOutcome{State: "delivered", Reason: "accepted"},
			at,
		); err != nil {
			return err
		}
	}
	return nil
}

type blockedHistory struct {
	started chan struct{}
	release chan struct{}
	effects string
}

func (s *blockedHistory) HistoryPage(
	ctx context.Context,
	ref domain.ConversationRef,
	_ string,
	_ int,
) (domain.HistoryPage, error) {
	if err := recordEffect(s.effects, "history"); err != nil {
		return domain.HistoryPage{}, err
	}
	close(s.started)
	select {
	case <-s.release:
	case <-ctx.Done():
		return domain.HistoryPage{}, ctx.Err()
	}
	more := false
	//nolint:exhaustruct_v5 // HistoryPage preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
	return domain.HistoryPage{
		Messages: []domain.Message{
			//nolint:exhaustruct_v5 // Message preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
			{
				Conversation: ref,
				ID:           "late-history",
				SenderID:     fixturePeerID,
				Text:         "late history",
				SentAt:       time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
			},
		},
		HasMore: &more,
	}, nil
}

// StartBlockedHistory starts the real worker with a deterministic in-flight page.
func (f *Fixture) StartBlockedHistory(ctx context.Context) (<-chan struct{}, func(), error) {
	if f.offline == nil {
		return nil, nil, errors.New("extended fixture required")
	}
	source := &blockedHistory{started: make(chan struct{}), release: make(chan struct{}), effects: f.offline.effects}
	run, cancel := context.WithCancel(ctx)
	f.offline.historyCancel = cancel
	f.offline.historyDone = make(chan error, 1)
	go func() { f.offline.historyDone <- historyimport.RunRecovered(run, f.Store, source) }()
	var once sync.Once
	return source.started, func() { once.Do(func() { close(source.release) }) }, nil
}

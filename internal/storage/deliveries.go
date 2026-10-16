package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type DeliveryPolicy struct {
	MaxJobs     int
	MaxBytes    int64
	MaxAttempts int
	MaxAge      time.Duration
	Lease       time.Duration
}

func DefaultDeliveryPolicy() DeliveryPolicy {
	return DeliveryPolicy{
		MaxJobs:     maximumDeliveryJobs,
		MaxBytes:    maximumDeliveryBytes,
		MaxAttempts: maximumDeliveryAttempts,
		MaxAge:      7 * 24 * time.Hour,
		Lease:       deliveryLease,
	}
}

func (p DeliveryPolicy) validate() error {
	if p.MaxJobs < 1 || p.MaxBytes < 1 || p.MaxAttempts < 1 || p.MaxAge <= 0 || p.Lease <= 0 {
		return errors.New("invalid delivery policy")
	}
	return nil
}

type pendingMessageEvent struct {
	seq                            int64
	id, group, kind, body, created string
	direction                      string
	firstIncoming                  *bool
}

// FanoutEvents commits tasks and its watermark together. Encoding is a pure
// application port; no network calls or event transport types enter storage.
func (s *Store) FanoutEvents(
	ctx context.Context,
	at time.Time,
	p DeliveryPolicy,
	encode func(string, domain.Message) ([]byte, error),
) (int, error) {
	return s.FanoutProfileEvents(ctx, at, p, func(profile, id string, m domain.Message) ([]byte, error) {
		if profile != domain.LegacyMessageCreated {
			return nil, errors.New("profile encoder required")
		}
		return encode(id, m)
	})
}

//nolint:cyclop,funlen,gocognit,gocyclo // This ledger operation keeps validation, state changes and effect counters within one transaction/commit boundary.
func (s *Store) FanoutProfileEvents(
	ctx context.Context,
	at time.Time,
	p DeliveryPolicy,
	encode func(string, string, domain.Message) ([]byte, error),
) (int, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	//nolint:errcheck // Rollback is fallback cleanup; preserve the primary operation or Commit error.
	defer tx.Rollback()
	rows, err := tx.QueryContext(
		ctx,
		"SELECT seq,event_id,group_id,conversation_type,message_json,created_at,direction,first_incoming FROM message_events WHERE seq>(SELECT seq FROM event_fanout WHERE id=1) ORDER BY seq LIMIT 32",
	)
	if err != nil {
		return 0, err
	}
	var batch []pendingMessageEvent
	for rows.Next() {
		var e pendingMessageEvent
		if err := rows.Scan(
			&e.seq,
			&e.id,
			&e.group,
			&e.kind,
			&e.body,
			&e.created,
			&e.direction,
			&e.firstIncoming,
		); err != nil {
			//nolint:gosec,sqlclosecheck // G104: Close the read-only query immediately on error; later paths close before reuse of the connection.
			rows.Close()
			return 0, err
		}
		batch = append(batch, e)
	}
	if err := rows.Err(); err != nil {
		//nolint:gosec // G104: Read-only query cleanup; Scan/Rows.Err determines the result.
		rows.Close()
		return 0, err
	}
	//nolint:gosec // G104: Read-only query cleanup; Scan/Rows.Err determines the result.
	rows.Close()
	if len(batch) == 0 {
		return 0, tx.Commit()
	}
	var jobs int
	var payloadBytes int64
	if err := tx.QueryRowContext(ctx, "SELECT count(*),COALESCE(sum(length(payload)),0) FROM event_deliveries WHERE state IN ('pending','sending')").
		Scan(&jobs, &payloadBytes); err != nil {
		return 0, err
	}
	for _, event := range batch {
		created, err := time.Parse(time.RFC3339Nano, event.created)
		if err != nil {
			return 0, err
		}
		message, encodeErr := decodeContentMessage([]byte(event.body))
		message.Conversation = domain.ConversationRef{Type: event.kind, ID: event.group}
		message.Direction = event.direction
		message.FirstIncoming = event.firstIncoming
		if encodeErr == nil {
			if scanErr := tx.QueryRowContext(ctx, "SELECT name FROM conversations WHERE conversation_type=? AND conversation_id=?", event.kind, event.group).
				Scan(&message.ConversationName); scanErr != nil &&
				!errors.Is(scanErr, sql.ErrNoRows) {
				return 0, scanErr
			}
		}
		rows, err := tx.QueryContext(
			ctx,
			"SELECT id,generation,profile FROM event_subscriptions WHERE active=1 AND profile IN ('zalo.message.created','zalo.conversation.message.created.v2','zalo.conversation.message.created.v3') AND (scope='all' OR scope=? OR (scope='conversation' AND group_id=? AND conversation_type=?)) AND (direction='all' OR direction=?) AND (first_incoming_only=0 OR ?=1) AND start_seq<? AND (expires_at IS NULL OR julianday(expires_at)>julianday(?))",
			event.kind,
			event.group,
			event.kind,
			event.direction,
			event.firstIncoming,
			event.seq,
			at.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			return 0, err
		}
		type target struct{ id, generation, profile string }
		var targets []target
		for rows.Next() {
			var operationValue target
			if err := rows.Scan(&operationValue.id, &operationValue.generation, &operationValue.profile); err != nil {
				//nolint:gosec,sqlclosecheck // G104: Close the read-only query immediately on error; later paths close before reuse of the connection.
				rows.Close()
				return 0, err
			}
			targets = append(targets, operationValue)
		}
		if err := rows.Err(); err != nil {
			//nolint:gosec // G104: Read-only query cleanup; Scan/Rows.Err determines the result.
			rows.Close()
			return 0, err
		}
		//nolint:gosec // G104: Read-only query cleanup; Scan/Rows.Err determines the result.
		rows.Close()
		for _, target := range targets {
			var payload []byte
			payloadErr := encodeErr
			if payloadErr == nil {
				payload, payloadErr = encode(target.profile, event.id, message)
			}
			state := statePendingText
			var reason, completed any
			body := payload
			switch {
			case !s.AllowsConversation(message.Ref()):
				state, reason = stateCancelledText, "access_revoked"
			case payloadErr != nil:
				state, reason = stateFailedText, reasonInvalidPayload
			case !at.Before(created.Add(p.MaxAge)):
				state, reason = stateFailedText, "deadline"
			case jobs >= p.MaxJobs || payloadBytes+int64(len(payload)) > p.MaxBytes:
				state, reason = stateFailedText, "queue_capacity"
			}
			if state != statePendingText {
				body = []byte{}
				completed = at.UTC().Format(time.RFC3339Nano)
			}
			result, err := tx.ExecContext(
				ctx,
				`INSERT OR IGNORE INTO event_deliveries(event_id,subscription_id,generation,payload,state,next_attempt_at,deadline,last_reason,completed_at,message_seq) VALUES(?,?,?,?,?,?,?,?,?,?)`,
				event.id,
				target.id,
				target.generation,
				body,
				state,
				at.UTC().Format(time.RFC3339Nano),
				created.Add(p.MaxAge).UTC().Format(time.RFC3339Nano),
				reason,
				completed,
				event.seq,
			)
			if err != nil {
				return 0, err
			}
			inserted, err := result.RowsAffected()
			if err != nil {
				return 0, err
			}
			if state == statePendingText && inserted > 0 {
				jobs++
				payloadBytes += int64(len(body))
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE event_fanout SET seq=? WHERE id=1", event.seq); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM message_events WHERE seq=?", event.seq); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(batch), nil
}

type EventDelivery struct {
	CreatedAt      *time.Time // Original event creation; nil if unavailable.
	ID             int64
	EventID        string
	SubscriptionID string
	Generation     string
	Payload        []byte
	Callback       string
	Secret         string
	Principal      string
	GroupID        string
	Attempts       int
	Deadline       time.Time
	LeaseUntil     time.Time
}

// ClaimDelivery uses a durable lease; an unacknowledged request is retried after
// lease expiry. One active lease per subscription serializes its callbacks.
//
//nolint:gocognit // This ledger operation keeps validation, state changes and effect counters within one transaction/commit boundary.
func (s *Store) ClaimDelivery(ctx context.Context, at time.Time, p DeliveryPolicy) (*EventDelivery, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // Rollback is fallback cleanup; preserve the primary operation or Commit error.
	defer tx.Rollback()
	ts := at.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		"UPDATE event_deliveries SET state='pending',lease_until=NULL WHERE state='sending' AND julianday(lease_until)<=julianday(?)",
		ts,
	); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE event_deliveries SET state='cancelled',last_reason='subscription_inactive',completed_at=?,payload=X'',lease_until=NULL WHERE state IN ('pending','sending') AND NOT EXISTS(SELECT 1 FROM event_subscriptions s WHERE s.id=subscription_id AND s.generation=event_deliveries.generation AND s.active=1 AND (s.expires_at IS NULL OR julianday(s.expires_at)>julianday(?)))`,
		ts,
		ts,
	); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(
		ctx,
		"UPDATE event_deliveries SET state='failed',last_reason=CASE WHEN attempts>=? THEN 'attempts' ELSE 'deadline' END,completed_at=?,lease_until=NULL WHERE state='pending' AND (attempts>=? OR julianday(deadline)<=julianday(?))",
		p.MaxAttempts,
		ts,
		p.MaxAttempts,
		ts,
	); err != nil {
		return nil, err
	}
	for {
		var d EventDelivery
		var deadline, profile, scope, kind string
		var created *string
		var ref domain.ConversationRef
		err := tx.QueryRowContext(ctx, `SELECT d.id,d.event_id,d.subscription_id,d.generation,d.payload,d.attempts,d.deadline,s.callback,s.secret,s.principal,s.group_id,s.profile,s.scope,s.conversation_type,CASE WHEN d.activity_seq IS NOT NULL THEN COALESCE(a.conversation_type,'') ELSE COALESCE(m.conversation_type,s.conversation_type) END,CASE WHEN d.activity_seq IS NOT NULL THEN COALESCE(a.conversation_id,'') ELSE COALESCE(m.conversation_id,s.group_id) END,COALESCE(a.created_at,ev.created_at) FROM event_deliveries d LEFT JOIN message_events ev ON ev.event_id=d.event_id JOIN event_subscriptions s ON s.id=d.subscription_id LEFT JOIN messages m ON m.seq=d.message_seq LEFT JOIN activity_observations a ON a.seq=d.activity_seq AND a.expired=0 WHERE d.state='pending' AND julianday(d.next_attempt_at)<=julianday(?) AND NOT EXISTS(SELECT 1 FROM event_deliveries busy WHERE busy.subscription_id=d.subscription_id AND busy.state='sending') AND NOT EXISTS(SELECT 1 FROM event_deliveries earlier WHERE earlier.subscription_id=d.subscription_id AND earlier.id<d.id AND earlier.state IN ('pending','sending')) ORDER BY d.id LIMIT 1`, ts).
			Scan(&d.ID, &d.EventID, &d.SubscriptionID, &d.Generation, &d.Payload, &d.Attempts, &deadline, &d.Callback, &d.Secret, &d.Principal, &d.GroupID, &profile, &scope, &kind, &ref.Type, &ref.ID, &created)
		if errors.Is(err, sql.ErrNoRows) {
			if commitErr := tx.Commit(); commitErr != nil {
				return nil, commitErr
			}
			//nolint:nilnil // An empty delivery queue returns no job and no failure.
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !s.AllowsConversation(ref) || !subscriptionMatches(profile, scope, kind, d.GroupID, ref) {
			if _, execContextErr := tx.ExecContext(
				ctx,
				"UPDATE event_deliveries SET state='cancelled',last_reason='access_revoked',completed_at=?,payload=X'' WHERE id=?",
				ts,
				d.ID,
			); execContextErr != nil {
				return nil, execContextErr
			}
			continue
		}
		if created != nil {
			if stamp, e := time.Parse(time.RFC3339Nano, *created); e == nil {
				d.CreatedAt = &stamp
			}
		}
		d.Deadline, err = time.Parse(time.RFC3339Nano, deadline)
		if err != nil {
			return nil, err
		}
		d.LeaseUntil = at.Add(p.Lease)
		d.Attempts++
		if _, err := tx.ExecContext(
			ctx,
			"UPDATE event_deliveries SET state='sending',attempts=?,lease_until=? WHERE id=?",
			d.Attempts,
			d.LeaseUntil.UTC().Format(time.RFC3339Nano),
			d.ID,
		); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &d, nil
	}
}

type DeliveryOutcome struct {
	State   string // delivered, pending (retry), failed
	Reason  string // safe category, never a raw HTTP error
	RetryAt time.Time
	Gone    bool
}

func (s *Store) FinishDelivery(ctx context.Context, d EventDelivery, outcome DeliveryOutcome, at time.Time) error {
	if outcome.State != stateDeliveredText && outcome.State != statePendingText && outcome.State != stateFailedText {
		return errors.New("invalid delivery outcome")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	//nolint:errcheck // Rollback is fallback cleanup; preserve the primary operation or Commit error.
	defer tx.Rollback()
	var completed any
	if outcome.State != statePendingText {
		completed = at.UTC().Format(time.RFC3339Nano)
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE event_deliveries SET state=?,last_reason=?,next_attempt_at=?,completed_at=?,lease_until=NULL WHERE id=? AND state='sending' AND attempts=? AND lease_until=?`,
		outcome.State,
		outcome.Reason,
		outcome.RetryAt.UTC().Format(time.RFC3339Nano),
		completed,
		d.ID,
		d.Attempts,
		d.LeaseUntil.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if outcome.Gone && updated > 0 {
		if _, err := tx.ExecContext(
			ctx,
			"UPDATE event_subscriptions SET active=0 WHERE id=? AND generation=?",
			d.SubscriptionID,
			d.Generation,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			"UPDATE event_subscription_revisions SET revision=revision+1 WHERE id=?",
			d.SubscriptionID,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			"UPDATE event_deliveries SET state='cancelled',last_reason='callback_gone',completed_at=?,lease_until=NULL,payload=X'' WHERE subscription_id=? AND generation=? AND state IN ('pending','sending')",
			at.UTC().Format(time.RFC3339Nano),
			d.SubscriptionID,
			d.Generation,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PruneDeliveries(ctx context.Context, at time.Time) error {
	if err := s.PruneActivities(ctx, at); err != nil {
		return err
	}
	return s.pruneEventJournal(ctx, at)
}

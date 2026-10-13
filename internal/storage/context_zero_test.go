package storage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type zeroContextRead func(context.Context, string, int, int) (map[string]any, error)

//nolint:gocognit,gocyclo,cyclop // This complete AAA fixture checks the same zero/mixed window and reply contract in all three public namespaces with one storage lifetime.
func TestZeroContextWindowsPreserveWireAndReply(t *testing.T) {
	for _, kind := range []string{"legacy", domain.ConversationGroup, domain.ConversationDirect} {
		t.Run(kind, func(t *testing.T) {
			// Arrange: fixed Unicode messages and opaque IDs, with a real reply.
			ctx := t.Context()
			// The synthetic all-policy fixture intentionally leaves the optional selected map unset.
			s, err := OpenWithPolicy(
				ctx,
				filepath.Join(t.TempDir(), "corpus.sqlite"),
				domain.CollectionPolicy{All: true},
				90,
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if closeErr := s.Close(); closeErr != nil {
					t.Error(closeErr)
				}
			})
			ref := domain.ConversationRef{Type: kind, ID: "9007199254740993"}
			if kind == "legacy" {
				ref.Type = domain.ConversationGroup
			}
			read := zeroContextRead(func(ctx context.Context, id string, before, after int) (map[string]any, error) {
				return s.ConversationContext(ctx, ref, id, before, after)
			})
			if kind == "legacy" {
				read = func(ctx context.Context, id string, before, after int) (map[string]any, error) {
					return s.Context(ctx, ref.ID, id, before, after)
				}
			}
			ids := []string{"9007199254740993", "9007199254740994", "9007199254740995"}
			for n, id := range ids {
				// Synthetic message preserves absent optional metadata/attachments; only the anchor has an explicit reply.
				message := domain.Message{
					Conversation: ref,
					ID:           id,
					SenderID:     "peer",
					SentAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Second),
					Text:         "ремонт Cà phê",
					Source:       "live",
				}
				if n == 1 {
					replyID := ids[0]
					message.ReplyTo = &replyID
				}
				if err = s.Put(ctx, message); err != nil {
					t.Fatal(err)
				}
			}
			for _, window := range [][2]int{{0, 0}, {0, 1}, {1, 0}, {1, 1}} {
				// Act.
				result, readErr := read(ctx, ids[1], window[0], window[1])
				if readErr != nil {
					t.Fatal(readErr)
				}
				raw, marshalErr := json.Marshal(result)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				fields := map[string]json.RawMessage{}
				if err = json.Unmarshal(raw, &fields); err != nil {
					t.Fatal(err)
				}
				// Assert: anchor identity/Unicode, empty arrays and neighbor order are retained.
				anchor := map[string]any{}
				if err = json.Unmarshal(fields["anchor"], &anchor); err != nil {
					t.Fatal(err)
				}
				if anchor["message_id"] != ids[1] || anchor["text"] != "ремонт Cà phê" {
					t.Fatal(string(raw))
				}
				for index, key := range []string{"before", "after"} {
					messages := []map[string]any{}
					if err = json.Unmarshal(fields[key], &messages); err != nil {
						t.Fatal(err)
					}
					if len(messages) != window[index] {
						t.Fatal(string(raw))
					}
					if window[index] == 0 && string(fields[key]) != "[]" {
						t.Fatal(string(raw))
					}
					if window[index] != 0 && messages[0]["message_id"] != ids[index*2] {
						t.Fatal(string(raw))
					}
				}
				reply := map[string]any{}
				if err = json.Unmarshal(fields["reply_to"], &reply); err != nil {
					t.Fatal(err)
				}
				if reply["message_id"] != ids[0] {
					t.Fatal(string(raw))
				}
			}
			// Act/Assert: after deleting the reply, zero-window output represents absence as null.
			if ref.Type == domain.ConversationDirect {
				err = s.DeleteConversation(ctx, ref, ids[0])
			} else {
				err = s.Delete(ctx, ref.ID, ids[0])
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := read(ctx, ids[1], 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			fields := map[string]json.RawMessage{}
			if err = json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if string(fields["reply_to"]) != "null" {
				t.Fatal(string(raw))
			}
		})
	}
}

func TestZeroContextNeighborCancellation(t *testing.T) {
	// Arrange: cancellation happens immediately before the optional neighbor phase.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// No database is needed for an empty window; this fixture tests cancellation before any query.
	s := &Store{}
	// Act.
	// Anchor metadata is irrelevant to a zero window; the cancellation must be propagated without reading it.
	messages, err := s.contextNeighbors(
		ctx,
		domain.ConversationRef{Type: domain.ConversationGroup, ID: "g"},
		domain.Message{},
		"m",
		"<",
		"DESC",
		0,
		false,
	)
	// Assert.
	if !errors.Is(err, context.Canceled) || messages == nil || len(messages) != 0 {
		t.Fatalf("messages=%v err=%v", messages, err)
	}
}

func TestZeroContextWindowsFailClosed(t *testing.T) {
	// Arrange: known allowed anchor and an otherwise empty synthetic database.
	s := openTest(t)
	ctx := t.Context()
	putTest(t, s, "9007199254740993", "g1", "Cà phê ремонт", time.Now())
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	cases := []struct {
		name      string
		ctx       context.Context
		group, id string
		before    int
	}{
		{name: "denied policy", ctx: ctx, group: "forbidden", id: "9007199254740993", before: 0},
		{name: "missing anchor", ctx: ctx, group: "g1", id: "missing", before: 0},
		{name: "invalid window", ctx: ctx, group: "g1", id: "9007199254740993", before: -1},
		{name: "canceled", ctx: canceled, group: "g1", id: "9007199254740993", before: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			result, err := s.Context(tc.ctx, tc.group, tc.id, tc.before, 0)
			// Assert: the empty neighbor shortcut cannot bypass required guards.
			if err == nil || result != nil {
				t.Fatalf("result=%v err=%v", result, err)
			}
		})
	}
	// Act: the owner closes before the read.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := s.Context(ctx, "g1", "9007199254740993", 0, 0)
	// Assert.
	if err == nil || result != nil {
		t.Fatalf("closedDB result=%v err=%v", result, err)
	}
}

package zalo

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/amrakk/zcago/model"
)

type replayCapture struct {
	calls  int
	first  bool
	cursor string
	queue  model.ThreadType
	err    error
}

func (r *replayCapture) RequestReplayPage(_ context.Context, queue model.ThreadType, first bool, cursor *string) error {
	r.calls++
	r.first = first
	r.cursor = *cursor
	r.queue = queue
	return r.err
}

func TestReplayPagerContinuesExactQueueAndStopsRepeatedCursor(t *testing.T) {
	// Arrange
	ctx := context.Background()
	p := newReplayPager(true)
	r := &replayCapture{}
	more := true
	id := "90071992547409931234"
	meta := &model.ReplayContinuation{Queue: model.ThreadTypeUser, More: &more, Valid: true, LastActionID: id, MessageCount: 2}
	// Act
	if err := p.Continue(ctx, r, meta); err != nil {
		t.Fatal(err)
	}
	if err := p.Continue(ctx, r, meta); err != nil {
		t.Fatal(err)
	}
	// Assert
	if r.calls != 1 || r.first || r.cursor != id || r.queue != model.ThreadTypeUser || !p[model.ThreadTypeUser].stopped || p[model.ThreadTypeGroup].stopped {
		t.Fatal("wrong queue continuation or loop")
	}
}

func TestReplayPagerBoundsAndDiagnosticsWithoutIDs(t *testing.T) {
	for _, reason := range []string{"invalid", "missing", "unknown", "exhausted", "pages", "messages", "unsupported", "group_only"} {
		t.Run(reason, func(t *testing.T) {
			// Arrange
			p := newReplayPager(reason != "group_only")
			r := &replayCapture{}
			more := true
			meta := &model.ReplayContinuation{Queue: model.ThreadTypeUser, More: &more, Valid: true, LastActionID: "123456789", MessageCount: 1}
			var source any = r
			switch reason {
			case "invalid":
				meta.Valid = false
			case "missing":
				meta.LastActionID = ""
			case "unknown":
				meta.More = nil
			case "exhausted":
				more = false
			case "pages":
				p[model.ThreadTypeUser].pages = 99
			case "messages":
				p[model.ThreadTypeUser].messages = 9999
			case "unsupported":
				source = struct{}{}
			}
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			defer slog.SetDefault(previous)
			// Act
			err := p.Continue(context.Background(), source, meta)
			// Assert
			if err != nil || r.calls != 0 || (meta.LastActionID != "" && strings.Contains(logs.String(), meta.LastActionID)) {
				t.Fatal("unsafe, unbounded or unintended continuation", err)
			}
		})
	}
}

func TestReplayPagerPropagatesRequestFailure(t *testing.T) {
	// Arrange
	p := newReplayPager(true)
	cause := errors.New("synthetic request failure")
	r := &replayCapture{err: cause}
	more := true
	// Act
	err := p.Continue(context.Background(), r, &model.ReplayContinuation{Queue: model.ThreadTypeUser, More: &more, LastActionID: "12", Valid: true})
	// Assert
	if !errors.Is(err, cause) {
		t.Fatal("request failure hidden")
	}
}

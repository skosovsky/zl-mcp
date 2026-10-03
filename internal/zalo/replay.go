package zalo

import (
	"context"
	"log/slog"

	"github.com/amrakk/zcago/model"
)

type replayRequester interface {
	RequestReplayPage(context.Context, model.ThreadType, bool, *string) error
}
type replayQueue struct {
	pages, messages int
	cursors         map[string]bool
	stopped         bool
}
type replayPager map[model.ThreadType]*replayQueue

func newReplayPager(includeDirect bool) replayPager {
	p := replayPager{model.ThreadTypeGroup: {cursors: map[string]bool{}}}
	if includeDirect {
		p[model.ThreadTypeUser] = &replayQueue{cursors: map[string]bool{}}
	}
	return p
}

// Continue is called only after all homogeneous batches in a response persist.
func (p replayPager) Continue(ctx context.Context, source any, meta *model.ReplayContinuation) error {
	if meta == nil {
		return nil
	}
	q := p[meta.Queue]
	if q == nil || q.stopped {
		return nil
	}
	q.pages++
	q.messages += meta.MessageCount
	reason := ""
	switch {
	case !meta.Valid:
		reason = "invalid_metadata"
	case meta.More == nil:
		reason = "metadata_unavailable"
	case !*meta.More:
		reason = "queue_exhausted"
	case meta.LastActionID == "":
		reason = "missing_cursor"
	case q.cursors[meta.LastActionID]:
		reason = "repeated_cursor"
	case q.pages >= 100:
		reason = "page_limit"
	case q.messages >= 10000:
		reason = "message_limit"
	}
	requester, supported := source.(replayRequester)
	if reason == "" && !supported {
		reason = "continuation_unsupported"
	}
	if reason != "" {
		q.stopped = true
		slog.Info("zalo_replay_queue_stopped", "group_queue", meta.Queue == model.ThreadTypeGroup, "reason", reason, "pages", q.pages, "messages", q.messages)
		return nil
	}
	q.cursors[meta.LastActionID] = true
	cursor := meta.LastActionID
	return requester.RequestReplayPage(ctx, meta.Queue, false, &cursor)
}

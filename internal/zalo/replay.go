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
	//nolint:exhaustive,exhaustruct_v5 // User replay is opt-in; omitted queues make no direct request and counters start at zero.
	p := replayPager{model.ThreadTypeGroup: {cursors: map[string]bool{}}}
	if includeDirect {
		//nolint:exhaustruct_v5 // replayQueue initializes only this owner/phase; optional collaborators, buffers and counters keep their zero defaults.
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
	case q.pages >= maximumReplayPages:
		reason = "page_limit"
	case q.messages >= maximumReplayMessages:
		reason = "message_limit"
	}
	requester, supported := source.(replayRequester)
	if reason == "" && !supported {
		reason = "continuation_unsupported"
	}
	if reason != "" {
		q.stopped = true
		//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
		slog.InfoContext(
			ctx,
			"zalo_replay_queue_stopped",
			"group_queue",
			meta.Queue == model.ThreadTypeGroup,
			"reason",
			reason,
			"pages",
			q.pages,
			"messages",
			q.messages,
		)
		return nil
	}
	q.cursors[meta.LastActionID] = true
	cursor := meta.LastActionID
	return requester.RequestReplayPage(ctx, meta.Queue, false, &cursor)
}

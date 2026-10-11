package conversations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type catalogProbe struct {
	CatalogPort
	groupCalls int
	requests   []GroupRequest
	group      GroupDetail
	err        error
}

func (p *catalogProbe) Group(context.Context, string) (GroupDetail, error) {
	p.groupCalls++
	return p.group, p.err
}
func (p *catalogProbe) Groups(_ context.Context, r GroupRequest) (GroupList, error) {
	p.requests = append(p.requests, r)
	return GroupList{Groups: make([]domain.Group, r.Limit), HasMore: true}, nil
}

type liveGroupProbe struct {
	calls    int
	deadline time.Time
	err      error
	result   GroupDetail
}

func (p *liveGroupProbe) Group(ctx context.Context, _ string) (GroupDetail, error) {
	p.calls++
	p.deadline, _ = ctx.Deadline()
	return p.result, p.err
}

func TestGroupRefreshKeepsBoundedDeadlineAndNativeFallback(t *testing.T) {
	// Arrange.
	local := &catalogProbe{group: GroupDetail{Group: domain.Group{ID: "9007199254740993"}}}
	live := &liveGroupProbe{err: errors.New("synthetic refresh unavailable")}
	h := &Handler{Catalog: local, LiveGroups: live}
	started := time.Now()
	// Act.
	result, err := h.Group(context.Background(), "9007199254740993")
	// Assert: one bounded live read then one original-context local fallback.
	if err != nil || !result.Stale || result.Group.ID != "9007199254740993" || live.calls != 1 || local.groupCalls != 1 {
		t.Fatalf("fallback changed: %+v %v", result, err)
	}
	if live.deadline.Before(started.Add(5*time.Second)) || live.deadline.After(time.Now().Add(5*time.Second)) {
		t.Fatal("refresh timeout changed")
	}
	live.err = nil
	live.result = GroupDetail{Group: domain.Group{ID: "9007199254740993"}, Stale: false}
	result, err = h.Group(context.Background(), "9007199254740993")
	if err != nil || result.Stale || local.groupCalls != 1 || live.calls != 2 {
		t.Fatal("live success incorrectly fell back")
	}
	local.err = domain.Invalid("Synthetic local error.")
	live.err = errors.New("refresh failed")
	_, err = h.Group(context.Background(), "9007199254740993")
	if err != local.err || local.groupCalls != 2 {
		t.Fatalf("local cause changed: %v", err)
	}
}

func TestGroupContinuationShortensNativePageWithoutRedispatch(t *testing.T) {
	// Arrange.
	local := &catalogProbe{}
	h := &Handler{Catalog: local}
	request := GroupRequest{Query: "Cà phê 🍀", Limit: 3, Cursor: "opaque"}
	// Act.
	page, err := h.Groups(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err = page.Shorten(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = page.Shorten(context.Background()); err != nil {
		t.Fatal(err)
	}
	lastErr := page.Shorten(context.Background())
	// Assert: native queries retain filters/cursor; singleton remains a bounded error.
	if len(local.requests) != 3 || len(page.Result.Groups) != 1 || lastErr == nil {
		t.Fatal("unbounded group continuation")
	}
	for i, r := range local.requests {
		if r.Query != request.Query || r.Cursor != request.Cursor || r.Limit != 3-i {
			t.Fatalf("changed page selection: %+v", local.requests)
		}
	}
}

package mobilebackup

import (
	"math"
	"time"
)

// millisecondWindow maps exact [since,until) onto integer source timestamps.
// Both bounds round upward; a nonempty time interval may contain no integer ticks.
func millisecondWindow(since, until time.Time) (from, to int64, ok bool) {
	if since.IsZero() || !until.After(since) {
		return 0, 0, false
	}
	from, ok = ceilMilliseconds(since)
	if !ok {
		return 0, 0, false
	}
	to, ok = ceilMilliseconds(until)
	if !ok || from > to {
		return 0, 0, false
	}
	return from, to, true
}
func ceilMilliseconds(at time.Time) (int64, bool) {
	value := at.UnixMilli()
	if !time.UnixMilli(value).Equal(at.Truncate(time.Millisecond)) {
		return 0, false
	}
	if at.Nanosecond()%int(time.Millisecond) != 0 {
		if value == math.MaxInt64 {
			return 0, false
		}
		value++
	}
	return value, true
}

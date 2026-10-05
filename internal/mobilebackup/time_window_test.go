package mobilebackup

import (
	"math"
	"testing"
	"time"
)

func TestIntegerTimeWindowPreservesExactInterval(t *testing.T) {
	// Arrange / Act / Assert: independently specified integer sets, including before epoch.
	for _, tc := range []struct {
		lower, upper time.Time
		from, to     int64
	}{
		{time.Unix(0, 500000), time.Unix(0, 1500000), 1, 2},
		{time.Unix(0, 100000), time.Unix(0, 900000), 1, 1},
		{time.Unix(0, -1500000), time.Unix(0, -500000), -1, 0},
		{time.UnixMilli(1), time.UnixMilli(2), 1, 2},
	} {
		from, to, ok := millisecondWindow(tc.lower, tc.upper)
		if !ok || from != tc.from || to != tc.to {
			t.Fatal("integer interval changed")
		}
	}
	if _, _, ok := millisecondWindow(time.UnixMilli(math.MaxInt64).Add(time.Nanosecond), time.UnixMilli(math.MaxInt64).Add(time.Millisecond)); ok {
		t.Fatal("overflow accepted")
	}
	if _, ok := ceilMilliseconds(time.Unix(1<<62, 0)); ok {
		t.Fatal("unrepresentable epoch accepted")
	}
}

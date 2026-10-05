package mobilebackup

import (
	"errors"
	"math"
	"testing"
)

func TestMessageExpiryMS(t *testing.T) {
	cases := []struct {
		name                 string
		timestamp, ttl, want int64
		declared, invalid    bool
	}{
		{name: "no expiration", timestamp: 1700000000000},
		{name: "milliseconds preserved", timestamp: 1700000000000, ttl: 60000, want: 1700000060000, declared: true},
		{name: "large exact integer", timestamp: 9007199254740993, ttl: 1, want: 9007199254740994, declared: true},
		{name: "maximum exact sum", timestamp: math.MaxInt64 - 1, ttl: 1, want: math.MaxInt64, declared: true},
		{name: "overflow", timestamp: math.MaxInt64, ttl: 1, invalid: true},
		{name: "missing timestamp no fallback", ttl: 1, invalid: true},
		{name: "negative timestamp", timestamp: -1, ttl: 1, invalid: true},
		{name: "negative ttl", timestamp: 1, ttl: -1, invalid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: exact archive scalars above, independent of the current clock.
			// Act.
			got, declared, err := MessageExpiryMS(tc.timestamp, tc.ttl)
			// Assert.
			if tc.invalid {
				if !errors.Is(err, ErrExpiry) || got != 0 || declared {
					t.Fatalf("invalid expiry accepted: %d %v %v", got, declared, err)
				}
				return
			}
			if err != nil || got != tc.want || declared != tc.declared {
				t.Fatalf("expiry = %d %v %v; want %d %v", got, declared, err, tc.want, tc.declared)
			}
		})
	}
}

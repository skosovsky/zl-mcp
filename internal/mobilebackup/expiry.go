package mobilebackup

import (
	"errors"
	"math"
)

var ErrExpiry = errors.New("mobile backup expiry invalid")

// MessageExpiryMS computes the client-derived expiry without importing a row or
// applying a wall-clock fallback. A zero TTL means no message expiry is declared.
// Callers must use the archive's server timestamp, never the time of import.
func MessageExpiryMS(serverTimestampMS, ttlMS int64) (expiresMS int64, declared bool, err error) {
	if serverTimestampMS <= 0 || ttlMS < 0 {
		return 0, false, ErrExpiry
	}
	if ttlMS == 0 {
		return 0, false, nil
	}
	if ttlMS > math.MaxInt64-serverTimestampMS {
		return 0, false, ErrExpiry
	}
	return serverTimestampMS + ttlMS, true, nil
}

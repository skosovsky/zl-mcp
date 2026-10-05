package domain

// ExpiringHistoryRecord carries normalized source data, not an ingestion policy.
type ExpiringHistoryRecord struct {
	Message     Message `json:"-"`
	ExpiresAtMS int64   `json:"-"`
}

func (ExpiringHistoryRecord) String() string   { return "historical record [redacted]" }
func (ExpiringHistoryRecord) GoString() string { return "historical record [redacted]" }

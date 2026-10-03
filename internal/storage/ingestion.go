package storage

import "sync/atomic"

// Counters are per open store, not a permanent ledger. They contain no upstream
// identities or content and count persistence only after transaction commit.
type ingestionCounters struct {
	received, excluded, inserted, duplicates, errors atomic.Uint64
}

type IngestionCount struct {
	Received   uint64 `json:"received"`
	Excluded   uint64 `json:"excluded"`
	Inserted   uint64 `json:"inserted"`
	Duplicates uint64 `json:"duplicates"`
	Errors     uint64 `json:"errors"`
}

func (c *ingestionCounters) snapshot() IngestionCount {
	return IngestionCount{Received: c.received.Load(), Excluded: c.excluded.Load(), Inserted: c.inserted.Load(), Duplicates: c.duplicates.Load(), Errors: c.errors.Load()}
}

func (s *Store) IngestionDiagnostics() map[string]IngestionCount {
	return map[string]IngestionCount{"direct": s.directIngestion.snapshot(), "group": s.groupIngestion.snapshot()}
}

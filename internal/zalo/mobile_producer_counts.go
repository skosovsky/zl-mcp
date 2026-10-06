package zalo

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// Optional producer claims are diagnostic only; never use them as source proof.
type mobileProducerCounts struct {
	State        string  `json:"state"`
	Verification string  `json:"verification"`
	Total        *uint64 `json:"msg_total,omitempty"`
	Threads      *uint64 `json:"msg_thread,omitempty"`
}

func inspectMobileProducerCounts(info string) mobileProducerCounts {
	r := mobileProducerCounts{State: "invalid", Verification: "unverified"}
	var envelope struct {
		Backup json.RawMessage `json:"backup_db"`
	}
	data := bytes.TrimSpace([]byte(info))
	if len(info) > 16<<10 || len(data) == 0 || data[0] != '{' || json.Unmarshal(data, &envelope) != nil {
		return r
	}
	if len(envelope.Backup) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Backup), []byte("null")) {
		r.State = "absent"
		return r
	}
	var counts struct {
		Total   json.RawMessage `json:"msg_total"`
		Threads json.RawMessage `json:"msg_thread"`
	}
	if json.Unmarshal(envelope.Backup, &counts) != nil {
		return r
	}
	parse := func(raw json.RawMessage) (uint64, bool) {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 {
			return 0, false
		}
		for _, b := range raw {
			if b < '0' || b > '9' {
				return 0, false
			}
		}
		n, err := strconv.ParseUint(string(raw), 10, 64)
		return n, err == nil
	}
	total, okTotal := parse(counts.Total)
	threads, okThreads := parse(counts.Threads)
	if !okTotal || !okThreads {
		return r
	}
	r.State, r.Total, r.Threads = "available", &total, &threads
	return r
}

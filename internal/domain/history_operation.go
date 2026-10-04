package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type HistoryImportRequest struct {
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
	RequestID        string `json:"request_id"`
	Since            string `json:"since"`
	Until            string `json:"until"`
	PageSize         int    `json:"page_size"`
	MaxPages         int    `json:"max_pages"`
	MaxMessages      int    `json:"max_messages"`
}

func (r HistoryImportRequest) Ref() ConversationRef {
	return ConversationRef{Type: r.ConversationType, ID: r.ConversationID}
}

func (r HistoryImportRequest) Normalize() (HistoryImportRequest, error) {
	id, err := uuid.Parse(r.RequestID)
	if err != nil || len(r.RequestID) != 36 {
		return r, Invalid("request_id must be a UUID.")
	}
	r.RequestID = id.String()
	if !r.Ref().Valid() || !utf8.ValidString(r.ConversationID) || utf8.RuneCountInString(r.ConversationID) > 256 {
		return r, Invalid("Use an exact typed conversation identity.")
	}
	since, e1 := time.Parse(time.RFC3339Nano, r.Since)
	until, e2 := time.Parse(time.RFC3339Nano, r.Until)
	if e1 != nil || e2 != nil || !since.Before(until) {
		return r, Invalid("Use RFC3339 since/until with since before until.")
	}
	r.Since, r.Until = since.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano)
	if r.PageSize == 0 {
		r.PageSize = 50
	}
	if r.MaxPages == 0 {
		r.MaxPages = 20
	}
	if r.MaxMessages == 0 {
		r.MaxMessages = 1000
	}
	if r.PageSize < 1 || r.PageSize > 50 || r.MaxPages < 1 || r.MaxPages > 100 || r.MaxMessages < 1 || r.MaxMessages > 5000 {
		return r, Invalid("History import limits exceed the contract.")
	}
	return r, nil
}

func (r HistoryImportRequest) Fingerprint() string {
	r.RequestID = ""
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type HistoryImportStatus struct {
	HistoryImportRequest
	OperationID          string  `json:"operation_id"`
	NotificationPolicy   string  `json:"notification_policy"`
	SourceKind           string  `json:"source_kind"`
	State                string  `json:"state"`
	PagesObserved        int     `json:"pages_observed"`
	RecordsObserved      int     `json:"records_observed"`
	InsertedCount        int     `json:"inserted_count"`
	DuplicateCount       int     `json:"duplicate_count"`
	OutOfIntervalCount   int     `json:"out_of_interval_count"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
	EarliestImportedAt   *string `json:"earliest_imported_at"`
	LatestImportedAt     *string `json:"latest_imported_at"`
	SourceHasMore        *bool   `json:"source_has_more"`
	IsFiltered           *bool   `json:"is_filtered"`
	IsFilteredByPhase    *bool   `json:"is_filtered_by_phase"`
	IsFilteredByTimeJoin *bool   `json:"is_filtered_by_time_join"`
	IsOld                *bool   `json:"is_old"`
	JoinTimestampMillis  *string `json:"join_timestamp_millis"`
	HistoryComplete      bool    `json:"history_complete"`
	StopReason           *string `json:"stop_reason"`
}

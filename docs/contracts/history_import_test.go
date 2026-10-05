package contracts

import "testing"

func TestHistoryImportRequestBoundsAndSilentPolicy(t *testing.T) {
	// Arrange
	request, err := Compile("history_import_request", "input")
	if err != nil {
		t.Fatal(err)
	}
	lookup, err := Compile("history_import_operation", "input")
	if err != nil {
		t.Fatal(err)
	}
	output, err := Compile("history_import_operation", "output")
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{"conversation_type": "group", "conversation_id": "synthetic-group", "request_id": "00000000-0000-4000-8000-000000000001", "since": "2026-09-01T00:00:00Z", "until": "2026-10-01T00:00:00Z"}
	for _, tc := range []struct {
		name, field string
		value       any
		valid       bool
	}{
		{"default limits", "", nil, true},
		{"snapshot source", "source", "conversation_preload", true},
		{"legacy source", "source", "group_cloud", true},
		{"arbitrary source", "source", "private-endpoint", false},
		{"maximum page", "page_size", 50, true},
		{"oversized page", "page_size", 51, false},
		{"zero pages", "max_pages", 0, false},
		{"maximum pages", "max_pages", 100, true},
		{"oversized pages", "max_pages", 101, false},
		{"maximum records", "max_messages", 5000, true},
		{"oversized records", "max_messages", 5001, false},
		{"notification override", "notification_policy", "notify", false},
		{"cursor injection", "cursor", "1", false},
		{"callback injection", "callback", "https://example.test", false},
		{"ambiguous date", "since", "01/02/2026", false},
		{"invalid request identity", "request_id", "not-a-uuid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{}
			for k, v := range base {
				args[k] = v
			}
			if tc.field != "" {
				args[tc.field] = tc.value
			}
			// Act
			err := request.Validate(args)
			// Assert
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
	status := map[string]any{
		"operation_id": base["request_id"], "request_id": base["request_id"],
		"conversation_type": "group", "conversation_id": base["conversation_id"],
		"since": base["since"], "until": base["until"], "notification_policy": "none",
		"source_kind": "group_cloud", "state": "partial", "page_size": 50, "max_pages": 20, "max_messages": 1000,
		"pages_observed": 1, "records_observed": 50, "inserted_count": 40, "duplicate_count": 10, "out_of_interval_count": 0,
		"created_at": "2026-10-04T00:00:00Z", "updated_at": "2026-10-04T00:00:01Z",
		"earliest_imported_at": nil, "latest_imported_at": nil, "source_has_more": nil,
		"is_filtered": nil, "is_filtered_by_phase": nil, "is_filtered_by_time_join": nil, "is_old": nil,
		"join_timestamp_millis": "9007199254740993123", "history_complete": false, "stop_reason": "missing_continuation",
	}
	// Act
	err = output.Validate(status)
	lookupErr := lookup.Validate(map[string]any{"operation_id": base["request_id"]})
	// Assert
	if err != nil || lookupErr != nil {
		t.Fatalf("valid operation rejected: output=%v lookup=%v", err, lookupErr)
	}
	for field, value := range map[string]any{"notification_policy": "notify", "history_complete": true, "join_timestamp_millis": float64(9007199254740992), "stop_reason": "raw upstream exception"} {
		original := status[field]
		status[field] = value
		if output.Validate(status) == nil {
			t.Fatalf("invalid %s accepted", field)
		}
		status[field] = original
	}
}

package mobilebackup

import (
	"strconv"
	"unicode/utf8"
)

// These are fixed observations only, never a replacement upstream identity.
func rejectedMessageIDShape(value any) string {
	switch v := value.(type) {
	case nil:
		return "missing"
	case int64:
		if v == 0 {
			return "zero"
		}
		if v < 0 {
			return "negative_integer"
		}
		return "other_invalid"
	case string:
		if len(v) > 64 {
			return "oversized"
		}
		if v == "" {
			return "empty"
		}
		if v == "0" {
			return "zero"
		}
		if len(v) > 1 && v[0] == '-' && asciiDigits(v[1:]) {
			return "negative_text"
		}
		if !asciiDigits(v) {
			return "nondigit"
		}
		if v[0] == '0' {
			return "leading_zero"
		}
		if len(v) > 20 {
			return "overflow"
		}
		if _, err := strconv.ParseUint(v, 10, 64); err != nil {
			return "overflow"
		}
		return "other_invalid"
	default:
		return "wrong_storage_type"
	}
}
func asciiDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
func recordRejectedMessageID(batch *SQLiteBatch, values [9]any) {
	if batch.RejectedMessageIDShapes == nil {
		batch.RejectedMessageIDShapes = map[string]int{}
		batch.RejectedMessageIDContext = map[string]int{}
	}
	batch.RejectedMessageIDShapes[rejectedMessageIDShape(values[1])]++
	status := "invalid_status"
	if n, ok := values[7].(int64); ok {
		status = "nonpositive_status"
		if n > 0 {
			status = "positive_status"
		}
	}
	kind := "unknown_kind"
	if n, ok := values[6].(int64); ok {
		if deferredMobileControl(n) {
			kind = "source_control_kind"
		} else if _, known := mobilePayloadKind(n); known {
			kind = "known_payload_kind"
		}
	}
	text := "source_text_invalid"
	if s, ok := values[3].(string); ok && len(s) <= 1<<20 && utf8.ValidString(s) {
		text = "source_text_absent"
		if s != "" {
			text = "source_text_present"
		}
	}
	client := "client_id_invalid"
	if _, ok := sqliteID(values[2]); ok {
		client = "client_id_valid"
	}
	for _, category := range []string{status, kind, text, client} {
		batch.RejectedMessageIDContext[category]++
	}
}

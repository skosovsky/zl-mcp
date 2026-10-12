package evaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ParityRecord supplements assertion quality with complete behavior equality.
type CanonicalPair struct {
	CaseID          string `json:"case_id"`
	Repeat          int    `json:"repeat"`
	BaselineSHA256  string `json:"baseline_sha256"`
	CandidateSHA256 string `json:"candidate_sha256"`
	Status          string `json:"status"`
}
type ParityRecord struct {
	Pairs    []CanonicalPair `json:"pairs"`
	Status   string          `json:"status"`
	Matched  int             `json:"matched"`
	Failures []string        `json:"failures,omitempty"`
}
type parityCapture struct {
	caseID string
	raw    []byte
}
type canonicalizer struct {
	aliases    map[string]map[string]string
	start, end time.Time
}

var operationPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var eventPattern = regexp.MustCompile(`^evt_[0-9a-f]{32}$`)
var approvalPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func exactJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("trailing capture JSON")
	}
	return value, nil
}
func (c *canonicalizer) alias(kind string, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	s, ok := value.(string)
	if !ok || s == "" {
		return nil, fmt.Errorf("invalid dynamic %s", kind)
	}
	switch kind {
	case "time":
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return nil, fmt.Errorf("invalid dynamic timestamp")
		}
		s = t.UTC().Format(time.RFC3339Nano)
	case "operation":
		if !operationPattern.MatchString(s) {
			return nil, fmt.Errorf("invalid operation ID")
		}
	case "event":
		if !eventPattern.MatchString(s) {
			return nil, fmt.Errorf("invalid event ID")
		}
	case "approval":
		if !approvalPattern.MatchString(s) {
			return nil, fmt.Errorf("invalid approval token")
		}
	case "cursor":
		parts := strings.Split(s, ".")
		if len(parts) > 2 || len(s) < 32 {
			return nil, fmt.Errorf("invalid opaque cursor")
		}
		for _, part := range parts {
			if _, err := base64.RawURLEncoding.DecodeString(part); err != nil || part == "" {
				return nil, fmt.Errorf("invalid opaque cursor")
			}
		}
	case "receipt":
		parts := strings.Split(s, ".")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid receipt")
		}
		for _, part := range parts {
			if _, err := base64.RawURLEncoding.DecodeString(part); err != nil || part == "" {
				return nil, fmt.Errorf("invalid receipt")
			}
		}
	}
	aliases := c.aliases[kind]
	if aliases == nil {
		aliases = map[string]string{}
		c.aliases[kind] = aliases
	}
	if alias, ok := aliases[s]; ok {
		return alias, nil
	}
	alias := fmt.Sprintf("<%s:%d>", kind, len(aliases)+1)
	aliases[s] = alias
	return alias, nil
}

// These paths are fixture-generated values, not a generic key-name allowlist.
func dynamicKind(caseID, tool, path string) string {
	reads := tool == "zalo_search_messages" || tool == "zalo_get_message_context" || tool == "zalo_list_conversation_messages"
	if reads {
		switch path {
		case "snapshot_at", "coverage_observed_at", "coverage.collection_started_at", "coverage.*.collection_started_at", "coverage.known_gaps.*.from", "coverage.*.known_gaps.*.from":
			return "time"
		case "next_cursor":
			return "cursor"
		}
	}
	if tool == "zalo_list_conversation_messages" && strings.HasPrefix(caseID, "archive-") || tool == "zalo_list_archive_sources" && strings.HasPrefix(caseID, "archive-") {
		switch path {
		case "source.captured_at", "source.preserved_at", "source.capture_cache_expires_at", "sources.*.captured_at", "sources.*.preserved_at", "sources.*.capture_cache_expires_at":
			return "time"
		}
	}
	if caseID == "ttl" && tool == "zalo_list_conversation_messages" {
		switch path {
		case "messages.*.sent_at", "coverage.earliest_stored_at", "coverage.latest_stored_at":
			return "time"
		}
	}
	if tool == "zalo_list_conversations" && caseID == "same-name" {
		switch path {
		case "conversations.*.first_discovered_at", "conversations.*.updated_at":
			return "time"
		}
	}
	if tool == "zalo_send_direct_message" || tool == "zalo_get_send_status" {
		if path == "updated_at" {
			return "time"
		}
	}
	if caseID == "join-approved" && (tool == "zalo_join_group" || tool == "zalo_get_join_status") {
		if path == "checked_at" {
			return "time"
		}
		if path == "operation_id" {
			return "operation"
		}
	}
	if caseID == "history-cancel-restart" && (tool == "zalo_import_conversation_history" || tool == "zalo_cancel_history_import" || tool == "zalo_get_history_import_status") {
		if path == "created_at" || path == "updated_at" {
			return "time"
		}
		if path == "operation_id" {
			return "operation"
		}
	}
	if caseID == "events-redelivery" && tool == "zalo_read_subscription_events" {
		switch path {
		case "events.*.event.timestamp", "events.*.event.data.sent_at":
			return "time"
		case "events.*.event.eventId":
			return "event"
		case "events.*.receipt":
			return "receipt"
		}
	}
	return ""
}
func (c *canonicalizer) payload(caseID, tool, path string, value any) (any, error) {
	if kind := dynamicKind(caseID, tool, path); kind != "" {
		if kind == "time" && value != nil && !c.start.IsZero() {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("invalid timestamp type")
			}
			at, err := time.Parse(time.RFC3339Nano, text)
			if err != nil {
				return nil, err
			}
			if caseID == "ttl" && (path == "messages.*.sent_at" || path == "coverage.earliest_stored_at" || path == "coverage.latest_stored_at") {
				at = at.Add(time.Hour)
			}
			if strings.HasSuffix(path, "capture_cache_expires_at") {
				at = at.Add(-time.Hour)
			}
			if at.Before(c.start.Add(-time.Second)) || at.After(c.end.Add(time.Second)) {
				return nil, fmt.Errorf("dynamic timestamp outside fixture execution window")
			}
		}
		return c.alias(kind, value)
	}
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			p := key
			if path != "" {
				p = path + "." + key
			}
			n, err := c.payload(caseID, tool, p, v[key])
			if err != nil {
				return nil, err
			}
			v[key] = n
		}
	case []any:
		for i, item := range v {
			p := "*"
			if path != "" {
				p = path + ".*"
			}
			n, err := c.payload(caseID, tool, p, item)
			if err != nil {
				return nil, err
			}
			v[i] = n
		}
	}
	return value, nil
}
func canonicalCapture(raw []byte) ([]byte, error) {
	decoded, err := exactJSON(raw)
	if err != nil {
		return nil, err
	}
	out, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("capture must be object")
	}
	caseID, ok := out["case_id"].(string)
	if !ok || caseID == "" {
		return nil, fmt.Errorf("missing case ID")
	}
	delete(out, "elapsed_seconds")
	delete(out, "bytes")
	c := canonicalizer{aliases: map[string]map[string]string{}}
	if start, present := out["capture_start_at"]; present {
		end, ok := out["capture_end_at"].(string)
		first, ok2 := start.(string)
		if !ok || !ok2 {
			return nil, fmt.Errorf("invalid host capture window")
		}
		c.start, err = time.Parse(time.RFC3339Nano, first)
		if err != nil {
			return nil, err
		}
		c.end, err = time.Parse(time.RFC3339Nano, end)
		if err != nil || c.end.Before(c.start) {
			return nil, fmt.Errorf("invalid host capture window")
		}
	}
	delete(out, "capture_start_at")
	delete(out, "capture_end_at")
	traces, ok := out["trace"].([]any)
	if !ok {
		return nil, fmt.Errorf("missing trace")
	}
	for _, value := range traces {
		trace, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid trace")
		}
		if trace["kind"] != "tool" {
			continue
		} // Resource text and URI are exact.
		tool, ok := trace["name"].(string)
		if !ok {
			return nil, fmt.Errorf("missing tool name")
		}
		if input, ok := trace["input"].(map[string]any); ok {
			for _, field := range []string{"cursor", "operation_id", "plan_token", "receipt"} {
				value, present := input[field]
				if !present {
					continue
				}
				kind := ""
				switch field {
				case "cursor":
					if tool == "zalo_search_messages" || tool == "zalo_list_conversation_messages" {
						kind = "cursor"
					}
				case "operation_id":
					if caseID == "join-approved" || caseID == "history-cancel-restart" {
						kind = "operation"
					}
				case "plan_token":
					if caseID == "join-approved" && tool == "zalo_join_group" {
						kind = "approval"
					}
				case "receipt":
					if caseID == "events-redelivery" && tool == "zalo_ack_subscription_events" {
						kind = "receipt"
					}
				}
				if kind != "" {
					n, e := c.alias(kind, value)
					if e != nil {
						return nil, e
					}
					input[field] = n
				}
			}
		}
		result, ok := trace["result"].(map[string]any)
		if !ok {
			continue
		}
		// Both projections are read independently; equal dynamic values reuse aliases.
		if value, present := result["structuredContent"]; present {
			n, e := c.payload(caseID, tool, "", value)
			if e != nil {
				return nil, e
			}
			result["structuredContent"] = n
		}
		if content, ok := result["content"].([]any); ok {
			for _, item := range content {
				block, ok := item.(map[string]any)
				if !ok || block["type"] != "text" {
					continue
				}
				text, ok := block["text"].(string)
				if !ok {
					continue
				}
				parsed, e := exactJSON([]byte(text))
				if e != nil {
					continue
				}
				n, e := c.payload(caseID, tool, "", parsed)
				if e != nil {
					return nil, e
				}
				// Keep the text projection distinct from structured content.
				encoded, e := json.Marshal(n)
				if e != nil {
					return nil, e
				}
				block["text"] = string(encoded)
			}
		}
	}
	// Preserve the temporal order as well as equality/reuse, without requiring
	// identical wall clocks across independently prepared fixtures.
	times := make([]string, 0, len(c.aliases["time"]))
	for value := range c.aliases["time"] {
		times = append(times, value)
	}
	sort.Slice(times, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, times[i])
		b, _ := time.Parse(time.RFC3339Nano, times[j])
		return a.Before(b)
	})
	order := make([]string, 0, len(times))
	for _, value := range times {
		order = append(order, c.aliases["time"][value])
	}
	out["_harness_dynamic_time_order"] = order
	return json.Marshal(out)
}
func compareCaptures(baseline, candidate []parityCapture) ParityRecord {
	record := ParityRecord{Status: "PASS"}
	indexed := map[string][]parityCapture{}
	for _, capture := range candidate {
		indexed[capture.caseID] = append(indexed[capture.caseID], capture)
	}
	seen := map[string]int{}
	for _, b := range baseline {
		i := seen[b.caseID]
		seen[b.caseID]++
		peers := indexed[b.caseID]
		label := fmt.Sprintf("%s/repeat-%d", b.caseID, i+1)
		if i >= len(peers) {
			record.Failures = append(record.Failures, label+":missing candidate")
			continue
		}
		left, e1 := canonicalCapture(b.raw)
		right, e2 := canonicalCapture(peers[i].raw)
		pair := CanonicalPair{CaseID: b.caseID, Repeat: i + 1, Status: "FAIL"}
		if e1 == nil {
			sum := sha256.Sum256(left)
			pair.BaselineSHA256 = hex.EncodeToString(sum[:])
		}
		if e2 == nil {
			sum := sha256.Sum256(right)
			pair.CandidateSHA256 = hex.EncodeToString(sum[:])
		}
		if e1 != nil || e2 != nil || !bytes.Equal(left, right) {
			record.Failures = append(record.Failures, label+":capture differs or invalid")
		} else {
			record.Matched++
			pair.Status = "PASS"
		}
		record.Pairs = append(record.Pairs, pair)
	}
	if len(baseline) == 0 || len(baseline) != len(candidate) {
		record.Failures = append(record.Failures, "incomplete capture counts")
	}
	for id, peers := range indexed {
		if len(peers) != seen[id] {
			record.Failures = append(record.Failures, id+":unmatched captures")
		}
	}
	sort.Strings(record.Failures)
	if len(record.Failures) > 0 {
		record.Status = "FAIL"
	}
	return record
}

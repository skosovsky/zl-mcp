package evaluation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStandaloneContractsRejectContentIdentityAndEffectRegressions(t *testing.T) {
	// Arrange: fixed synthetic expectations replace comparison to an old binary.
	cs := Case{ID: "contracts", ExpectedEffects: map[string]int{}, Checks: []Check{
		{Name: "exact-id", Step: -1, Path: "id", Op: "equal", Value: json.Number("9007199254740993")},
		{Name: "exact-native-id", Step: -1, Path: "native_id", Op: "equal", Value: "9007199254740995"},
		{Name: "exact-string-id", Step: -1, Path: "string_id", Op: "equal", Value: "9007199254740993"},
		{Name: "exact-content", Step: 0, Path: "text", Op: "equal", Value: "Cà phê 🍀 2026-10-01T10:00:00Z"},
		{Name: "explicit-null", Step: 0, Path: "next_cursor", Op: "equal", Value: nil},
	}}
	for _, mutation := range []string{"none", "numeric-id", "native-id", "string-id", "text", "fixed-time", "missing-null", "effect"} {
		t.Run(mutation, func(t *testing.T) {
			facts := map[string]any{
				"id":        json.Number("9007199254740993"),
				"native_id": "9007199254740995",
				"string_id": "9007199254740993",
			}
			payload := map[string]any{"text": "Cà phê 🍀 2026-10-01T10:00:00Z", "next_cursor": nil}
			effects := map[string]int{}
			switch mutation {
			case "numeric-id":
				facts["id"] = json.Number("9007199254740992")
			case "native-id":
				facts["native_id"] = "9007199254740994"
			case "string-id":
				facts["string_id"] = "9007199254740992"
			case "text":
				payload["text"] = "changed text"
			case "fixed-time":
				payload["text"] = "Cà phê 🍀 2026-10-01T10:00:01Z"
			case "missing-null":
				delete(payload, "next_cursor")
			case "effect":
				effects["send"] = 1
			}
			out := Output{
				CaseID:   cs.ID,
				Complete: true,
				Cleanup:  true,
				Facts:    facts,
				Effects:  effects,
				Trace:    []Trace{{Kind: "resource", Result: marshaled(payload)}},
			}
			// Act.
			assertions := Grade(cs, out)
			// Assert.
			passed := true
			for _, assertion := range assertions {
				passed = passed && assertion.Pass
			}
			if passed != (mutation == "none") {
				t.Fatalf("mutation %s: %+v", mutation, assertions)
			}
		})
	}
}

func TestStandaloneProjectionMismatchCannotPass(t *testing.T) {
	// Arrange: dynamic timestamp differs only in the text projection.
	payload := map[string]any{"snapshot_at": "2026-10-12T00:00:00Z"}
	text := map[string]any{"snapshot_at": "2026-10-12T00:00:01Z"}
	cs := Case{ID: "projection", AllowedTools: []string{"zalo_search_messages"}}
	out := Output{
		CaseID:   cs.ID,
		Complete: true,
		Cleanup:  true,
		Trace: []Trace{
			{
				Kind: "tool",
				Name: "zalo_search_messages",
				Result: marshaled(
					map[string]any{
						"structuredContent": payload,
						"content":           []any{map[string]any{"type": "text", "text": string(marshaled(text))}},
					},
				),
			},
		},
	}
	// Act.
	assertions := Grade(cs, out)
	// Assert: inspect projection independently of intentionally incomplete schema.
	for _, assertion := range assertions {
		if assertion.Name == "text-structured-projection" {
			if assertion.Pass {
				t.Fatal("mismatched text projection accepted")
			}
			return
		}
	}
	t.Fatal("projection assertion missing")
}

func TestDecodePreservesNumericIdentityAndRejectsTrailingJSON(t *testing.T) {
	// Arrange.
	for _, raw := range []string{`{"id":9007199254740993}`, `{"id":9007199254740993} {}`, `{"id":`} {
		// Act.
		value, err := decode([]byte(raw))
		// Assert.
		if raw == `{"id":9007199254740993}` {
			if err != nil || value.(map[string]any)["id"] != json.Number("9007199254740993") {
				t.Fatal(value, err)
			}
		} else if err == nil {
			t.Fatalf("invalid JSON accepted: %s", raw)
		}
	}
}

func TestDatasetRequiresExactSDKEnvelopeIDWithoutBaseline(t *testing.T) {
	// Arrange: load the actual offline contract instead of a test-only expectation.
	dataset, err := loadDataset(filepath.Join("..", "..", "docs", "evals", "migration-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cs Case
	for _, candidate := range dataset.Cases {
		if candidate.ID == "numeric-precision" {
			cs = candidate
			break
		}
	}
	if cs.ID == "" {
		t.Fatal("numeric precision case missing")
	}
	for _, id := range []string{"9007199254740992", "9007199254740993"} {
		out := Output{
			CaseID:   cs.ID,
			Complete: true,
			Cleanup:  true,
			Facts:    map[string]any{"sdk-envelope-9007199254740993": id},
		}
		// Act.
		assertions := Grade(cs, out)
		// Assert: the production dataset must reject the previously rounded SDK ID.
		found := false
		for _, assertion := range assertions {
			if assertion.Name == "SDK-numeric-envelope-exact" {
				found = true
				if assertion.Pass != (id == "9007199254740993") {
					t.Fatalf("SDK ID %s: %+v", id, assertion)
				}
			}
		}
		if !found {
			t.Fatal("mandatory exact SDK ID assertion missing")
		}
	}
}

func TestIncompleteStandaloneChildCannotPass(t *testing.T) {
	// Arrange: an exit-zero child omits operation evidence.
	dir := t.TempDir()
	binary := filepath.Join(dir, "incomplete-child")
	if err := os.WriteFile(
		binary,
		[]byte("#!/bin/sh\nprintf '%s' '{\"complete\":false,\"cleanup\":true}'\n"),
		0700,
	); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	report, err := Run(
		context.Background(),
		Options{
			Root:        root,
			Binary:      binary,
			ReportDir:   filepath.Join(dir, "reports"),
			DatasetPath: filepath.Join(root, "docs/evals/migration-v1.json"),
			Profile:     "offline",
			RunBudget:   time.Minute,
			Repeats:     1,
			Seed:        42,
		},
	)
	// Assert.
	if err != nil || report.Status != "FAIL" || report.ExitStatus == 0 || report.PassedCases != 0 ||
		report.Experiment == nil ||
		len(report.Experiment.Trials) != report.ExpectedCases {
		t.Fatalf("incomplete child accepted: %+v; %v", report, err)
	}
	if report.ModelGate != "NOT_RUN" || report.LiveGate != "NOT_RUN" {
		t.Fatal("offline changed deferred gates")
	}
}

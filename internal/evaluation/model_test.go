package evaluation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestModelAdmissionBeforeDispatch(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		// Arrange: the server owns admission and dispatch counting.
		capture := &modelCapture{allowed: map[string]bool{"synthetic": allowed}}
		dispatched := 0
		handler := capture.middleware(func(context.Context, string, mcp.Request) (mcp.Result, error) {
			dispatched++
			return &mcp.CallToolResult{}, nil
		})
		req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{Params: &mcp.CallToolParamsRaw{Name: "synthetic", Arguments: []byte(`{}`)}}
		// Act: attempt one more than the fixed model budget.
		var lastErr error
		for i := 0; i < 31; i++ {
			_, lastErr = handler(context.Background(), "tools/call", req)
		}
		// Assert: violations are sticky and never dispatch.
		expected := 0
		if allowed {
			expected = 30
		}
		if dispatched != expected || lastErr == nil || !capture.violated {
			t.Fatal(dispatched, lastErr, capture.violated)
		}
	}
}
func TestModelOptInAbsentAndUnknownProfile(t *testing.T) {
	// Arrange
	opts := Options{Profile: "model", RunBudget: 1, Repeats: 1}
	// Act
	report, err := Run(context.Background(), opts)
	// Assert
	if err != nil || report.ModelGate != "NOT_RUN" || report.ExitStatus != 2 || report.Manifest["max_tool_calls"] != 30 {
		t.Fatal(report, err)
	}
	// Arrange: a misspelled profile must not silently become a skipped model run.
	opts.Profile = "typo"
	// Act
	_, err = Run(context.Background(), opts)
	// Assert
	if err == nil {
		t.Fatal("unknown profile accepted")
	}
}
func TestCrossTraceIdentityRegression(t *testing.T) {
	// Arrange: a repeated first page or replaced event must fail.
	for _, op := range []string{"same-as", "different-from"} {
		first, second := `{"id":"one"}`, `{"id":"two"}`
		if op == "different-from" {
			second = first
		}
		cs := Case{ID: "case", Checks: []Check{{Name: "identity", Step: 1, Path: "id", Op: op, Value: map[string]any{"step": 0, "path": "id"}}}}
		out := Output{CaseID: "case", Trace: []Trace{{Result: []byte(first)}, {Result: []byte(second)}}}
		// Act
		assertions := Grade(cs, out)
		// Assert
		if assertions[len(assertions)-1].Pass {
			t.Fatal("semantic identity regression passed")
		}
	}
}

func TestFullModelGatePreflightNeverLaunchesDriver(t *testing.T) {
	// Arrange: a sentinel driver would leave a marker if it were executed.
	dir := t.TempDir()
	marker := filepath.Join(dir, "launched")
	driver := filepath.Join(dir, "driver")
	if err := os.WriteFile(driver, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	opts := Options{Profile: "model", ModelDriver: driver, DatasetPath: "../../docs/evals/migration-v1.json", RunBudget: time.Minute, Repeats: 1, ReportDir: dir}
	// Act
	report, err := Run(context.Background(), opts)
	_, statErr := os.Stat(marker)
	// Assert: model subset/scaffolding cannot certify the full gate.
	if err != nil || report.ExitStatus != 2 || report.ModelGate != "NOT_RUN" || report.Manifest["full_model_gate"] != "NOT_RUN" || len(report.SkippedCases) == 0 || !os.IsNotExist(statErr) {
		t.Fatal(report, err, statErr)
	}
}
func TestUnsupportedModelScenarioRejectedBeforeFixture(t *testing.T) {
	// Arrange
	opts := ChildOptions{StateDir: filepath.Join(t.TempDir(), "state")}
	// Act
	_, err := RunModelCase(context.Background(), opts, Case{ID: "empty-corpus"}, "/tmp/nonexistent-model-driver")
	_, statErr := os.Stat(opts.StateDir)
	// Assert
	if err == nil || !os.IsNotExist(statErr) {
		t.Fatal(err, statErr)
	}
}

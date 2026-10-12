package evaluation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/evaly"
)

func TestPairedModeRejectsModelAndRelativeBaselineBeforeExecution(t *testing.T) {
	for _, opts := range []Options{
		{Profile: "model", BaselineBinary: "/private/nonexistent", ModelDriver: "/private/unauthorized", RunBudget: time.Second, Repeats: 1},
		{Profile: "offline", BaselineBinary: "relative", RunBudget: time.Second, Repeats: 1},
	} {
		// Arrange/Act: malformed mode is rejected before dataset or process work.
		report, err := Run(context.Background(), opts)
		// Assert.
		if err == nil || report.Status == "PASS" || report.Experiment != nil || report.Comparison != nil || report.ModelGate != "NOT_RUN" || report.LiveGate != "NOT_RUN" {
			t.Fatal("invalid paired configuration entered execution")
		}
	}
}

func TestIncompletePairedSidesCannotPassComparison(t *testing.T) {
	// Arrange: both synthetic child stubs exit0 but deliberately omit valid evidence.
	dir := t.TempDir()
	binary := filepath.Join(dir, "incomplete-child")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s' '{\"complete\":false,\"cleanup\":true}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "reports")
	if err = os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	// Act: all sealed mandatory cases run on two independent sides.
	report, err := Run(context.Background(), Options{Root: root, Binary: binary, BaselineBinary: binary, ReportDir: output, DatasetPath: filepath.Join(root, "docs/evals/migration-v1.json"), Profile: "offline", RunBudget: time.Minute, Repeats: 1, Seed: 42})
	// Assert: a zero process exit is insufficient for offline acceptance.
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "FAIL" || report.ExitStatus == 0 || report.Comparison == nil || report.Comparison.Verdict == evaly.GatePass || report.BaselineExperiment == nil || len(report.BaselineExperiment.Trials) != report.ExpectedCases || report.PassedCases != 0 {
		t.Fatal("missing paired evidence was accepted")
	}
	if report.ModelGate != "NOT_RUN" || report.LiveGate != "NOT_RUN" {
		t.Fatal("paired offline changed deferred gates")
	}
}

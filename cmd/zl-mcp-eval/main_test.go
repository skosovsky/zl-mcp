package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/evaluation"
)

func TestMain(m *testing.M) {
	// This copied Go test executable is a sentinel, never an authorized model.
	if filepath.Base(os.Args[0]) == "sentinel-driver" {
		if err := os.WriteFile(filepath.Join(filepath.Dir(os.Args[0]), "model-was-launched"), []byte("unexpected execution"), 0600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestCLIExitCodesAndFailedReports(t *testing.T) {
	// Arrange: build the real CLI once; use only local synthetic assets.
	dir := t.TempDir()
	binary := filepath.Join(dir, "zl-mcp-eval")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	datasetPath := filepath.Join(root, "docs", "evals", "migration-v1.json")
	raw, err := os.ReadFile(datasetPath)
	if err != nil {
		t.Fatal(err)
	}
	var dataset evaluation.Dataset
	if err := json.Unmarshal(raw, &dataset); err != nil {
		t.Fatal(err)
	}
	dataset.Cases[0].Checks = append(dataset.Cases[0].Checks, evaluation.Check{Name: "deliberate-failure", Step: 0, Path: "structuredContent.messages.0.message_id", Op: "equal", Value: "impossible"})
	failingDataset := filepath.Join(dir, "failing-dataset.json")
	raw, err = json.Marshal(dataset)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failingDataset, raw, 0600); err != nil {
		t.Fatal(err)
	}
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(testBinary)
	if err != nil {
		t.Fatal(err)
	}
	driver := filepath.Join(dir, "sentinel-driver")
	if err := os.WriteFile(driver, raw, 0700); err != nil {
		t.Fatal(err)
	}
	controls := []struct {
		name   string
		exit   int
		status string
		args   []string
	}{
		{"assertion-failure", 1, "FAIL", []string{"--dataset", failingDataset}},
		{"missing-child", 1, "FAIL", []string{"--child-binary", filepath.Join(dir, "missing-binary")}},
		{"global-timeout", 2, "INCOMPLETE", []string{"--run-budget", "1ms"}},
		{"model-not-run", 2, "INCOMPLETE", []string{"--profile", "model"}},
		{"model-full-preflight", 2, "INCOMPLETE", []string{"--profile", "model", "--model-driver", driver}},
		{"unknown-profile", 2, "INCOMPLETE", []string{"--profile", "typo"}},
	}
	for _, control := range controls {
		t.Run(control.name, func(t *testing.T) {
			// Arrange: each invocation owns a fresh report directory.
			reportDir := filepath.Join(dir, control.name)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			args := append([]string{"--root", root, "--output", reportDir}, control.args...)
			command := exec.CommandContext(ctx, binary, args...)
			// Act.
			output, err := command.CombinedOutput()
			// Assert: an artifact or skipped model cannot imply acceptance.
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != control.exit || ctx.Err() != nil {
				t.Fatalf("want exit %d: %v\n%s", control.exit, err, output)
			}
			raw, err := os.ReadFile(filepath.Join(reportDir, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			var report evaluation.Report
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			if report.ExitStatus != control.exit || report.Status != control.status || report.ModelGate != "NOT_RUN" || report.LiveGate != "NOT_RUN" {
				t.Fatalf("invalid failure report: %+v", report)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(dir, "model-was-launched")); !os.IsNotExist(err) {
		t.Fatal("model preflight executed sentinel driver", err)
	}
}

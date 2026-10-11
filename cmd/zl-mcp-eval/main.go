package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/skosovsky/zl-mcp/internal/evaluation"
)

func main() { os.Exit(run()) }
func run() int {
	root := flag.String("root", ".", "repository with versioned synthetic assets")
	profile := flag.String("profile", "offline", "offline or explicit opt-in model profile (requires authorized driver)")
	driver := flag.String("model-driver", "", "explicit authorized absolute host driver; absent model profile is NOT_RUN")
	output := flag.String("output", "/tmp/zl-mcp-offline-report", "new report directory")
	binary := flag.String("child-binary", "", "baseline/candidate evaluation child binary; defaults to this executable")
	dataset := flag.String("dataset", "docs/evals/migration-v1.json", "versioned synthetic dataset")
	budget := flag.Duration("run-budget", 30*time.Minute, "explicit global wall-clock budget")
	repeats := flag.Int("repeats", 1, "independent repeats per case")
	seed := flag.Int64("seed", 1376, "reproducible evaluation seed")
	measure := flag.String("measure-kind", "", "internal performance worker:read/search/context")
	calls := flag.Int("measure-calls", 1000, "measured calls, minimum1000")
	child := flag.String("child-case", "", "internal isolated case ID")
	state := flag.String("state-dir", "", "internal parent-owned synthetic state directory")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments")
		return 2
	}
	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *measure != "" {
		if *state == "" || !filepath.IsAbs(*state) {
			fmt.Fprintln(os.Stderr, "parent-owned absolute state required")
			return 2
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		result, e := evaluation.Measure(ctx, absoluteRoot, *state, *measure, *calls)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
		}
		if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
			return 1
		}
		if e != nil {
			return 1
		}
		return 0
	}
	if *child != "" {
		if *state == "" || !filepath.IsAbs(*state) {
			fmt.Fprintln(os.Stderr, "parent-owned absolute state required")
			return 2
		}
		caseBudget := 60 * time.Second
		if *profile == "model" {
			caseBudget = 180 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), caseBudget)
		defer cancel()
		childOpts := evaluation.DefaultChildOptions(absoluteRoot, *state, *child)
		var result evaluation.Output
		var childErr error
		if *profile == "model" {
			if !filepath.IsAbs(*dataset) {
				*dataset = filepath.Join(absoluteRoot, *dataset)
			}
			cs, e := evaluation.FindCase(*dataset, *child)
			if e != nil {
				childErr = e
			} else {
				result, childErr = evaluation.RunModelCase(ctx, childOpts, cs, *driver)
			}
		} else {
			result, childErr = evaluation.RunCase(ctx, childOpts)
		}
		if childErr != nil {
			result.Failure = childErr.Error()
			result.Complete = false
		}
		if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
			return 1
		}
		if childErr != nil {
			return 1
		}
		return 0
	}
	if *binary == "" {
		*binary, err = os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	absoluteOutput, err := filepath.Abs(*output)
	if err != nil {
		return 2
	}
	if err = os.Mkdir(absoluteOutput, 0700); err != nil {
		fmt.Fprintln(os.Stderr, "report directory must be new:", err)
		return 2
	}
	if !filepath.IsAbs(*dataset) {
		*dataset = filepath.Join(absoluteRoot, *dataset)
	}
	report, runErr := evaluation.Run(context.Background(), evaluation.Options{Root: absoluteRoot, Binary: *binary, ReportDir: absoluteOutput, DatasetPath: *dataset, Profile: *profile, ModelDriver: *driver, RunBudget: *budget, Seed: *seed, Repeats: *repeats})
	if runErr != nil {
		report.Reason = runErr.Error()
		if report.ExitStatus == 0 {
			report.ExitStatus = 1
		}
		fmt.Fprintln(os.Stderr, runErr)
	}
	file, err := os.OpenFile(filepath.Join(absoluteOutput, "report.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	err = json.NewEncoder(file).Encode(report)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return 1
	}
	fmt.Printf("%s cases%d/%d assertions%d/%d; model=%s live=%s\n", report.Status, report.PassedCases, report.ExpectedCases, report.PassedAssertions, report.ExpectedAssertions, report.ModelGate, report.LiveGate)
	return report.ExitStatus
}

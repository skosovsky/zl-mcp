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

//nolint:funlen // The fixed CLI flag table and mode selection share one FlagSet; mode execution is split into separate handlers.
func run() int {
	checkpoint := flag.String(
		"grade-checkpoints",
		"",
		"offline host observation JSON; checkpoints only, never launches a driver",
	)
	checkpointDataset := flag.String(
		"checkpoint-dataset",
		"docs/evals/model-checkpoints-v1.json",
		"versioned checkpoint dataset",
	)
	root := flag.String("root", ".", "repository with versioned synthetic assets")
	profile := flag.String(
		"profile",
		"offline",
		"offline or explicit opt-in model profile (requires authorized driver)",
	)
	driver := flag.String(
		"model-driver",
		"",
		"explicit authorized absolute host driver; absent model profile is NOT_RUN",
	)
	output := flag.String("output", "/tmp/zl-mcp-offline-report", "new report directory")
	binary := flag.String("child-binary", "", "evaluation child binary; defaults to this executable")
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
	if *checkpoint != "" {
		return runCheckpointMode(absoluteRoot, *checkpointDataset, *checkpoint, *profile, *driver, *child, *measure)
	}
	if *measure != "" {
		return runMeasurementMode(absoluteRoot, *state, *measure, *calls)
	}
	if *child != "" {
		return runChildMode(absoluteRoot, *state, *child, *profile, *dataset, *driver)
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
	report, runErr := evaluation.Run(
		context.Background(),
		evaluation.Options{
			Root:        absoluteRoot,
			Binary:      *binary,
			ReportDir:   absoluteOutput,
			DatasetPath: *dataset,
			Profile:     *profile,
			ModelDriver: *driver,
			RunBudget:   *budget,
			Seed:        *seed,
			Repeats:     *repeats,
		},
	)
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
	fmt.Printf(
		"%s cases%d/%d assertions%d/%d; model=%s live=%s\n",
		report.Status,
		report.PassedCases,
		report.ExpectedCases,
		report.PassedAssertions,
		report.ExpectedAssertions,
		report.ModelGate,
		report.LiveGate,
	)
	return report.ExitStatus
}

func runChildMode(absoluteRoot string, state string, child string, profile string, dataset string, driver string) int {
	var err error

	if state == "" || !filepath.IsAbs(state) {
		fmt.Fprintln(os.Stderr, "parent-owned absolute state required")
		return 2
	}
	caseBudget := 60 * time.Second
	if profile == "model" {
		caseBudget = 180 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), caseBudget)
	defer cancel()
	childOpts := evaluation.DefaultChildOptions(absoluteRoot, state, child)
	var result evaluation.Output
	var childErr error
	if profile == "model" {
		if !filepath.IsAbs(dataset) {
			dataset = filepath.Join(absoluteRoot, dataset)
		}
		cs, e := evaluation.FindCase(dataset, child)
		if e != nil {
			childErr = e
		} else {
			result, childErr = evaluation.RunModelCase(ctx, childOpts, cs, driver)
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

func runMeasurementMode(absoluteRoot string, state string, measure string, calls int) int {
	var err error

	if state == "" || !filepath.IsAbs(state) {
		fmt.Fprintln(os.Stderr, "parent-owned absolute state required")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	result, e := evaluation.Measure(ctx, absoluteRoot, state, measure, calls)
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

func runCheckpointMode(absoluteRoot string, checkpointDataset string, checkpoint string, profile string, driver string, child string, measure string) int {

	if profile != "offline" || driver != "" || child != "" || measure != "" {
		fmt.Fprintln(os.Stderr, "checkpoint grading is offline only")
		return 2
	}
	if !filepath.IsAbs(checkpointDataset) {
		checkpointDataset = filepath.Join(absoluteRoot, checkpointDataset)
	}
	report, e := evaluation.GradeCheckpointFile(checkpointDataset, checkpoint)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 2
	}
	if e = json.NewEncoder(os.Stdout).Encode(report); e != nil {
		return 1
	}
	if report.Status != "PASS" {
		return 1
	}
	return 0

}

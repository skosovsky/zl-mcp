package evaluation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/skosovsky/evaly"
)

type Options struct {
	Root, Binary, ReportDir, DatasetPath, Profile, ModelDriver string
	RunBudget                                                  time.Duration
	Seed                                                       int64
	Repeats                                                    int
}
type Report struct {
	Status             string                  `json:"status"`
	ExitStatus         int                     `json:"exit_status"`
	ModelGate          string                  `json:"model_gate"`
	LiveGate           string                  `json:"live_gate"`
	Reason             string                  `json:"reason,omitempty"`
	Manifest           map[string]any          `json:"manifest"`
	Experiment         *evaly.ExperimentRecord `json:"experiment,omitempty"`
	ExpectedCases      int                     `json:"expected_cases"`
	PassedCases        int                     `json:"passed_cases"`
	ExpectedAssertions int                     `json:"expected_assertions"`
	PassedAssertions   int                     `json:"passed_assertions"`
	FailedCases        []string                `json:"failed_cases"`
	SkippedCases       []string                `json:"skipped_cases"`
}
type environment struct{ dir, traceDir string }

func loadDataset(path string) (Dataset, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Dataset{}, err
	}
	var ds Dataset
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&ds); err != nil {
		return ds, err
	}
	if err = decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ds, errors.New("trailing dataset content")
	}
	if len(ds.Cases) < 22 || ds.Revision == "" || ds.GraderRevision == "" {
		return ds, errors.New("incomplete dataset")
	}
	ids := map[string]bool{}
	for _, cs := range ds.Cases {
		if cs.ID == "" || ids[cs.ID] || cs.TimeoutSeconds != 60 || len(cs.Checks) == 0 {
			return ds, fmt.Errorf("invalid case %q", cs.ID)
		}
		if err := validateCaseChecks(cs); err != nil {
			return ds, err
		}
		for kind, count := range cs.ExpectedEffects {
			if count < 0 || (kind != "send" && kind != "join" && kind != "history") {
				return ds, fmt.Errorf("invalid effects in %s", cs.ID)
			}
		}
		ids[cs.ID] = true
	}
	return ds, nil
}
func hashFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "unavailable: " + err.Error()
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func readEffects(path string) (map[string]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// G104: Read-only input file cleanup cannot replace the parse error.
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 64<<10 {
		return nil, errors.New("effect journal overflow")
	}
	counts := map[string]int{}
	for line := range strings.SplitSeq(string(b), "\n") {
		if line == "" {
			continue
		}
		var effect struct {
			Kind string `json:"kind"`
		}
		if err = json.Unmarshal([]byte(line), &effect); err != nil || effect.Kind == "" {
			return counts, errors.New("incomplete effect journal")
		}
		counts[effect.Kind]++
	}
	return counts, nil
}
func Run(ctx context.Context, opts Options) (Report, error) {
	var report Report
	var err error
	report = Report{
		Status:     "INCOMPLETE",
		ExitStatus: 2,
		ModelGate:  "NOT_RUN",
		LiveGate:   "NOT_RUN",
		Manifest:   map[string]any{},
	}
	if opts.RunBudget <= 0 || opts.Repeats <= 0 {
		return report, errors.New("explicit positive run budget/repeats required")
	}
	if opts.Profile != "offline" && opts.Profile != "model" {
		return report, errors.New("unknown evaluation profile")
	}
	if opts.Profile == "model" && opts.ModelDriver == "" {
		report.Manifest = map[string]any{
			"profile":             opts.Profile,
			"case_budget_seconds": 180,
			"max_tool_calls":      30,
			"run_budget_seconds":  opts.RunBudget.Seconds(),
			"model_gate":          "NOT_RUN",
			"live_gate":           "NOT_RUN",
		}
		report.Reason = "Model profile requires a separately authorized host agent driver; no model executed"
		return report, nil
	}
	ds, err := loadDataset(opts.DatasetPath)
	if err != nil {
		return report, err
	}
	if opts.Profile == "model" {
		report.Manifest = map[string]any{
			"profile":             "model",
			"case_budget_seconds": 180,
			"max_tool_calls":      30,
			"run_budget_seconds":  opts.RunBudget.Seconds(),
			"model_gate":          "NOT_RUN",
			"full_model_gate":     "NOT_RUN",
			"model_driver_sha256": hashFile(opts.ModelDriver),
			"model_id":            nil,
			"model_settings":      nil,
			"prompt_revision":     nil,
			"skill_revision":      nil,
			"tokens":              nil,
			"unavailable_reason":  "No separately authorized full scenario model driver run",
		}
		for _, cs := range ds.Cases {
			if !modelScenarioSupported(cs.ID) {
				report.SkippedCases = append(report.SkippedCases, cs.ID)
			}
		}
		report.Reason = "Protocol scaffolding only: full paired model scenario lifecycle and acceptance remain NOT_RUN; no driver executed"
		return report, nil
	}
	if err = os.MkdirAll(opts.ReportDir, 0700); err != nil {
		return report, err
	}
	run, cancel := context.WithTimeout(ctx, opts.RunBudget)
	defer cancel()
	started := time.Now()
	report.ExpectedCases = len(ds.Cases) * opts.Repeats
	for _, cs := range ds.Cases {
		report.ExpectedAssertions += (len(cs.Checks) + 6) * opts.Repeats
	}
	report.Manifest = offlineManifest(opts, ds)
	attachBuildProvenance(&report, opts.Binary)

	codec := evaly.JSONCodec[Case]{ID: "zl-mcp-case", Version: ds.Revision}
	draft := evaly.DatasetDraft[Case, Case]{Selection: "all"}
	for _, cs := range ds.Cases {
		reference := cs
		draft.Cases = append(draft.Cases, evaly.Case[Case, Case]{ID: cs.ID, Input: cs, Reference: &reference})
	}
	dataset, err := draft.Seal(codec, codec)
	if err != nil {
		return report, err
	}
	fixtureHash, ok := report.Manifest["fixture_sha256"].(string)
	if !ok {
		return report, errors.New("invalid fixture digest")
	}

	experiment, err := evaly.Run(run, offlineRunConfig(opts, ds, dataset, fixtureHash))
	report.Manifest["elapsed_seconds"] = time.Since(started).Seconds()
	if err != nil {
		return report, err
	}
	record := experiment.Record()
	report.Experiment = &record
	gradeOfflineExperiment(&report, opts, ds, record, run.Err())
	return report, nil
}

// FindCase loads the same sealed input contract used by the parent.
func FindCase(datasetPath, id string) (Case, error) {
	ds, err := loadDataset(datasetPath)
	if err != nil {
		return Case{}, err
	}
	for _, cs := range ds.Cases {
		if cs.ID == id {
			return cs, nil
		}
	}
	return Case{}, fmt.Errorf("unknown case %q", id)
}

func validateCaseChecks(cs Case) error {
	names := map[string]bool{}
	for _, check := range cs.Checks {
		if check.Name == "" || names[check.Name] || check.Step < -2 {
			return fmt.Errorf("invalid check in %s", cs.ID)
		}
		switch check.Op {
		case "same-as",
			"different-from",
			"equal",
			"length",
			"min-length",
			"gte",
			"contains",
			"success",
			"resource-error",
			"code",
			"one-of":
		default:
			return fmt.Errorf("unknown check op %q", check.Op)
		}
		names[check.Name] = true
	}
	return nil
}

func attachBuildProvenance(report *Report, binary string) {
	// Only attach build provenance when the recorded executable digest matches.
	buildPath := filepath.Join(filepath.Dir(binary), "build-manifest.json")
	raw, readErr := os.ReadFile(buildPath)
	if readErr != nil {
		return
	}

	var build struct {
		Builds []struct {
			Kind   string `json:"kind"`
			Digest string `json:"binary_sha256"`
		} `json:"builds"`
	}
	if json.Unmarshal(raw, &build) == nil {
		for _, b := range build.Builds {
			if b.Digest == report.Manifest["binary_sha256"] {
				var provenance any
				if json.Unmarshal(raw, &provenance) == nil {
					report.Manifest["build_provenance"] = provenance
					report.Manifest["build_kind"] = b.Kind
					report.Manifest["build_manifest_sha256"] = hashFile(buildPath)
				}
			}
		}
	}

}
func offlineRunConfig(opts Options, ds Dataset, dataset evaly.Dataset[Case, Case], fixtureHash string) evaly.RunConfig[Case, Output, Case, *environment] {
	return evaly.RunConfig[Case, Output, Case, *environment]{
		ID:          "offline",
		Dataset:     dataset,
		OutputCodec: evaly.JSONCodec[Output]{ID: "zl-mcp-output", Version: "1"},
		Lifecycle: evaly.LifecycleFuncs[*environment]{
			IdentityValue: evaly.LifecycleIdentity{
				Fixture:   fixtureHash,
				Reset:     "fresh-state-v1",
				Isolation: evaly.Isolated,
			},
			PrepareFunc: func(_ context.Context, id string) (*environment, error) {
				dir, e := os.MkdirTemp("", "zl-mcp-case-")
				if e != nil {
					return nil, e
				}
				sum := sha256.Sum256([]byte(id))
				trace := filepath.Join(opts.ReportDir, hex.EncodeToString(sum[:8]))
				if e = os.Mkdir(trace, 0700); e != nil {
					//nolint:gosec // G104: synthetic fixture/session cleanup preserves the primary evaluation error.
					os.RemoveAll(dir)
					return nil, e
				}
				return &environment{dir: dir, traceDir: trace}, nil
			},
			ResetFunc: func(_ context.Context, e *environment) error {
				return os.WriteFile(filepath.Join(e.dir, "effects.jsonl"), nil, 0600)
			},
			CleanupFunc: func(_ context.Context, e *environment) error { return os.RemoveAll(e.dir) },
		},
		Target: evaly.TargetFunc[Case, Output, *environment](
			func(ctx context.Context, cs Case, trial evaly.TrialContext[*environment]) (evaly.TargetResult[Output], error) {
				return executeOfflineCase(ctx, opts, cs, trial)
			},
		),
		Capture: evaly.CaptureConfig{
			Policy: evaly.FieldPolicy{
				ID: "synthetic-metadata-v1",
				Allowed: map[string][]string{
					"operation": {"case", "complete"},
					"effects":   {"case", "complete"},
					"lifecycle": {"case", "complete"},
				},
				KeepReferences: true,
			},
			RequiredKinds: []string{"operation", "effects", "lifecycle"},
			KnownKinds:    []string{"operation", "effects", "lifecycle"},
			MaxEvents:     3,
			MaxBytes:      8192,
		},
		CriticalEvidence:   true,
		ProjectionRevision: "raw-synthetic-trace-v1",
		Project: func(_ context.Context, cs evaly.Case[Case, Case], out Output, evidence evaly.EvidenceRecord) (evaly.View[Case, Output, Case], error) {
			return evaly.View[Case, Output, Case]{Case: cs, Output: out, Evidence: evidence}, nil
		},
		Graders: []evaly.Grader[Case, Output, Case]{grader(ds.GraderRevision)},
		Plan: evaly.RunPlan{
			Repeats:         opts.Repeats,
			Concurrency:     1,
			MaxAttempts:     1,
			Timeout:         60 * time.Second,
			CleanupTimeout:  5 * time.Second,
			Seed:            opts.Seed,
			AssertionPolicy: "all",
		},
		Provenance: evaly.Provenance{
			Target:  "real-mcp-offline-v1",
			Tools:   "contracts-22",
			Policy:  "case-tools-effects-v1",
			Unknown: []string{"model", "prompt"},
		},
	}
}

func executeOfflineCase(ctx context.Context, opts Options, cs Case, trial evaly.TrialContext[*environment]) (evaly.TargetResult[Output], error) {
	before := time.Now()
	e := trial.Environment
	args := []string{"--child-case", cs.ID, "--state-dir", e.dir, "--root", opts.Root}
	raw, logs, processErr := Process(ctx, opts.Binary, args, 4<<20)
	output := Output{CaseID: cs.ID, Failure: "missing child output"}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decodeErr := decoder.Decode(&output)
	if decodeErr == nil {
		if trailingErr := decoder.Decode(new(any)); trailingErr != io.EOF {
			decodeErr = errors.New("trailing child output")
		}
	}
	if processErr != nil {
		output.Complete = false
		output.Failure = processErr.Error() + ": " + output.Failure
	} else if decodeErr != nil {
		output.Complete = false
		output.Failure = decodeErr.Error()
	}
	counts, effectErr := readEffects(filepath.Join(e.dir, "effects.jsonl"))
	output.Effects = counts
	if effectErr != nil {
		output.Complete = false
		output.Failure = effectErr.Error()
	}
	output.ElapsedSeconds = time.Since(before).Seconds()
	output.Bytes = len(raw)
	for name, data := range map[string][]byte{"stdout.json": raw, "stderr.log": logs, "effects.json": marshaled(counts), "output.json": marshaled(output)} {
		if writeErr := os.WriteFile(filepath.Join(e.traceDir, name), data, 0600); writeErr != nil {
			return evaly.TargetResult[Output]{Output: output}, writeErr
		}
	}
	for i, kind := range []string{"operation", "effects", "lifecycle"} {
		if recordErr := trial.Evidence.Record(
			ctx,
			evaly.Event{
				Version:    1,
				Sequence:   i + 1,
				Kind:       kind,
				Payload:    marshaled(map[string]any{"case": cs.ID, "complete": output.Complete}),
				References: []string{filepath.Base(e.traceDir) + "/output.json"},
			},
		); recordErr != nil {
			return evaly.TargetResult[Output]{Output: output}, recordErr
		}
	}
	return evaly.TargetResult[Output]{Output: output, Usage: evaly.Usage{Known: false}}, nil
}

func gradeOfflineExperiment(report *Report, opts Options, ds Dataset, record evaly.ExperimentRecord, runErr error) {
	observed := map[string]int{}
	collectTraceMetrics(report, opts.ReportDir)
	for _, trial := range record.Trials {
		observed[trial.CaseID]++
		quality, scored := evaly.AssertionOutcome(trial.Grades, "all")
		healthy := trial.Status == evaly.Completed && trial.Cleanup.State == "completed" && trial.UsageError == "" &&
			evaly.CompleteFor(trial.Evidence, "operation") &&
			evaly.CompleteFor(trial.Evidence, "effects") &&
			evaly.CompleteFor(trial.Evidence, "lifecycle") &&
			len(trial.Grades) == 1 &&
			len(trial.SkippedGraders) == 0
		report.PassedAssertions += passedTrialAssertions(trial.Grades)
		if quality && scored && healthy {
			report.PassedCases++
		} else {
			report.FailedCases = append(report.FailedCases, trial.CaseID)
		}
	}
	for _, cs := range ds.Cases {
		if observed[cs.ID] != opts.Repeats {
			report.SkippedCases = append(report.SkippedCases, cs.ID)
		}
	}
	report.Status = "FAIL"
	report.ExitStatus = 1
	if runErr != nil || len(report.SkippedCases) > 0 || len(record.Trials) != report.ExpectedCases {
		report.Status = "INCOMPLETE"
		report.ExitStatus = 2
	}
	if report.PassedCases == report.ExpectedCases && report.PassedAssertions == report.ExpectedAssertions &&
		len(report.SkippedCases) == 0 &&
		runErr == nil {
		report.Status = "PASS"
		report.ExitStatus = 0
	}
}

func collectTraceMetrics(report *Report, reportDir string) {
	report.Manifest["raw_trace_directory"] = reportDir
	var calls, responseBytes int
	var timeouts []string
	entries, _ := os.ReadDir(reportDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		b, e := os.ReadFile(filepath.Join(reportDir, entry.Name(), "output.json"))
		var output Output
		if e != nil || json.Unmarshal(b, &output) != nil {
			continue
		}
		calls += len(output.Trace)
		for _, trace := range output.Trace {
			responseBytes += len(trace.Result)
		}
		if strings.Contains(output.Failure, "deadline exceeded") {
			timeouts = append(timeouts, output.CaseID)
		}
	}
	report.Manifest["calls"] = calls
	report.Manifest["response_bytes"] = responseBytes
	report.Manifest["timeouts"] = timeouts
}

func offlineManifest(opts Options, ds Dataset) map[string]any {
	info, _ := debug.ReadBuildInfo()
	return map[string]any{
		"profile":          opts.Profile,
		"dataset_revision": ds.Revision,
		"dataset_sha256":   hashFile(opts.DatasetPath),
		"fixture_sha256":   hashFile(filepath.Join(opts.Root, "docs/evals/fixtures.json")),
		"archive_fixture_sha256": hashFile(
			filepath.Join(opts.Root, "internal/mobilebackup/testdata/format1-account-vector.json"),
		),
		"binary_sha256":             hashFile(opts.Binary),
		"go":                        runtime.Version(),
		"os":                        runtime.GOOS,
		"arch":                      runtime.GOARCH,
		"module_build_info":         info,
		"seed":                      opts.Seed,
		"repeats":                   opts.Repeats,
		"concurrency":               1,
		"run_budget_seconds":        opts.RunBudget.Seconds(),
		"case_budget_seconds":       60,
		"stdout_max_bytes":          4 << 20,
		"stderr_max_bytes":          4 << 20,
		"tokens":                    nil,
		"tokens_unavailable_reason": "offline scripted clients expose no model usage",
		"grader_revision":           ds.GraderRevision,
		"model_gate":                "NOT_RUN",
		"live_gate":                 "NOT_RUN",
	}
}

func passedTrialAssertions(grades []evaly.Grade) int {
	passed := 0
	for _, grade := range grades {
		for _, a := range grade.Assertions {
			if a.Pass {
				passed++
			}
		}
	}
	return passed
}

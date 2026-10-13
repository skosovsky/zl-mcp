package performance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

func workerEnv() []string {
	return []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.TempDir(), "TZ=UTC"}
}
func trial(ctx context.Context, root, binary, output string, index int, kind string) Object {
	record := Object{"run": index, "kind": kind, fieldStatus: codeFail, "exit_status": -1}
	state, e := os.MkdirTemp("", "zl-mcp-bench-")
	if e != nil {
		record["error"] = e.Error()
		return record
	}
	defer os.RemoveAll(state)
	command := []string{binary, "--root", root, "--measure-kind", kind, "--measure-calls", "1000", "--state-dir", state}
	record["command"] = command
	ctx, cancel := context.WithTimeout(ctx, measurementTrialBudgetSeconds*time.Second)
	defer cancel()
	out, stderr, code, e := process(ctx, root, workerEnv(), command, commandOutputLimit)
	record["exit_status"] = code
	//nolint:gosec // G306: this synthetic evidence contains build/host provenance, never account credentials.
	if writeErr := os.WriteFile(
		filepath.Join(output, fmt.Sprintf("run-%d-%s.stderr", index, kind)),
		stderr,
		0644,
	); writeErr != nil {
		e = writeErr
	}
	//nolint:gosec // G306: this synthetic evidence contains build/host provenance, never account credentials.
	if writeErr := os.WriteFile(
		filepath.Join(output, fmt.Sprintf("run-%d-%s.stdout", index, kind)),
		out,
		0644,
	); writeErr != nil {
		e = writeErr
	}
	if e == nil {
		var measurement Object
		measurement, e = decodeMeasurement(out, kind)
		if measurement != nil {
			record["measurement"] = measurement
		}
	}
	if e == nil {
		record[fieldStatus] = codePass
	} else {
		record["error"] = e.Error()
	}
	if e = Write(filepath.Join(output, fmt.Sprintf("run-%d-%s.json", index, kind)), record); e != nil {
		record[fieldStatus] = codeFail
		record["error"] = e.Error()
	}
	return record
}
func machine(ctx context.Context, root string) (Object, error) {
	m := Object{}
	for field, flag := range map[string]string{wireSystem: "-s", wireRelease: "-r", wireMachine: "-m", "node": "-n", "version": "-v"} {
		b, e := hostCommand(ctx, root, "uname", flag)
		if e != nil {
			return nil, e
		}
		m[field] = strings.TrimSpace(string(b))
	}
	m["processor"] = runtime.GOARCH
	return m, nil
}
func Bench(ctx context.Context, root, binary, buildPath, output, cpuModel string) error {
	ctx, cancel := context.WithTimeout(ctx, measurementRunBudgetSeconds*time.Second)
	defer cancel()
	var e error
	root, e = filepath.Abs(root)
	if e != nil {
		return e
	}
	binary, e = filepath.Abs(binary)
	if e != nil {
		return e
	}
	buildPath, e = filepath.Abs(buildPath)
	if e != nil {
		return e
	}
	output, e = filepath.Abs(output)
	if e != nil {
		return e
	}
	//nolint:gosec // G301: this directory holds shareable synthetic evidence/executables, never account state.
	if e = os.Mkdir(output, 0755); e != nil {
		return e
	}
	manifest := Object{
		fieldStatus: codeFail,
		"records":   []any{},
		"summary":   Object{},
		wireModel:   codeNotRun,
		wireLive:    codeNotRun,
	}
	started := time.Now()
	resultErr := runMeasurements(ctx, root, binary, buildPath, output, cpuModel, manifest)
	manifest["elapsed_seconds"] = time.Since(started).Seconds()
	if resultErr != nil {
		manifest["error"] = resultErr.Error()
	}
	if e = Write(filepath.Join(output, "manifest.json"), manifest); e != nil {
		return e
	}
	return resultErr
}

func resolveCPUModel(ctx context.Context, root, cpuModel string) string {
	if cpuModel != "" {
		return cpuModel
	}
	if runtime.GOOS == "darwin" {
		b, e := hostCommand(ctx, root, "sysctl", "-n", "machdep.cpu.brand_string")
		if e == nil {
			cpuModel = strings.TrimSpace(string(b))
		}
	} else {
		b, _ := os.ReadFile("/proc/cpuinfo")
		for line := range strings.SplitSeq(string(b), "\n") {
			if strings.HasPrefix(line, "model name") {
				_, cpuModel, _ = strings.Cut(line, ":")
				cpuModel = strings.TrimSpace(cpuModel)
				break
			}
		}
	}
	return cpuModel
}

func summarizeMeasurements(manifest Object) {
	summary := object(manifest["summary"])
	for _, kind := range measurementKinds() {
		vals := map[string][]float64{}
		var calls, errorsCount, response int64
		for _, r := range array(manifest["records"]) {
			row := object(r)
			if row["kind"] != kind {
				continue
			}
			a := object(row["measurement"])
			for _, f := range []string{wireP95Seconds, wireAllocationsPerOp, wireAllocBytesPerOp} {
				vals[f] = append(vals[f], value(a[f]))
			}
			n, _ := integer(a["calls"])
			calls += n
			n, _ = integer(a[wireErrors])
			errorsCount += n
			n, _ = integer(a["bytes"])
			response += n
		}
		summary[kind] = Object{
			"median_of_run_p95_seconds": median(vals[wireP95Seconds]),
			"median_allocations_per_op": median(vals[wireAllocationsPerOp]),
			"median_alloc_bytes_per_op": median(vals[wireAllocBytesPerOp]),
			"measured_calls":            calls,
			wireErrors:                  errorsCount,
			"response_bytes":            response,
		}
	}
}

func runMeasurements(ctx context.Context, root, binary, buildPath, output, cpuModel string, manifest Object) error {
	hash, raw, err := verifiedMeasurementBuild(binary, buildPath)
	if err != nil {
		return err
	}
	var e error
	cpuModel = resolveCPUModel(ctx, root, cpuModel)
	if strings.TrimSpace(cpuModel) == "" || cpuModel == "unavailable" {
		return errors.New("requires verified --cpu-model")
	}
	metadata, e := machine(ctx, root)
	if e != nil {
		return e
	}
	fixture, e := fileDigest(filepath.Join(root, "docs/evals/fixtures.json"))
	if e != nil {
		return e
	}
	maps.Copy(
		manifest,
		Object{
			"binary_sha256":                      hash,
			"build_role":                         wireCandidate,
			"build_manifest_sha256":              digest(raw),
			"build_manifest":                     buildPath,
			wireMachine:                          metadata,
			"cpu_count":                          runtime.NumCPU(),
			"cpu_model":                          cpuModel,
			"cpu_model_source":                   "verified host metadata",
			wireFixtureSHA256:                    fixture,
			wireRuns:                             measurementRuns,
			"calls_per_kind_per_run":             callsPerKindPerRun,
			wireWarmupCalls:                      measurementWarmupCalls,
			wireConcurrencyPerKind:               1,
			wireParallelKinds:                    parallelMeasurementKinds,
			wireAdmissionEnabled:                 true,
			wireExportersEnabled:                 false,
			wirePacingMillis:                     measurementPacingMillis,
			"run_budget_seconds":                 measurementRunBudgetSeconds,
			"trial_budget_seconds":               measurementTrialBudgetSeconds,
			"p95_ratio_threshold":                historicalP95Ratio,
			"allocations_ratio_threshold":        maxAllocationRatio,
			"alloc_bytes_ratio_threshold":        maxAllocBytesRatio,
			"performance_gate_revision":          MeasurementRevision,
			"zero_baseline_allocation_threshold": 0,
		},
	)
	if err := runMeasurementTrials(ctx, root, binary, output, manifest); err != nil {
		return err
	}
	// Decode once to apply identical strict validation to newly generated records.
	rawManifestPath := filepath.Join(output, "manifest.json")
	m, _, e := readObject(rawManifestPath)
	if e != nil {
		return e
	}
	if errs := validate(m, wireCandidate, buildPath); len(errs) > 0 {
		return fmt.Errorf("invalid measurement: %v", errs)
	}
	summarizeMeasurements(manifest)
	manifest[fieldStatus] = codePass
	return nil
}

func verifiedMeasurementBuild(binary, buildPath string) (string, []byte, error) {
	build, raw, e := readObject(buildPath)
	if e != nil {
		return "", nil, e
	}
	hash, e := fileDigest(binary)
	if e != nil {
		return "", nil, e
	}
	matches := 0
	for _, r := range array(build["builds"]) {
		b := object(r)
		if b["binary_sha256"] == hash && b["binary"] == binary && b["kind"] == wireCandidate &&
			b["target"] == wireCmdZlMcpEval &&
			equal(b["exit_status"], 0) {
			matches++
		}
	}
	if build[fieldStatus] != codePass || matches != 1 ||
		!validDigest(object(build["candidate_sources"])["source_digest_sha256"]) {
		return "", nil, errors.New("benchmark requires verified successful current evaluation build")
	}
	return hash, raw, nil
}

func runMeasurementTrials(ctx context.Context, root, binary, output string, manifest Object) error {
	for index := 1; index <= measurementRuns; index++ {
		rows := make([]Object, parallelMeasurementKinds)
		var wg sync.WaitGroup
		for i, kind := range measurementKinds() {
			wg.Add(1)
			go func(i int, kind string) { defer wg.Done(); rows[i] = trial(ctx, root, binary, output, index, kind) }(
				i,
				kind,
			)
		}
		wg.Wait()
		failed := false
		records := array(manifest["records"])
		for _, row := range rows {
			records = append(records, row)
			if row[fieldStatus] != codePass {
				failed = true
			}
		}
		manifest["records"] = records
		if err := Write(filepath.Join(output, "manifest.json"), manifest); err != nil {
			return err
		}
		//nolint:forbidigo // This executable writes the documented CLI result to stdout.
		fmt.Printf(
			"run%d/5: read=%s, search=%s, context=%s\n",
			index,
			rows[0][fieldStatus],
			rows[1][fieldStatus],
			rows[2][fieldStatus],
		)
		if failed || ctx.Err() != nil {
			return errors.New("performance failure or global budget exceeded")
		}
	}
	return nil
}

func decodeMeasurement(out []byte, kind string) (Object, error) {
	if len(out) > 1<<20 || !bytes.Contains(out, []byte{'\n'}) {
		return nil, errors.New("oversized or incomplete JSON response")
	}
	measurement, err := Decode(out)
	if err != nil {
		return nil, err
	}
	if measurement["kind"] != kind || !equal(measurement["calls"], callsPerKindPerRun) ||
		!equal(measurement["warmup"], measurementWarmupCalls) ||
		!equal(measurement[wireErrors], 0) ||
		measurement["cleanup"] != true ||
		!equal(measurement["concurrency"], 1) ||
		!equal(measurement[wirePacingMillis], measurementPacingMillis) ||
		measurement[wireAdmissionEnabled] != true ||
		measurement[wireExportersEnabled] != false {
		return measurement, errors.New("incomplete or failed performance run")
	}
	return measurement, nil
}

package performance

import (
	"bytes"
	"context"
	"fmt"
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
	record := Object{"run": index, "kind": kind, "status": "FAIL", "exit_status": -1}
	state, e := os.MkdirTemp("", "zl-mcp-bench-")
	if e != nil {
		record["error"] = e.Error()
		return record
	}
	defer os.RemoveAll(state)
	command := []string{binary, "--root", root, "--measure-kind", kind, "--measure-calls", "1000", "--state-dir", state}
	record["command"] = command
	ctx, cancel := context.WithTimeout(ctx, 900*time.Second)
	defer cancel()
	out, stderr, code, e := process(ctx, root, workerEnv(), command, 4<<20)
	record["exit_status"] = code
	if writeErr := os.WriteFile(filepath.Join(output, fmt.Sprintf("run-%d-%s.stderr", index, kind)), stderr, 0644); writeErr != nil {
		e = writeErr
	}
	if writeErr := os.WriteFile(filepath.Join(output, fmt.Sprintf("run-%d-%s.stdout", index, kind)), out, 0644); writeErr != nil {
		e = writeErr
	}
	if e == nil {
		if len(out) > 1<<20 || !bytes.Contains(out, []byte{'\n'}) {
			e = fmt.Errorf("oversized or incomplete JSON response")
		} else {
			var measurement Object
			measurement, e = Decode(out)
			if e == nil {
				record["measurement"] = measurement
				if measurement["kind"] != kind || !equal(measurement["calls"], 1000) || !equal(measurement["warmup"], 50) || !equal(measurement["errors"], 0) || measurement["cleanup"] != true || !equal(measurement["concurrency"], 1) || !equal(measurement["pacing_millis"], 110) || measurement["admission_enabled"] != true || measurement["exporters_enabled"] != false {
					e = fmt.Errorf("incomplete or failed performance run")
				}
			}
		}
	}
	if e == nil {
		record["status"] = "PASS"
	} else {
		record["error"] = e.Error()
	}
	if e = Write(filepath.Join(output, fmt.Sprintf("run-%d-%s.json", index, kind)), record); e != nil {
		record["status"] = "FAIL"
		record["error"] = e.Error()
	}
	return record
}
func machine(ctx context.Context, root string) (Object, error) {
	m := Object{}
	for field, flag := range map[string]string{"system": "-s", "release": "-r", "machine": "-m", "node": "-n", "version": "-v"} {
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
	ctx, cancel := context.WithTimeout(ctx, 1200*time.Second)
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
	if e = os.Mkdir(output, 0755); e != nil {
		return e
	}
	manifest := Object{"status": "FAIL", "records": []any{}, "summary": Object{}, "model": "NOT_RUN", "live": "NOT_RUN"}
	started := time.Now()
	resultErr := func() error {
		build, raw, e := readObject(buildPath)
		if e != nil {
			return e
		}
		hash, e := fileDigest(binary)
		if e != nil {
			return e
		}
		matches := 0
		for _, r := range array(build["builds"]) {
			b := object(r)
			if b["binary_sha256"] == hash && b["binary"] == binary && b["kind"] == "candidate" && b["target"] == "./cmd/zl-mcp-eval" && equal(b["exit_status"], 0) {
				matches++
			}
		}
		if build["status"] != "PASS" || matches != 1 || !validDigest(object(build["candidate_sources"])["source_digest_sha256"]) {
			return fmt.Errorf("benchmark requires verified successful current evaluation build")
		}
		if cpuModel == "" {
			if runtime.GOOS == "darwin" {
				b, e := hostCommand(ctx, root, "sysctl", "-n", "machdep.cpu.brand_string")
				if e == nil {
					cpuModel = strings.TrimSpace(string(b))
				}
			} else {
				b, _ := os.ReadFile("/proc/cpuinfo")
				for _, line := range strings.Split(string(b), "\n") {
					if strings.HasPrefix(line, "model name") {
						_, cpuModel, _ = strings.Cut(line, ":")
						cpuModel = strings.TrimSpace(cpuModel)
						break
					}
				}
			}
		}
		if strings.TrimSpace(cpuModel) == "" || cpuModel == "unavailable" {
			return fmt.Errorf("requires verified --cpu-model")
		}
		metadata, e := machine(ctx, root)
		if e != nil {
			return e
		}
		fixture, e := fileDigest(filepath.Join(root, "docs/evals/fixtures.json"))
		if e != nil {
			return e
		}
		for k, v := range (Object{"binary_sha256": hash, "build_role": "candidate", "build_manifest_sha256": digest(raw), "build_manifest": buildPath, "machine": metadata, "cpu_count": runtime.NumCPU(), "cpu_model": cpuModel, "cpu_model_source": "verified host metadata", "fixture_sha256": fixture, "runs": 5, "calls_per_kind_per_run": 1000, "warmup_calls": 50, "concurrency_per_kind": 1, "parallel_kinds": 3, "admission_enabled": true, "exporters_enabled": false, "pacing_millis": 110, "run_budget_seconds": 1200, "trial_budget_seconds": 900, "p95_ratio_threshold": 1.15, "allocations_ratio_threshold": 2.0, "alloc_bytes_ratio_threshold": 1.20, "performance_gate_revision": MeasurementRevision, "zero_baseline_allocation_threshold": 0}) {
			manifest[k] = v
		}
		for index := 1; index <= 5; index++ {
			rows := make([]Object, 3)
			var wg sync.WaitGroup
			for i, kind := range Kinds {
				wg.Add(1)
				go func(i int, kind string) { defer wg.Done(); rows[i] = trial(ctx, root, binary, output, index, kind) }(i, kind)
			}
			wg.Wait()
			failed := false
			records := array(manifest["records"])
			for _, row := range rows {
				records = append(records, row)
				if row["status"] != "PASS" {
					failed = true
				}
			}
			manifest["records"] = records
			if e = Write(filepath.Join(output, "manifest.json"), manifest); e != nil {
				return e
			}
			fmt.Printf("run%d/5: read=%s, search=%s, context=%s\n", index, rows[0]["status"], rows[1]["status"], rows[2]["status"])
			if failed || ctx.Err() != nil {
				return fmt.Errorf("performance failure or global budget exceeded")
			}
		}
		// Decode once to apply identical strict validation to newly generated records.
		rawManifestPath := filepath.Join(output, "manifest.json")
		m, _, e := readObject(rawManifestPath)
		if e != nil {
			return e
		}
		if errs := validate(m, "candidate", buildPath); len(errs) > 0 {
			return fmt.Errorf("invalid measurement: %v", errs)
		}
		summary := object(manifest["summary"])
		for _, kind := range Kinds {
			vals := map[string][]float64{}
			var calls, errorsCount, response int64
			for _, r := range array(manifest["records"]) {
				row := object(r)
				if row["kind"] != kind {
					continue
				}
				a := object(row["measurement"])
				for _, f := range []string{"p95_seconds", "allocations_per_op", "alloc_bytes_per_op"} {
					vals[f] = append(vals[f], value(a[f]))
				}
				n, _ := integer(a["calls"])
				calls += n
				n, _ = integer(a["errors"])
				errorsCount += n
				n, _ = integer(a["bytes"])
				response += n
			}
			summary[kind] = Object{"median_of_run_p95_seconds": median(vals["p95_seconds"]), "median_allocations_per_op": median(vals["allocations_per_op"]), "median_alloc_bytes_per_op": median(vals["alloc_bytes_per_op"]), "measured_calls": calls, "errors": errorsCount, "response_bytes": response}
		}
		manifest["status"] = "PASS"
		return nil
	}()
	manifest["elapsed_seconds"] = time.Since(started).Seconds()
	if resultErr != nil {
		manifest["error"] = resultErr.Error()
	}
	if e = Write(filepath.Join(output, "manifest.json"), manifest); e != nil {
		return e
	}
	return resultErr
}

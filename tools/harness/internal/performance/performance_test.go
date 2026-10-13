package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func repo(t *testing.T) string {
	t.Helper()
	r, e := filepath.Abs("../../../..")
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func fixture(t *testing.T, p string) Object {
	t.Helper()
	m, _, e := readObject(filepath.Join(repo(t), p))
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func clone(t *testing.T, m Object) Object {
	t.Helper()
	b, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	c, e := Decode(b)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func pair(t *testing.T) (Object, Object, string) {
	t.Helper()
	b := fixture(t, "docs/mcp-migration/evidence/T03/performance-baseline/manifest.json")
	b["build_manifest"] = filepath.Join(repo(t), "docs/mcp-migration/evidence/T03/baseline-build-manifest-v2.json")
	c := clone(t, b)
	c["allocations_ratio_threshold"] = 2.0
	c["alloc_bytes_ratio_threshold"] = 1.20
	c["performance_gate_revision"] = MeasurementRevision
	c["binary_sha256"] = strings.Repeat("a", 64)
	c["build_manifest"] = filepath.Join(t.TempDir(), "build.json")
	for _, r := range array(c["records"]) {
		array(object(r)["command"])[0] = "/synthetic/candidate"
	}
	build := Object{"status": "PASS", "toolchain": "go version go1.27.1 darwin/arm64", "candidate_sources": Object{"source_digest_sha256": strings.Repeat("b", 64)}, "builds": []Object{{"kind": "candidate", "target": "./cmd/zl-mcp-eval", "exit_status": 0, "binary": "/synthetic/candidate", "build_info": "go1.27.1", "binary_sha256": c["binary_sha256"]}}}
	if e := Write(str(c["build_manifest"]), build); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(str(c["build_manifest"]))
	if e != nil {
		t.Fatal(e)
	}
	c["build_manifest_sha256"] = digest(raw)
	return b, clone(t, c), str(c["build_manifest"])
}
func requireStatus(t *testing.T, r Object, want string) {
	t.Helper()
	if r["status"] != want {
		t.Fatalf("want %s, got %v", want, r)
	}
}
func TestRawCompletePassAndHistoricalReplay(t *testing.T) {
	b, c, build := pair(t)
	requireStatus(t, Compare(b, c, "", build), "PASS")
	root := repo(t)
	cand := fixture(t, "docs/mcp-migration/evidence/SDK-01/performance-4347-v1/manifest.json")
	old := fixture(t, "docs/mcp-migration/evidence/SDK-01/performance-comparison-4347-v1.json")
	result := Compare(b, cand, filepath.Join(root, "docs/mcp-migration/evidence/T03/baseline-build-manifest-v2.json"), filepath.Join(root, "docs/mcp-migration/evidence/SDK-01/build-manifest-4347-v1.json"))
	requireStatus(t, result, "PASS")
	for _, field := range []string{"summaries", "gates"} {
		a, _ := json.Marshal(result[field])
		z, _ := json.Marshal(old[field])
		if string(a) != string(z) {
			t.Fatalf("historical %s differs: %s / %s", field, a, z)
		}
	}
}
func TestComparatorNegativeCases(t *testing.T) {
	cases := map[string]func(Object, Object){
		"claimed summary": func(b, c Object) {
			c["summary"] = Object{"read": Object{"median_of_run_p95_seconds": 0}}
			for _, r := range array(c["records"]) {
				object(object(r)["measurement"])["p95_seconds"] = 10
			}
		},
		"different machine": func(b, c Object) { c["cpu_model"] = "other" }, "zero objects": func(b, c Object) {
			for _, r := range array(b["records"]) {
				object(object(r)["measurement"])["allocations_per_op"] = 0
			}
		}, "zero bytes": func(b, c Object) {
			for _, r := range array(b["records"]) {
				object(object(r)["measurement"])["alloc_bytes_per_op"] = 0
			}
		},
		"duplicate": func(b, c Object) { array(c["records"])[3] = array(c["records"])[0] }, "missing": func(b, c Object) { c["records"] = array(c["records"])[:14] }, "threshold": func(b, c Object) { c["p95_ratio_threshold"] = 9 }, "revision": func(b, c Object) { c["performance_gate_revision"] = "other" }, "bool zero": func(b, c Object) { c["zero_baseline_allocation_threshold"] = false }, "objects budget": func(b, c Object) { c["allocations_ratio_threshold"] = 3 }, "bytes budget": func(b, c Object) { c["alloc_bytes_ratio_threshold"] = 2 }, "pacing": func(b, c Object) { b["pacing_millis"] = 0; c["pacing_millis"] = 0 }, "admission": func(b, c Object) { b["admission_enabled"] = false; c["admission_enabled"] = false }, "warmup": func(b, c Object) { b["warmup_calls"] = 0; c["warmup_calls"] = 0 }, "masquerade": func(b, c Object) { c["binary_sha256"] = b["binary_sha256"] }, "provenance digest": func(b, c Object) { c["build_manifest_sha256"] = strings.Repeat("c", 64) },
		"exit": func(b, c Object) { object(array(c["records"])[0])["exit_status"] = 7 }, "cleanup": func(b, c Object) { object(object(array(c["records"])[0])["measurement"])["cleanup"] = false }, "run setting": func(b, c Object) { object(object(array(c["records"])[0])["measurement"])["pacing_millis"] = 0 }, "bad executable": func(b, c Object) { array(object(array(c["records"])[0])["command"])[0] = "wrong" }, "mixed runtime": func(b, c Object) { object(object(array(c["records"])[0])["measurement"])["arch"] = "amd64" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			b, c, build := pair(t)
			change(b, c)
			requireStatus(t, Compare(b, c, "", build), "FAIL")
		})
	}
	for _, field := range []string{"cpu_model", "cpu_count", "fixture_sha256"} {
		t.Run("missing "+field, func(t *testing.T) {
			b, c, build := pair(t)
			delete(b, field)
			delete(c, field)
			requireStatus(t, Compare(b, c, "", build), "FAIL")
		})
	}
	for _, field := range []string{"go", "os", "arch"} {
		t.Run("missing measured "+field, func(t *testing.T) {
			b, c, build := pair(t)
			for _, m := range []Object{b, c} {
				for _, r := range array(m["records"]) {
					delete(object(object(r)["measurement"]), field)
				}
			}
			requireStatus(t, Compare(b, c, "", build), "FAIL")
		})
	}
	for _, field := range []string{"p95_seconds", "allocations_per_op", "alloc_bytes_per_op", "elapsed_seconds", "calls", "warmup", "errors", "bytes", "concurrency", "pacing_millis"} {
		for _, v := range []any{-1, true, "0", nil, math.NaN(), math.Inf(1), json.Number("1" + strings.Repeat("0", 1000))} {
			t.Run(fmt.Sprintf("invalid %s %v", field, v), func(t *testing.T) {
				b, c, build := pair(t)
				object(object(array(c["records"])[0])["measurement"])[field] = v
				requireStatus(t, Compare(b, c, "", build), "FAIL")
			})
		}
	}
}
func TestFixedBudgets(t *testing.T) {
	b, c, build := pair(t)
	for _, r := range array(c["records"]) {
		object(object(r)["measurement"])["p95_seconds"] = P95Limits[str(object(r)["kind"])]
	}
	requireStatus(t, Compare(b, c, "", build), "PASS")
	for _, r := range array(c["records"]) {
		m := object(object(r)["measurement"])
		m["p95_seconds"] = value(m["p95_seconds"]) * 1.000001
	}
	requireStatus(t, Compare(b, c, "", build), "FAIL")
	for _, metric := range []string{"allocations_per_op", "alloc_bytes_per_op"} {
		b, c, build = pair(t)
		for _, r := range array(c["records"]) {
			m := object(object(r)["measurement"])
			factor := 2.01
			if metric == "alloc_bytes_per_op" {
				factor = 1.21
			}
			m[metric] = value(m[metric]) * factor
		}
		requireStatus(t, Compare(b, c, "", build), "FAIL")
	}
}
func TestMalformedInputWritesFail(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	report := filepath.Join(dir, "report.json")
	os.WriteFile(bad, []byte("{incomplete"), 0600)
	if e := CompareFiles(bad, bad, "", "", report); e == nil {
		t.Fatal("must fail")
	}
	m, _, e := readObject(report)
	if e != nil {
		t.Fatal(e)
	}
	requireStatus(t, m, "FAIL")
}
func TestDecodePreservesNumberAndRejectsExtra(t *testing.T) {
	m, e := Decode([]byte(`{"id":9007199254740993}`))
	if e != nil || strNumber(m["id"]) != "9007199254740993" {
		t.Fatal(m, e)
	}
	if _, e = Decode([]byte(`{} {}`)); e == nil {
		t.Fatal("accepted trailing object")
	}
}
func strNumber(v any) string { n, _ := v.(json.Number); return string(n) }

var workerOnce sync.Once
var workerPath string
var errWorker error

func worker(t *testing.T, mode string) string {
	t.Helper()
	workerOnce.Do(func() {
		dir, e := os.MkdirTemp("", "zl-go-perf-test-worker-")
		if e != nil {
			errWorker = e
			return
		}
		workerPath = filepath.Join(dir, "worker")
		cmd := exec.Command("go", "build", "-o", workerPath, "./testdata/worker")
		cmd.Env = append(os.Environ(), "GOWORK=off")
		b, e := cmd.CombinedOutput()
		if e != nil {
			errWorker = fmt.Errorf("%w: %s", e, b)
		}
	})
	if errWorker != nil {
		t.Fatal(errWorker)
	}
	path := filepath.Join(t.TempDir(), mode)
	if e := os.Symlink(workerPath, path); e != nil {
		t.Fatal(e)
	}
	return path
}
func TestProcessWatchdogAndBoundedOutput(t *testing.T) {
	for _, mode := range []string{"partial", "flood", "exit"} {
		t.Run(mode, func(t *testing.T) {
			w := worker(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			start := time.Now()
			out, stderr, _, e := process(ctx, "", workerEnv(), []string{w}, 1024)
			if mode != "exit" && e == nil {
				t.Fatal("must fail")
			}
			if len(out)+len(stderr) > 1024 || time.Since(start) > 3*time.Second {
				t.Fatal("unbounded output/deadline", len(out), len(stderr), time.Since(start))
			}
		})
	}
}
func TestNormalCompletionKillsDescendant(t *testing.T) {
	w := worker(t, "descendant")
	if e := os.Symlink(workerPath, w+"-child"); e != nil {
		t.Fatal(e)
	} // worker detects child mode by suffix below
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, _, e := process(ctx, "", workerEnv(), []string{w, "--pidfile", pidFile}, 4096)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(pidFile)
	if e != nil {
		t.Fatal(e)
	}
	pid, e := strconv.Atoi(string(raw))
	if e != nil {
		t.Fatal(e)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		state := strings.TrimSpace(string(b))
		if state == "" || strings.HasPrefix(state, "Z") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("descendant survived owned process group")
}
func TestBenchProtocolWithSyntheticWorker(t *testing.T) {
	root := repo(t)
	w := worker(t, "worker")
	d := t.TempDir()
	build := filepath.Join(d, "build.json")
	hash, e := fileDigest(w)
	if e != nil {
		t.Fatal(e)
	}
	m := Object{"status": "PASS", "toolchain": "go version go1.27.1", "candidate_sources": Object{"source_digest_sha256": strings.Repeat("b", 64)}, "builds": []Object{{"kind": "candidate", "target": "./cmd/zl-mcp-eval", "exit_status": 0, "binary": w, "binary_sha256": hash, "build_info": "go1.27.1"}}}
	if e = Write(build, m); e != nil {
		t.Fatal(e)
	}
	out := filepath.Join(d, "results")
	if e = Bench(context.Background(), root, w, build, out, "synthetic test CPU"); e != nil {
		t.Fatal(e)
	}
	manifest, _, e := readObject(filepath.Join(out, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	requireStatus(t, manifest, "PASS")
	if len(array(manifest["records"])) != 15 {
		t.Fatal("incomplete records")
	}
	for _, kind := range Kinds {
		s := object(object(manifest["summary"])[kind])
		if !equal(s["measured_calls"], 5000) {
			t.Fatal(s)
		}
	}
	// Invalid provenance still produces a FAIL manifest, not an unreviewable panic.
	badOut := filepath.Join(d, "bad-results")
	m["status"] = "FAIL"
	Write(build, m)
	if e = Bench(context.Background(), root, w, build, badOut, "synthetic test CPU"); e == nil {
		t.Fatal("accepted failed build")
	}
	failed, _, _ := readObject(filepath.Join(badOut, "manifest.json"))
	requireStatus(t, failed, "FAIL")
}
func TestBuildProvenanceAndSourceFingerprint(t *testing.T) {
	root := repo(t)
	ctx := context.Background()
	a, e := SourceFingerprint(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	b, e := SourceFingerprint(ctx, root)
	if e != nil || !reflect.DeepEqual(a, b) || !validDigest(a["source_digest_sha256"]) {
		t.Fatal("unstable fingerprint", e)
	}
	for _, raw := range a["files"].([]Object) {
		if strings.HasPrefix(str(raw["path"]), "docs/mcp-migration/evidence/") {
			t.Fatal("historical evidence affects current fingerprint")
		}
	}
}

func TestOversizedJSONNumberWritesFailReport(t *testing.T) {
	b, c, build := pair(t)
	object(object(array(c["records"])[0])["measurement"])["p95_seconds"] = json.Number("1" + strings.Repeat("0", 1000))
	dir := t.TempDir()
	bp := filepath.Join(dir, "baseline.json")
	cp := filepath.Join(dir, "candidate.json")
	report := filepath.Join(dir, "comparison.json")
	if e := Write(bp, b); e != nil {
		t.Fatal(e)
	}
	if e := Write(cp, c); e != nil {
		t.Fatal(e)
	}
	if e := CompareFiles(bp, cp, "", build, report); e == nil {
		t.Fatal("oversized physical measurement accepted")
	}
	m, _, e := readObject(report)
	if e != nil {
		t.Fatal(e)
	}
	requireStatus(t, m, "FAIL")
}
func TestMain(m *testing.M) {
	code := m.Run()
	if workerPath != "" {
		_ = os.RemoveAll(filepath.Dir(workerPath))
	}
	os.Exit(code)
}

func TestHostMetadataCommandWithoutParentDeadlineIsBounded(t *testing.T) {
	// Arrange: an actual executable stands in for a hung uname metadata command.
	uname := worker(t, "uname")
	started := time.Now()
	// Act: exercise the same bounded implementation with a short test budget.
	_, err := boundedHostCommand(context.Background(), 100*time.Millisecond, "", uname, "-s")
	// Assert: context.Background cannot turn host discovery into an infinite wait.
	if err == nil || !strings.Contains(err.Error(), "wall-clock budget") || time.Since(started) > 3*time.Second {
		t.Fatalf("metadata watchdog failed: %v after %s", err, time.Since(started))
	}
}

func TestBenchmarkPreflightMetadataTimeoutRetainsFailManifest(t *testing.T) {
	// Arrange: successful synthetic build, but host uname hangs before workers start.
	root := repo(t)
	w := worker(t, "worker")
	uname := worker(t, "uname")
	t.Setenv("PATH", filepath.Dir(uname)+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	build := filepath.Join(dir, "build.json")
	hash, err := fileDigest(w)
	if err != nil {
		t.Fatal(err)
	}
	provenance := Object{"status": "PASS", "toolchain": "go version go1.27.1", "candidate_sources": Object{"source_digest_sha256": strings.Repeat("b", 64)}, "builds": []Object{{"kind": "candidate", "target": "./cmd/zl-mcp-eval", "exit_status": 0, "binary": w, "binary_sha256": hash, "build_info": "go1.27.1"}}}
	if err = Write(build, provenance); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "results")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	// Act.
	err = Bench(ctx, root, w, build, output, "synthetic test CPU")
	// Assert: overall benchmark deadline covers discovery, and failure evidence survives.
	if err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("preflight deadline failed: %v after %s", err, time.Since(started))
	}
	manifest, _, readErr := readObject(filepath.Join(output, "manifest.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	requireStatus(t, manifest, "FAIL")
	if len(array(manifest["records"])) != 0 || !strings.Contains(str(manifest["error"]), "wall-clock budget") {
		t.Fatal(manifest)
	}
}

package performance

import (
	"fmt"
	"strings"
)

func validate(m Object, side, buildPath string) []string {
	var errs []string
	bad := func(s string) { errs = append(errs, side+" "+s) }
	if m == nil {
		return []string{side + " invalid manifest"}
	}
	if !nonempty(m["cpu_model"]) || m["cpu_model"] == "unavailable" {
		bad("missing/invalid cpu_model")
	}
	if n, ok := integer(m["cpu_count"]); !ok || n <= 0 {
		bad("missing/invalid cpu_count")
	}
	for _, key := range []string{"fixture_sha256", "binary_sha256", "build_manifest_sha256"} {
		if !validDigest(m[key]) {
			bad("missing/invalid " + key)
		}
	}
	for _, key := range []string{"system", "release", "machine"} {
		if !nonempty(object(m["machine"])[key]) {
			bad("missing/invalid machine." + key)
		}
	}
	settings := Object{"runs": 5, "warmup_calls": 50, "concurrency_per_kind": 1, "parallel_kinds": 3, "admission_enabled": true, "exporters_enabled": false, "pacing_millis": 110}
	for key, want := range settings {
		if !equal(m[key], want) {
			bad("invalid protocol " + key)
		}
	}
	calls, ok := integer(m["calls_per_kind_per_run"])
	if !ok || calls != 1000 {
		bad("invalid protocol calls_per_kind_per_run")
	}
	records := array(m["records"])
	if len(records) != 15 {
		bad("incomplete run records")
	}
	for _, raw := range records {
		validateRecord(object(raw), calls, bad)
	}
	validateBuildProvenance(m, side, buildPath, records, bad)
	return errs
}
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if s == x {
			return true
		}
	}
	return false
}
func failure(errs []string) Object {
	return Object{"status": "FAIL", "errors": errs, "summaries": Object{}, "gates": Object{}, "fixed_thresholds": Object{"p95_limits_seconds": P95Limits, "allocations": 2.0, "alloc_bytes": 1.20, "zero_baseline": 0}, "gate_contract_revision": AcceptanceRevision, "model": "NOT_RUN", "live": "NOT_RUN"}
}

// Compare recomputes every median from verified raw records. Reported summaries
// are informational and never inputs to the acceptance decision.
func Compare(baseline, candidate Object, baselineBuild, candidateBuild string) Object {
	errs := append(validate(baseline, "baseline", baselineBuild), validate(candidate, "candidate", candidateBuild)...)
	result := failure(errs)
	if len(errs) > 0 {
		return result
	}
	errs = append(errs, compatibleMeasurements(baseline, candidate)...)
	summaries := Object{}
	for side, m := range map[string]Object{"baseline": baseline, "candidate": candidate} {
		summary, sideErrors := summarizeSide(side, m)
		errs = append(errs, sideErrors...)
		summaries[side] = summary
	}
	// Every worker must agree on build/runtime platform, across all kinds.
	for _, field := range []string{"go", "os", "arch"} {
		want := object(object(array(baseline["records"])[0])["measurement"])[field]
		for _, m := range []Object{baseline, candidate} {
			for _, r := range array(m["records"]) {
				if object(object(r)["measurement"])[field] != want {
					errs = append(errs, "incompatible "+field)
				}
			}
		}
	}
	gates := Object{}
	for _, kind := range Kinds {
		b := object(object(summaries["baseline"])[kind])
		c := object(object(summaries["candidate"])[kind])
		if b == nil || c == nil {
			continue
		}
		p95 := value(c["p95_seconds"]) <= P95Limits[kind]
		objects := value(c["allocations_per_op"]) <= 2*value(b["allocations_per_op"])
		allocBytes := value(c["alloc_bytes_per_op"]) <= 1.20*value(b["alloc_bytes_per_op"])
		gates[kind] = Object{"p95_pass": p95, "allocations_pass": objects, "alloc_bytes_pass": allocBytes, "p95_limit_seconds": P95Limits[kind], "allocations_limit_per_op": 2 * value(b["allocations_per_op"]), "alloc_bytes_limit_per_op": 1.20 * value(b["alloc_bytes_per_op"])}
		if !p95 || !objects || !allocBytes {
			errs = append(errs, kind+" performance threshold exceeded")
		}
	}
	result["summaries"] = summaries
	result["gates"] = gates
	result["errors"] = errs
	if len(errs) == 0 {
		result["status"] = "PASS"
	}
	result["measurement_gate_metadata_revision"] = MeasurementRevision
	result["latency_acceptance_basis"] = "User-authorized retrospective candidate +10% fixed budgets; not original frozen-baseline latency parity"
	return result
}
func CompareFiles(baseline, candidate, baselineBuild, candidateBuild, output string) error {
	b, br, be := readObject(baseline)
	c, cr, ce := readObject(candidate)
	inputs := []Object{}
	if br != nil {
		inputs = append(inputs, Object{"path": baseline, "sha256": digest(br)})
	}
	if cr != nil {
		inputs = append(inputs, Object{"path": candidate, "sha256": digest(cr)})
	}
	var r Object
	if be != nil || ce != nil {
		r = failure([]string{"unreadable/invalid input"})
	} else {
		r = Compare(b, c, baselineBuild, candidateBuild)
	}
	r["inputs"] = inputs
	if e := Write(output, r); e != nil {
		return e
	}
	if r["status"] != "PASS" {
		return fmt.Errorf("performance FAIL: %v", r["errors"])
	}
	return nil
}

func validateRecord(r Object, calls int64, bad func(string)) {
	kind := str(r["kind"])
	index, iok := integer(r["run"])
	if !contains(Kinds, kind) || !iok || index < 1 || index > 5 {
		bad("invalid run record")
		return
	}
	a := object(r["measurement"])
	if a == nil {
		bad("missing measurement")
		return
	}
	if a["kind"] != kind {
		bad("measurement kind mismatch")
	}
	for _, k := range []string{"p95_seconds", "allocations_per_op", "alloc_bytes_per_op", "elapsed_seconds"} {
		if _, ok := number(a[k]); !ok {
			bad("invalid measurement " + k)
		}
	}
	for _, k := range []string{"calls", "warmup", "errors", "bytes", "concurrency", "pacing_millis"} {
		if _, ok := integer(a[k]); !ok {
			bad("invalid measurement " + k)
		}
	}
	n, nok := integer(a["calls"])
	b, bok := integer(a["bytes"])
	if !nok || n != calls || !bok || b <= 0 || value(a["elapsed_seconds"]) <= 0 {
		bad("inconsistent measured counts")
	}
	for _, k := range []string{"go", "os", "arch"} {
		if !nonempty(a[k]) {
			bad("missing/invalid measurement " + k)
		}
	}
	for _, k := range []string{"cleanup", "admission_enabled", "exporters_enabled"} {
		if _, ok := a[k].(bool); !ok {
			bad("invalid measurement " + k)
		}
	}
	if _, ok := integer(r["exit_status"]); !ok {
		bad("invalid exit status")
	}

}

func validateBuildProvenance(m Object, side, buildPath string, records []any, bad func(string)) {
	if buildPath == "" {
		buildPath = str(m["build_manifest"])
	}
	build, raw, e := readObject(buildPath)
	if e != nil {
		bad("invalid build provenance: unreadable/invalid manifest")
		return
	}
	if digest(raw) != m["build_manifest_sha256"] {
		bad("invalid build provenance: build manifest digest mismatch")
	}
	if build["status"] != "PASS" {
		bad("invalid build provenance: failed build manifest")
	}
	var matches []Object
	for _, raw := range array(build["builds"]) {
		b := object(raw)
		target := str(b["target"])
		if target == "" {
			target = "./cmd/zl-mcp-eval"
		}
		if b["binary_sha256"] == m["binary_sha256"] && b["kind"] == side && target == "./cmd/zl-mcp-eval" && equal(b["exit_status"], 0) {
			matches = append(matches, b)
		}
	}
	if len(matches) != 1 {
		bad("invalid build provenance: measured binary role/digest mismatch")
		return
	}
	if side == "candidate" && !validDigest(object(build["candidate_sources"])["source_digest_sha256"]) {
		bad("invalid build provenance: missing candidate source digest")
	}
	match := matches[0]
	for _, raw := range records {
		r := object(raw)
		a := object(r["measurement"])
		goVersion := str(a["go"])
		if goVersion != "go1.27.1" || !strings.Contains(str(build["toolchain"]), goVersion) || !strings.Contains(str(match["build_info"]), goVersion) {
			bad("invalid build provenance: measured Go differs from pinned build toolchain")
		}
		command := array(r["command"])
		if len(command) == 0 || !nonempty(match["binary"]) || command[0] != match["binary"] {
			bad("invalid build provenance: run executable differs from verified build")
		}
	}
}

func summarizeSide(side string, m Object) (Object, []string) {
	var errs []string
	if m["status"] != "PASS" {
		errs = append(errs, side+" incomplete measurement")
	}
	expected := Object{"p95_ratio_threshold": 1.15, "allocations_ratio_threshold": 2.0, "zero_baseline_allocation_threshold": 0}
	revised := side == "candidate" || m["performance_gate_revision"] != nil
	if !revised {
		expected["allocations_ratio_threshold"] = 1.20
	} else {
		expected["alloc_bytes_ratio_threshold"] = 1.20
		if m["performance_gate_revision"] != MeasurementRevision {
			errs = append(errs, side+" missing/changed gate revision")
		}
	}
	for k, want := range expected {
		if _, ok := number(m[k]); !ok || value(m[k]) != value(want) {
			errs = append(errs, side+" changed thresholds")
		}
	}
	summary := Object{}
	records := array(m["records"])
	for _, kind := range Kinds {
		kindSummary, kindErrors := summarizeKind(side, kind, m, records)
		errs = append(errs, kindErrors...)
		if kindSummary != nil {
			summary[kind] = kindSummary
		}
	}
	return summary, errs
}

func summarizeKind(side, kind string, m Object, records []any) (Object, []string) {
	var errs []string
	seen := map[int64]bool{}
	rows := []Object{}
	for _, raw := range records {
		r := object(raw)
		if r["kind"] == kind {
			n, _ := integer(r["run"])
			if seen[n] {
				errs = append(errs, side+" missing/duplicate run "+kind)
			}
			seen[n] = true
			rows = append(rows, r)
		}
	}
	if len(rows) != 5 || len(seen) != 5 {
		errs = append(errs, side+" missing/duplicate run "+kind)
		return nil, errs
	}
	vals := map[string][]float64{}
	var calls, errorsCount, response int64
	for _, r := range rows {
		a := object(r["measurement"])
		if r["status"] != "PASS" || !equal(r["exit_status"], 0) || !equal(a["errors"], 0) || a["cleanup"] != true {
			errs = append(errs, side+" failed/invalid run "+kind)
		}
		for field, outer := range map[string]string{"warmup": "warmup_calls", "pacing_millis": "pacing_millis", "concurrency": "concurrency_per_kind", "admission_enabled": "admission_enabled", "exporters_enabled": "exporters_enabled"} {
			if !equal(a[field], m[outer]) {
				errs = append(errs, side+" mismatched run setting "+field)
			}
		}
		for _, field := range []string{"p95_seconds", "allocations_per_op", "alloc_bytes_per_op"} {
			vals[field] = append(vals[field], value(a[field]))
		}
		n, _ := integer(a["calls"])
		calls += n
		n, _ = integer(a["errors"])
		errorsCount += n
		n, _ = integer(a["bytes"])
		response += n
	}
	return Object{"p95_seconds": median(vals["p95_seconds"]), "allocations_per_op": median(vals["allocations_per_op"]), "alloc_bytes_per_op": median(vals["alloc_bytes_per_op"]), "calls": calls, "errors": errorsCount, "response_bytes": response}, errs
}

func compatibleMeasurements(baseline, candidate Object) []string {
	var errs []string
	for _, k := range []string{"cpu_count", "cpu_model", "fixture_sha256", "runs", "calls_per_kind_per_run", "warmup_calls", "concurrency_per_kind", "parallel_kinds", "admission_enabled", "exporters_enabled", "pacing_millis"} {
		if !equal(baseline[k], candidate[k]) {
			errs = append(errs, "incompatible "+k)
		}
	}
	for _, k := range []string{"system", "release", "machine"} {
		if !equal(object(baseline["machine"])[k], object(candidate["machine"])[k]) {
			errs = append(errs, "incompatible machine."+k)
		}
	}
	return errs
}

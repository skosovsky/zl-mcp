package performance

import (
	"fmt"
	"slices"
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
	for _, key := range []string{wireFixtureSHA256, "binary_sha256", "build_manifest_sha256"} {
		if !validDigest(m[key]) {
			bad("missing/invalid " + key)
		}
	}
	for _, key := range []string{wireSystem, wireRelease, wireMachine} {
		if !nonempty(object(m[wireMachine])[key]) {
			bad("missing/invalid machine." + key)
		}
	}
	settings := Object{
		wireRuns:               measurementRuns,
		wireWarmupCalls:        measurementWarmupCalls,
		wireConcurrencyPerKind: 1,
		wireParallelKinds:      parallelMeasurementKinds,
		wireAdmissionEnabled:   true,
		wireExportersEnabled:   false,
		wirePacingMillis:       measurementPacingMillis,
	}
	for key, want := range settings {
		if !equal(m[key], want) {
			bad("invalid protocol " + key)
		}
	}
	calls, ok := integer(m["calls_per_kind_per_run"])
	if !ok || calls != callsPerKindPerRun {
		bad("invalid protocol calls_per_kind_per_run")
	}
	records := array(m["records"])
	if len(records) != measurementRecords {
		bad("incomplete run records")
	}
	for _, raw := range records {
		validateRecord(object(raw), calls, bad)
	}
	validateBuildProvenance(m, side, buildPath, records, bad)
	return errs
}
func contains(xs []string, s string) bool {
	return slices.Contains(xs, s)
}
func failure(errs []string) Object {
	return Object{
		fieldStatus: codeFail,
		wireErrors:  errs,
		"summaries": Object{},
		"gates":     Object{},
		"fixed_thresholds": Object{
			"p95_limits_seconds": p95Limits(),
			"allocations":        maxAllocationRatio,
			"alloc_bytes":        maxAllocBytesRatio,
			"zero_baseline":      0,
		},
		"gate_contract_revision": AcceptanceRevision,
		wireModel:                codeNotRun,
		wireLive:                 codeNotRun,
	}
}

// Compare recomputes every median from verified raw records. Reported summaries
// are informational and never inputs to the acceptance decision.
func Compare(baseline, candidate Object, baselineBuild, candidateBuild string) Object {
	errs := append(validate(baseline, "baseline", baselineBuild), validate(candidate, wireCandidate, candidateBuild)...)
	result := failure(errs)
	if len(errs) > 0 {
		return result
	}
	errs = append(errs, compatibleMeasurements(baseline, candidate)...)
	summaries := Object{}
	for side, m := range map[string]Object{"baseline": baseline, wireCandidate: candidate} {
		summary, sideErrors := summarizeSide(side, m)
		errs = append(errs, sideErrors...)
		summaries[side] = summary
	}
	// Go is verified against each side's build; runtime platforms still agree across sides.
	for _, field := range []string{"os", "arch"} {
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
	for _, kind := range measurementKinds() {
		b := object(object(summaries["baseline"])[kind])
		c := object(object(summaries[wireCandidate])[kind])
		if b == nil || c == nil {
			continue
		}
		p95 := value(c[wireP95Seconds]) <= p95Limits()[kind]
		objects := value(c[wireAllocationsPerOp]) <= 2*value(b[wireAllocationsPerOp])
		allocBytes := value(c[wireAllocBytesPerOp]) <= maxAllocBytesRatio*value(b[wireAllocBytesPerOp])
		gates[kind] = Object{
			"p95_pass":                 p95,
			"allocations_pass":         objects,
			"alloc_bytes_pass":         allocBytes,
			"p95_limit_seconds":        p95Limits()[kind],
			"allocations_limit_per_op": 2 * value(b[wireAllocationsPerOp]),
			"alloc_bytes_limit_per_op": maxAllocBytesRatio * value(b[wireAllocBytesPerOp]),
		}
		if !p95 || !objects || !allocBytes {
			errs = append(errs, kind+" performance threshold exceeded")
		}
	}
	result["toolchains"] = Object{
		"baseline":    object(object(array(baseline["records"])[0])["measurement"])["go"],
		wireCandidate: object(object(array(candidate["records"])[0])["measurement"])["go"],
	}
	result["summaries"] = summaries
	result["gates"] = gates
	result[wireErrors] = errs
	if len(errs) == 0 {
		result[fieldStatus] = codePass
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
		inputs = append(inputs, Object{wirePath: baseline, wireSHA256: digest(br)})
	}
	if cr != nil {
		inputs = append(inputs, Object{wirePath: candidate, wireSHA256: digest(cr)})
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
	if r[fieldStatus] != codePass {
		return fmt.Errorf("performance FAIL: %v", r[wireErrors])
	}
	return nil
}

func validateRecord(r Object, calls int64, bad func(string)) {
	kind := str(r["kind"])
	index, iok := integer(r["run"])
	if !contains(measurementKinds(), kind) || !iok || index < 1 || index > measurementRuns {
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
	for _, k := range []string{wireP95Seconds, wireAllocationsPerOp, wireAllocBytesPerOp, "elapsed_seconds"} {
		if _, ok := number(a[k]); !ok {
			bad("invalid measurement " + k)
		}
	}
	for _, k := range []string{"calls", "warmup", wireErrors, "bytes", "concurrency", wirePacingMillis} {
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
	for _, k := range []string{"cleanup", wireAdmissionEnabled, wireExportersEnabled} {
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
	if build[fieldStatus] != codePass {
		bad("invalid build provenance: failed build manifest")
	}
	var matches []Object
	for _, raw := range array(build["builds"]) {
		b := object(raw)
		target := str(b["target"])
		if target == "" {
			target = wireCmdZlMcpEval
		}
		if b["binary_sha256"] == m["binary_sha256"] && b["kind"] == side && target == wireCmdZlMcpEval &&
			equal(b["exit_status"], 0) {
			matches = append(matches, b)
		}
	}
	if len(matches) != 1 {
		bad("invalid build provenance: measured binary role/digest mismatch")
		return
	}
	if side == wireCandidate && !validDigest(object(build["candidate_sources"])["source_digest_sha256"]) {
		bad("invalid build provenance: missing candidate source digest")
	}
	match := matches[0]
	for _, raw := range records {
		r := object(raw)
		a := object(r["measurement"])
		goVersion := str(a["go"])
		if !validGoProvenance(side, goVersion, build, match) {
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
	if m[fieldStatus] != codePass {
		errs = append(errs, side+" incomplete measurement")
	}
	expected := Object{
		"p95_ratio_threshold":                historicalP95Ratio,
		"allocations_ratio_threshold":        maxAllocationRatio,
		"zero_baseline_allocation_threshold": 0,
	}
	revised := side == wireCandidate || m["performance_gate_revision"] != nil
	if !revised {
		expected["allocations_ratio_threshold"] = historicalAllocationRatio
	} else {
		expected["alloc_bytes_ratio_threshold"] = maxAllocBytesRatio
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
	for _, kind := range measurementKinds() {
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
	if len(rows) != measurementRuns || len(seen) != measurementRuns {
		errs = append(errs, side+" missing/duplicate run "+kind)
		return nil, errs
	}
	vals := map[string][]float64{}
	var calls, errorsCount, response int64
	for _, r := range rows {
		a := object(r["measurement"])
		if r[fieldStatus] != codePass || !equal(r["exit_status"], 0) || !equal(a[wireErrors], 0) ||
			a["cleanup"] != true {
			errs = append(errs, side+" failed/invalid run "+kind)
		}
		for field, outer := range map[string]string{"warmup": wireWarmupCalls, wirePacingMillis: wirePacingMillis, "concurrency": wireConcurrencyPerKind, wireAdmissionEnabled: wireAdmissionEnabled, wireExportersEnabled: wireExportersEnabled} {
			if !equal(a[field], m[outer]) {
				errs = append(errs, side+" mismatched run setting "+field)
			}
		}
		for _, field := range []string{wireP95Seconds, wireAllocationsPerOp, wireAllocBytesPerOp} {
			vals[field] = append(vals[field], value(a[field]))
		}
		n, _ := integer(a["calls"])
		calls += n
		n, _ = integer(a[wireErrors])
		errorsCount += n
		n, _ = integer(a["bytes"])
		response += n
	}
	return Object{
		wireP95Seconds:       median(vals[wireP95Seconds]),
		wireAllocationsPerOp: median(vals[wireAllocationsPerOp]),
		wireAllocBytesPerOp:  median(vals[wireAllocBytesPerOp]),
		"calls":              calls,
		wireErrors:           errorsCount,
		"response_bytes":     response,
	}, errs
}

func compatibleMeasurements(baseline, candidate Object) []string {
	var errs []string
	for _, k := range []string{"cpu_count", "cpu_model", wireFixtureSHA256, wireRuns, "calls_per_kind_per_run", wireWarmupCalls, wireConcurrencyPerKind, wireParallelKinds, wireAdmissionEnabled, wireExportersEnabled, wirePacingMillis} {
		if !equal(baseline[k], candidate[k]) {
			errs = append(errs, "incompatible "+k)
		}
	}
	for _, k := range []string{wireSystem, wireRelease, wireMachine} {
		if !equal(object(baseline[wireMachine])[k], object(candidate[wireMachine])[k]) {
			errs = append(errs, "incompatible machine."+k)
		}
	}
	return errs
}

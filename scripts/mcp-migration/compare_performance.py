#!/usr/bin/env python3
"""Verify fixed migration performance gates from every independent run."""
import argparse
import hashlib
import json
import math
import re
from pathlib import Path
import statistics

KINDS = ('read', 'search', 'context')
GATE_REVISION = '2026-10-12-objects-and-bytes-v2'
# Measurement metadata remains immutable v2; acceptance is separately revised.
ACCEPTANCE_REVISION = '2026-10-12-candidate-latency-plus10-v3'
P95_LIMITS = {'read': 0.0078683, 'search': 0.0070726337, 'context': 0.0069967799}
THRESHOLDS = {'p95_limits_seconds': P95_LIMITS, 'allocations': 2.0, 'alloc_bytes': 1.20, 'zero_baseline': 0}



def finite_number(value):
    if type(value) not in (int, float) or value < 0:
        return False
    try:
        return math.isfinite(value)
    except OverflowError:
        # JSON integers are unbounded, while physical measurements are finite
        # floating-point values emitted by the Go measurement worker.
        return False


def valid_digest(value):
    return isinstance(value, str) and re.fullmatch(r'[0-9a-f]{64}', value) is not None


def validate(manifest, side, build_path=None):
    errors = []
    if not isinstance(manifest, dict):
        return [side + ' invalid manifest']
    for name in ('cpu_model',):
        if not isinstance(manifest.get(name), str) or not manifest[name].strip() or manifest[name] == 'unavailable':
            errors.append(side + ' missing/invalid ' + name)
    if type(manifest.get('cpu_count')) is not int or manifest['cpu_count'] <= 0:
        errors.append(side + ' missing/invalid cpu_count')
    for name in ('fixture_sha256', 'binary_sha256', 'build_manifest_sha256'):
        if not valid_digest(manifest.get(name)):
            errors.append(side + ' missing/invalid ' + name)
    machine = manifest.get('machine')
    for name in ('system', 'release', 'machine'):
        if not isinstance(machine, dict) or not isinstance(machine.get(name), str) or not machine[name].strip():
            errors.append(side + ' missing/invalid machine.' + name)
    settings = {'runs': 5, 'warmup_calls': 50, 'concurrency_per_kind': 1,
                'parallel_kinds': 3, 'admission_enabled': True,
                'exporters_enabled': False, 'pacing_millis': 110}
    for name, expected in settings.items():
        if type(manifest.get(name)) is not type(expected) or manifest[name] != expected:
            errors.append(side + ' invalid protocol ' + name)
    calls = manifest.get('calls_per_kind_per_run')
    if type(calls) is not int or calls < 1000:
        errors.append(side + ' invalid protocol calls_per_kind_per_run')
    records = manifest.get('records')
    if not isinstance(records, list) or len(records) != 15:
        errors.append(side + ' incomplete run records')
        records = []
    for row in records:
        if not isinstance(row, dict) or row.get('kind') not in KINDS or type(row.get('run')) is not int or row['run'] not in range(1, 6):
            errors.append(side + ' invalid run record')
            continue
        m = row.get('measurement')
        if not isinstance(m, dict):
            errors.append(side + ' missing measurement')
            continue
        if m.get('kind') != row['kind']:
            errors.append(side + ' measurement kind mismatch')
        for field in ('p95_seconds', 'allocations_per_op', 'alloc_bytes_per_op', 'elapsed_seconds'):
            if not finite_number(m.get(field)):
                errors.append(side + ' invalid measurement ' + field)
        for field in ('calls', 'warmup', 'errors', 'bytes', 'concurrency', 'pacing_millis'):
            if type(m.get(field)) is not int or m[field] < 0:
                errors.append(side + ' invalid measurement ' + field)
        if m.get('calls') != calls or not finite_number(m.get('elapsed_seconds')) or m.get('elapsed_seconds', 0) <= 0 or type(m.get('bytes')) is not int or m.get('bytes', 0) <= 0:
            errors.append(side + ' inconsistent measured counts')
        for field in ('go', 'os', 'arch'):
            if not isinstance(m.get(field), str) or not m[field].strip():
                errors.append(side + ' missing/invalid measurement ' + field)
        for field in ('cleanup', 'admission_enabled', 'exporters_enabled'):
            if type(m.get(field)) is not bool:
                errors.append(side + ' invalid measurement ' + field)
        if type(row.get('exit_status')) is not int:
            errors.append(side + ' invalid exit status')
    # Relocation is permitted only for the exact archived build manifest digest.
    try:
        recorded_path = manifest.get('build_manifest')
        if not isinstance(recorded_path, str) or not recorded_path:
            raise ValueError('missing build manifest path')
        raw = Path(build_path or recorded_path).read_bytes()
        build = json.loads(raw)
        if hashlib.sha256(raw).hexdigest() != manifest.get('build_manifest_sha256'):
            raise ValueError('build manifest digest mismatch')
        if build.get('status') != 'PASS':
            raise ValueError('failed build manifest')
        matches = [b for b in build.get('builds', []) if b.get('binary_sha256') == manifest.get('binary_sha256') and b.get('kind') == side and b.get('target', './cmd/zl-mcp-eval') == './cmd/zl-mcp-eval' and b.get('exit_status') == 0]
        if len(matches) != 1:
            raise ValueError('measured binary role/digest mismatch')
        if side == 'candidate' and not valid_digest(build.get('candidate_sources', {}).get('source_digest_sha256')):
            raise ValueError('missing candidate source digest')
        match = matches[0]
        info = match.get('build_info', '')
        for row in records:
            if not isinstance(row, dict):
                continue
            measurement = row.get('measurement', {})
            measured_go = measurement.get('go')
            if measured_go != 'go1.27.1' or measured_go not in build.get('toolchain', '') or measured_go not in info:
                raise ValueError('measured Go differs from pinned build toolchain')
            command = row.get('command')
            if not isinstance(command, list) or not command or command[0] != matches[0].get('binary'):
                raise ValueError('run executable differs from verified build')
    except (OSError, ValueError, TypeError, AttributeError) as error:
        errors.append(side + ' invalid build provenance: ' + str(error))
    return errors

def compare(baseline, candidate, baseline_build=None, candidate_build=None):
    errors = validate(baseline, "baseline", baseline_build) + validate(candidate, "candidate", candidate_build)
    if errors:
        return {"status": "FAIL", "errors": errors, "summaries": {}, "gates": {},
                "fixed_thresholds": dict(THRESHOLDS), "gate_contract_revision": ACCEPTANCE_REVISION}
    for name in ('cpu_count', 'cpu_model', 'fixture_sha256', 'runs',
                 'calls_per_kind_per_run', 'warmup_calls', 'concurrency_per_kind',
                 'parallel_kinds', 'admission_enabled', 'exporters_enabled', 'pacing_millis'):
        if baseline.get(name) != candidate.get(name):
            errors.append('incompatible ' + name)
    for name in ('system', 'release', 'machine'):
        if baseline.get('machine', {}).get(name) != candidate.get('machine', {}).get(name):
            errors.append('incompatible machine.' + name)
    summaries = {}
    for side, manifest in (('baseline', baseline), ('candidate', candidate)):
        if manifest.get('status') != 'PASS' or manifest.get('runs') != 5 or manifest.get('calls_per_kind_per_run', 0) < 1000:
            errors.append(side + ' incomplete measurement')
        expected = {'p95_ratio_threshold': 1.15,
                    'allocations_ratio_threshold': 1.20 if side == 'baseline' and manifest.get('performance_gate_revision') is None else 2.0,
                    'zero_baseline_allocation_threshold': 0}
        if side == 'candidate' or manifest.get('performance_gate_revision') is not None:
            expected['alloc_bytes_ratio_threshold'] = 1.20
        if any(not finite_number(manifest.get(k)) or manifest[k] != v for k, v in expected.items()):
            errors.append(side + ' changed thresholds')
        if (side == 'candidate' or manifest.get('performance_gate_revision') is not None) and manifest.get('performance_gate_revision') != GATE_REVISION:
            errors.append(side + ' missing/changed gate revision')
        if manifest.get('exporters_enabled') is not False:
            errors.append(side + ' exporter enabled')
        summaries[side] = {}
        records = manifest.get('records', [])
        if len(records) != 15:
            errors.append(side + ' incomplete run records')
        for kind in KINDS:
            rows = [r for r in records if r.get('kind') == kind]
            if sorted(r.get('run', 0) for r in rows) != [1, 2, 3, 4, 5]:
                errors.append(side + ' missing/duplicate run ' + kind)
                continue
            measurements = [r.get('measurement', {}) for r in rows]
            for r, m in zip(rows, measurements):
                if r.get('status') != 'PASS' or r.get('exit_status') != 0 or m.get('errors') != 0 or m.get('cleanup') is not True or m.get('calls', 0) < 1000:
                    errors.append(side + ' failed/invalid run ' + kind)
                for key, outer in [('warmup', 'warmup_calls'), ('pacing_millis', 'pacing_millis'), ('concurrency', 'concurrency_per_kind'), ('admission_enabled', 'admission_enabled'), ('exporters_enabled', 'exporters_enabled')]:
                    if m.get(key) != manifest.get(outer):
                        errors.append(side + ' mismatched run setting ' + key)
            summaries[side][kind] = {
                'p95_seconds': statistics.median(m['p95_seconds'] for m in measurements),
                'allocations_per_op': statistics.median(m['allocations_per_op'] for m in measurements),
                'alloc_bytes_per_op': statistics.median(m['alloc_bytes_per_op'] for m in measurements),
                'calls': sum(m['calls'] for m in measurements),
                'errors': sum(m['errors'] for m in measurements),
                'response_bytes': sum(m['bytes'] for m in measurements),
            }
    for kind in KINDS:
        b = [r['measurement'] for r in baseline.get('records', []) if r.get('kind') == kind]
        c = [r['measurement'] for r in candidate.get('records', []) if r.get('kind') == kind]
        if b and c:
            for field in ('go', 'os', 'arch'):
                if {m.get(field) for m in b + c} != {b[0].get(field)}:
                    errors.append('incompatible ' + field)
    gates = {}
    for kind in KINDS:
        if kind not in summaries['baseline'] or kind not in summaries['candidate']:
            continue
        b, c = summaries['baseline'][kind], summaries['candidate'][kind]
        latency = c['p95_seconds'] <= P95_LIMITS[kind]
        allocations = c['allocations_per_op'] <= 2.0 * b['allocations_per_op']
        alloc_bytes = c['alloc_bytes_per_op'] <= 1.20 * b['alloc_bytes_per_op']
        gates[kind] = {'p95_pass': latency, 'allocations_pass': allocations, 'alloc_bytes_pass': alloc_bytes,
                       'p95_limit_seconds': P95_LIMITS[kind],
                       'allocations_limit_per_op': 2.0 * b['allocations_per_op'],
                       'alloc_bytes_limit_per_op': 1.20 * b['alloc_bytes_per_op']}
        if not latency or not allocations or not alloc_bytes:
            errors.append(kind + ' performance threshold exceeded')
    return {'status': 'PASS' if not errors else 'FAIL', 'errors': errors,
            'summaries': summaries, 'gates': gates,
            'fixed_thresholds': dict(THRESHOLDS), 'gate_contract_revision': ACCEPTANCE_REVISION,
            'latency_acceptance_basis': 'User-authorized retrospective candidate +10% fixed budgets; not original frozen-baseline latency parity',
            'measurement_gate_metadata_revision': GATE_REVISION}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--baseline', required=True, type=Path)
    parser.add_argument('--candidate', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--baseline-build', type=Path)
    parser.add_argument('--candidate-build', type=Path)
    args = parser.parse_args()
    inputs = []
    try:
        manifests = []
        for path in (args.baseline, args.candidate):
            raw = path.read_bytes()
            inputs.append({'path': str(path.resolve()), 'sha256': hashlib.sha256(raw).hexdigest()})
            manifests.append(json.loads(raw))
        result = compare(*manifests, args.baseline_build, args.candidate_build)
    except (OSError, ValueError) as error:
        result = {'status': 'FAIL', 'errors': ['unreadable/invalid input: ' + type(error).__name__],
                  'summaries': {}, 'gates': {},
                  'fixed_thresholds': dict(THRESHOLDS), 'gate_contract_revision': ACCEPTANCE_REVISION}
    result['inputs'] = inputs
    args.output.write_text(json.dumps(result, indent=2) + '\n')
    print(result['status'])
    return 0 if result['status'] == 'PASS' else 1


if __name__ == '__main__':
    raise SystemExit(main())

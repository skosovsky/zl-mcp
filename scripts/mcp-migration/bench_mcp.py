#!/usr/bin/env python3
"""Five independent paced real-MCP baseline/candidate benchmark runs."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path
import platform
import signal
import statistics
import subprocess
import tempfile
import time
from subprocess_reader import BoundedJSONReader

ROOT = Path(__file__).resolve().parents[2]
KINDS = ['read', 'search', 'context']

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def trial(binary, output, index, kind, run_deadline):
    record = {'run': index, 'kind': kind, 'status': 'FAIL'}
    with tempfile.TemporaryDirectory(prefix='zl-mcp-bench-') as directory:
        command = [str(binary), '--root', str(ROOT), '--measure-kind', kind,
                   '--measure-calls', '1000', '--state-dir', directory]
        process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   stdin=subprocess.DEVNULL, start_new_session=True,
                                   env={'PATH': os.environ['PATH'], 'TMPDIR': tempfile.gettempdir(), 'TZ': 'UTC'})
        reader = BoundedJSONReader(process.stdout, process.stderr)
        deadline = min(time.monotonic() + 900, run_deadline)
        try:
            value = reader.read(deadline)
            code = process.wait(timeout=max(0.1, deadline - time.monotonic()))
            record.update(command=command, exit_status=code, measurement=value)
            if code != 0 or value['calls'] != 1000 or value['warmup'] != 50 or value['errors'] != 0 or not value['cleanup']:
                raise RuntimeError('incomplete or failed performance run')
            record['status'] = 'PASS'
        except Exception as error:
            record['error'] = str(error)
        finally:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait(timeout=5)
            (output / f'run-{index}-{kind}.stderr').write_bytes(bytes(reader.errors))
            process.stdout.close()
            process.stderr.close()
            (output / f'run-{index}-{kind}.json').write_text(json.dumps(record, indent=2) + '\n')
    return record

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--cpu-model', default='', help='verified host CPU model when sandbox cannot read sysctl')
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--build-manifest', required=True, type=Path)
    args = parser.parse_args()
    build = json.loads(args.build_manifest.read_text())
    digest = sha(args.binary)
    matches = [b for b in build.get('builds', []) if b.get('binary_sha256') == digest
               and b.get('binary') == str(args.binary.resolve())
               and b.get('target', './cmd/zl-mcp-eval') == './cmd/zl-mcp-eval'
               and b.get('kind') in ('baseline', 'candidate') and b.get('exit_status') == 0]
    if build.get('status') != 'PASS' or len(matches) != 1:
        raise ValueError('benchmark requires a verified successful evaluation build')
    role = matches[0]['kind']
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    manifest = {'status': 'FAIL', 'binary_sha256': digest, 'build_role': role,
                'build_manifest_sha256': sha(args.build_manifest), 'build_manifest': str(args.build_manifest),
                'machine': platform.uname()._asdict(), 'cpu_count': os.cpu_count(),
                'cpu_model': args.cpu_model or platform.processor() or 'unavailable',
                'cpu_model_source': 'explicit verified host metadata' if args.cpu_model else 'platform.processor',
                'fixture_sha256': sha(ROOT / 'docs/evals/fixtures.json'),
                'runs': 5, 'calls_per_kind_per_run': 1000, 'warmup_calls': 50,
                'concurrency_per_kind': 1, 'parallel_kinds': 3, 'admission_enabled': True,
                'exporters_enabled': False, 'pacing_millis': 110, 'run_budget_seconds': 1200,
                'trial_budget_seconds': 900, 'p95_ratio_threshold': 1.15,
                'allocations_ratio_threshold': 2.0, 'alloc_bytes_ratio_threshold': 1.20,
                'performance_gate_revision': '2026-10-12-objects-and-bytes-v2', 'zero_baseline_allocation_threshold': 0,
                'records': [], 'summary': {}}
    started = time.monotonic()
    try:
        with ThreadPoolExecutor(max_workers=3) as pool:
            for index in range(1, 6):
                futures = [pool.submit(trial, args.binary.resolve(), output, index, kind, started + 1200) for kind in KINDS]
                records = [future.result() for future in futures]
                manifest['records'].extend(records)
                (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
                print(f'run{index}/5: ' + ', '.join(r['kind'] + '=' + r['status'] for r in records), flush=True)
                if any(r['status'] != 'PASS' for r in records) or time.monotonic() - started > 1200:
                    raise RuntimeError('performance failure or global budget exceeded')
        for kind in KINDS:
            values = [r['measurement'] for r in manifest['records'] if r['kind'] == kind]
            manifest['summary'][kind] = {
                'median_of_run_p95_seconds': statistics.median(v['p95_seconds'] for v in values),
                'median_allocations_per_op': statistics.median(v['allocations_per_op'] for v in values),
                'median_alloc_bytes_per_op': statistics.median(v['alloc_bytes_per_op'] for v in values),
                'measured_calls': sum(v['calls'] for v in values), 'errors': sum(v['errors'] for v in values),
                'response_bytes': sum(v['bytes'] for v in values)}
        manifest['status'] = 'PASS'
    finally:
        manifest['elapsed_seconds'] = time.monotonic() - started
        (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')

if __name__ == '__main__':
    main()

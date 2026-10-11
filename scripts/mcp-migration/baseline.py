#!/usr/bin/env python3
"""Rebuild frozen T01 sources and smoke the real synthetic STDIO binary.
No installed service, account, credentials or production database is accessed.
"""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import select
import signal
import subprocess
import tarfile
import tempfile
import time

BASELINE = '8d913c48b797f54f277ffc1106e6bf28be9e697d'
ROOT = Path(__file__).resolve().parents[2]


class BoundedJSONReader:
    """A partial line cannot block the watchdog; both output streams are bounded."""
    def __init__(self, stdout, stderr, limit=4 << 20):
        self.stdout, self.stderr, self.limit = stdout, stderr, limit
        self.pending = b''
        self.errors = bytearray()
        self.received = 0
        self.streams = [stdout, stderr]
        for stream in self.streams:
            os.set_blocking(stream.fileno(), False)

    def read(self, deadline):
        while True:
            if b'\n' in self.pending:
                line, self.pending = self.pending.split(b'\n', 1)
                return json.loads(line)
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise RuntimeError('STDIO smoke wall-clock budget exceeded')
            ready, _, _ = select.select(self.streams, [], [], remaining)
            if not ready:
                raise RuntimeError('STDIO smoke wall-clock budget exceeded')
            for stream in ready:
                chunk = os.read(stream.fileno(), 65536)
                if not chunk:
                    self.streams.remove(stream)
                    if stream is self.stdout:
                        raise RuntimeError('closed STDIO response')
                    continue
                self.received += len(chunk)
                if self.received > self.limit:
                    raise RuntimeError('STDIO output budget exceeded')
                if stream is self.stdout:
                    self.pending += chunk
                    if len(self.pending) > 1 << 20:
                        raise RuntimeError('oversized STDIO response')
                else:
                    self.errors.extend(chunk)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--measure', action='store_true', help='bounded read/search/context timing smoke; full paired acceptance is T03/T09')
    args = parser.parse_args()
    out = args.output.resolve()
    out.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, GOWORK='off')
    records = []

    def command(argv, cwd, timeout=180):
        started = time.monotonic()
        run = subprocess.run(argv, cwd=cwd, env=env, capture_output=True, timeout=timeout)
        records.append({'command': argv, 'cwd': str(cwd), 'exit_status': run.returncode,
                        'elapsed_seconds': time.monotonic() - started,
                        'stdout': run.stdout.decode(errors='replace'), 'stderr': run.stderr.decode(errors='replace')})
        if run.returncode:
            raise RuntimeError(f'command failed: {argv}')
        return run.stdout

    report = {'baseline_sha': BASELINE, 'model_gate': 'NOT_RUN', 'live_gate': 'NOT_RUN', 'commands': records}
    child = None
    reader = None
    try:
        report['toolchain'] = command(['go', 'version'], ROOT).decode().strip()
        report['tree'] = command(['git', 'rev-parse', BASELINE + '^{tree}'], ROOT).decode().strip()
        archive = command(['git', 'archive', BASELINE], ROOT)
        # Do not persist binary archive bytes as command stdout.
        records[-1]['stdout'] = '<git archive bytes; SHA256 recorded>'
        report['source_archive_sha256'] = hashlib.sha256(archive).hexdigest()
        with tempfile.TemporaryDirectory(prefix='zl-baseline-') as directory:
            source = Path(directory)
            with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
                tar.extractall(source, filter='data')
            for name in ['zl-mcp', 'zl-mcp-eval-fixture']:
                binary = out / name
                command(['go', 'build', '-trimpath', '-o', str(binary), './cmd/' + name], source)
                report[name + '_sha256'] = hashlib.sha256(binary.read_bytes()).hexdigest()
                command(['go', 'version', '-m', str(binary)], source)
            fixture = source / 'docs/evals/fixtures.json'
            report['fixture_sha256'] = hashlib.sha256(fixture.read_bytes()).hexdigest()
            child = subprocess.Popen([str(out / 'zl-mcp-eval-fixture'), '-fixtures', str(fixture)],
                                     stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.PIPE, env=env, start_new_session=True)
            reader = BoundedJSONReader(child.stdout, child.stderr)
            trace = []
            case_deadline = time.monotonic() + 60

            def rpc(value):
                child.stdin.write(json.dumps(value).encode() + b'\n')
                child.stdin.flush()
                if 'id' not in value:
                    return None
                deadline = min(time.monotonic() + 30, case_deadline)
                while True:
                    response = reader.read(deadline)
                    trace.append(response)
                    if response.get('id') == value['id']:
                        if 'error' in response:
                            raise RuntimeError(f'protocol failure: {response}')
                        return response['result']

            rpc({'jsonrpc': '2.0', 'id': 1, 'method': 'initialize', 'params': {'protocolVersion': '2026-07-28', 'capabilities': {}, 'clientInfo': {'name': 'offline-baseline', 'version': '1'}}})
            rpc({'jsonrpc': '2.0', 'method': 'notifications/initialized'})
            meta = {'io.modelcontextprotocol/protocolVersion': '2026-07-28', 'io.modelcontextprotocol/clientCapabilities': {}}
            catalog = rpc({'jsonrpc': '2.0', 'id': 2, 'method': 'tools/list', 'params': {'_meta': meta}})
            frozen = json.loads((ROOT / 'internal/mcpserver/testdata/migration/tools.json').read_text())
            if catalog['tools'] != frozen['tools']:
                raise RuntimeError('frozen binary discovery differs from catalog')
            result = rpc({'jsonrpc': '2.0', 'id': 3, 'method': 'tools/call', 'params': {'_meta': meta, 'name': 'zalo_search_messages', 'arguments': {'query': 'ремонт', 'limit': '10'}}})
            if result.get('isError') or not result.get('structuredContent'):
                raise RuntimeError('synthetic search smoke failed')
            if args.measure:
                measurements = []
                message = json.loads(fixture.read_text())['messages'][0]
                for name, arguments in [
                    ('zalo_list_groups', {'limit': 10}),
                    ('zalo_search_messages', {'query': 'ремонт', 'limit': 10}),
                    ('zalo_get_message_context', {'group_id': message['group_id'], 'message_id': message['message_id'], 'before': 0, 'after': 0}),
                ]:
                    samples, sizes = [], []
                    for i in range(11):
                        time.sleep(0.11)  # Preserve normal admission; warmup + 10 timed requests.
                        started = time.perf_counter_ns()
                        value = rpc({'jsonrpc': '2.0', 'id': 10 + len(measurements) * 11 + i, 'method': 'tools/call', 'params': {'_meta': meta, 'name': name, 'arguments': arguments}})
                        elapsed = time.perf_counter_ns() - started
                        if value.get('isError'):
                            raise RuntimeError('measurement tool error')
                        if i:
                            samples.append(elapsed)
                            sizes.append(len(json.dumps(value, ensure_ascii=False).encode()))
                    measurements.append({'tool': name, 'samples_ns': samples, 'p95_ns': sorted(samples)[-1], 'calls': len(samples), 'errors': 0, 'response_bytes': sizes, 'allocations_per_op': None, 'allocation_reason': 'external STDIO timing smoke; allocation runner is T03', 'concurrency': 1, 'exporters': 'disabled'})
                report['measurement'] = {'status': 'PASS', 'scope': 'timing smoke, not five-run 1000-call performance acceptance', 'runs': 1, 'warmup_calls_per_tool': 1, 'machine': os.uname()._asdict() if hasattr(os.uname(), '_asdict') else list(os.uname()), 'tools': measurements}
            (out / 'smoke.json').write_text(json.dumps(trace, ensure_ascii=False, indent=2) + '\n')
            child.stdin.close()
            child.wait(timeout=10)
            if child.returncode:
                raise RuntimeError(f'fixture exit {child.returncode}')
            report['smoke'] = {'status': 'PASS', 'tool_count': len(catalog['tools']), 'child_exit_status': child.returncode}
        report['status'] = 'PASS'
    except Exception as error:
        report['status'] = 'FAIL'
        report['failure'] = str(error)
        raise
    finally:
        if child is not None and child.poll() is None:
            os.killpg(child.pid, signal.SIGTERM)
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait(timeout=5)
        if reader is not None:
            # Read only a bounded tail after the child has been waited for.
            tail = child.stderr.read(reader.limit - len(reader.errors)) or b''
            (out / 'smoke.stderr').write_bytes(bytes(reader.errors) + tail)
        if child is not None:
            for stream in (child.stdin, child.stdout, child.stderr):
                if stream is not None and not stream.closed:
                    stream.close()
        (out / 'build-manifest.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""Build frozen business baseline with disclosed evaluation-only source overlay."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[2]
BASELINE = '8d913c48b797f54f277ffc1106e6bf28be9e697d'

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, GOWORK='off')
    manifest = {'status': 'FAIL', 'baseline_sha': BASELINE,
                'baseline_tree': subprocess.check_output(['git', 'rev-parse', BASELINE + '^{tree}'], cwd=ROOT, text=True).strip(),
                'candidate_parent_sha': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
                'toolchain': subprocess.check_output(['go', 'version'], env=env, text=True).strip(),
                'original_t01_build_manifest': 'docs/mcp-migration/evidence/T01/build-manifest.json',
                'overlay': [], 'builds': [],
                'candidate_sources': json.loads(subprocess.check_output(['python3', 'scripts/mcp-migration/candidate.py'], cwd=ROOT, text=True))}
    try:
        with tempfile.TemporaryDirectory(prefix='zl-frozen-eval-source-') as directory:
            source = Path(directory)
            archive = subprocess.check_output(['git', 'archive', BASELINE], cwd=ROOT)
            with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
                tar.extractall(source, filter='data')
            overlays = [ROOT / 'go.mod', ROOT / 'go.sum', ROOT / 'internal/service/archive_port.go']
            for package in ['internal/evaluation', 'internal/evalfixture', 'cmd/zl-mcp-eval']:
                overlays.extend(p for p in (ROOT / package).rglob('*.go') if not p.name.endswith('_test.go'))
            for path in sorted(overlays):
                relative = path.relative_to(ROOT)
                target = source / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(path.read_bytes())
                manifest['overlay'].append({'path': str(relative), 'sha256': sha(path)})
            manifest['overlay_purpose'] = 'Only fixture/driver plus forwarding archive constructor and pinned harness dependency/toolchain; frozen business implementations unchanged'
            for name, work in [('baseline', source), ('candidate', ROOT)]:
                binary = output / ('zl-mcp-eval-' + name)
                command = ['go', 'build', '-trimpath', '-o', str(binary), './cmd/zl-mcp-eval']
                result = subprocess.run(command, cwd=work, env=env, capture_output=True, timeout=180)
                (output / (name + '.build.log')).write_bytes(result.stdout + result.stderr)
                record = {'kind': name, 'command': command, 'exit_status': result.returncode, 'binary': str(binary)}
                manifest['builds'].append(record)
                if result.returncode:
                    raise RuntimeError(f'{name} build failed')
                record['binary_sha256'] = sha(binary)
                record['build_info'] = subprocess.check_output(['go', 'version', '-m', str(binary)], text=True)
            manifest['status'] = 'PASS'
    finally:
        (output / 'build-manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')

if __name__ == '__main__':
    main()

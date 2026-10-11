#!/usr/bin/env python3
"""Compile and execute public pinned APIs from an isolated consumer, without replace."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import sys

source = Path(__file__).resolve().parent / 'probe'
output = Path(sys.argv[1])
report = {'status': 'FAIL', 'GOWORK': 'off', 'replace': [], 'commands': []}
with tempfile.TemporaryDirectory(prefix='zl-dependency-probe-') as directory:
    work = Path(directory)
    for name in ['go.mod', 'probe_test.go']:
        (work / name).write_bytes((source / (name + '.txt')).read_bytes())
    try:
        for command in [['go', 'mod', 'tidy'], ['go', 'test', '-v', './...'], ['go', 'list', '-m', '-json', 'all']]:
            result = subprocess.run(command, cwd=work, env=dict(os.environ, GOWORK='off'), capture_output=True, text=True, timeout=180)
            report['commands'].append({'command': command, 'exit_status': result.returncode, 'stdout': result.stdout, 'stderr': result.stderr})
            if result.returncode:
                raise RuntimeError('pinned consumer probe failed')
        report['go_mod'] = (work / 'go.mod').read_text()
        report['go_sum'] = (work / 'go.sum').read_text()
        report['status'] = 'PASS'
    finally:
        output.write_text(json.dumps(report, indent=2) + '\n')
if report['status'] != 'PASS':
    sys.exit(1)

#!/usr/bin/env python3
"""Digest a candidate's sources without self-referential evidence/reports."""
import hashlib
import json
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[2]
paths = subprocess.check_output(['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z'], cwd=root).decode().split('\0')
files = []
for name in sorted(set(paths)):
    if not name or name.startswith('docs/mcp-migration/evidence/') or name == 'docs/mcp-migration/spec-review.md':
        continue
    path = root / name
    if not path.is_file():
        continue
    files.append({'path': name, 'sha256': hashlib.sha256(path.read_bytes()).hexdigest(), 'executable': bool(path.stat().st_mode & 0o111)})
encoded = json.dumps(files, sort_keys=True, separators=(',', ':')).encode()
print(json.dumps({'parent_sha': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip(), 'source_digest_sha256': hashlib.sha256(encoded).hexdigest(), 'algorithm': 'SHA256 canonical sorted path/content SHA256/executable records; excludes evidence and spec-review reports', 'files': files}, indent=2))

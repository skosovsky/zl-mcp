#!/usr/bin/env python3
"""Transitional current-only evaluation build; replaced by Go in H04."""
import argparse, hashlib, json, os, subprocess
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--output', required=True, type=Path)
    args=parser.parse_args(); out=args.output.resolve();out.mkdir(parents=True,exist_ok=False)
    env=dict(os.environ,GOWORK='off')
    files=[]
    for name in sorted(set(subprocess.check_output(['git','ls-files','--cached','--others','--exclude-standard','-z'],cwd=ROOT).decode().split('\0'))):
        path=ROOT/name
        if not name or name.startswith('docs/mcp-migration/evidence/') or name=='docs/mcp-migration/spec-review.md' or not path.is_file():continue
        files.append(dict(path=name,sha256=hashlib.sha256(path.read_bytes()).hexdigest(),executable=bool(path.stat().st_mode&0o111)))
    sources=dict(source_digest_sha256=hashlib.sha256(json.dumps(files,sort_keys=True,separators=(',',':')).encode()).hexdigest(),files=files)
    manifest=dict(status='FAIL',candidate_parent_sha=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),toolchain=subprocess.check_output(['go','version'],text=True).strip(),candidate_sources=sources,builds=[])
    try:
        binary=out/'zl-mcp-eval-candidate';command=['go','build','-trimpath','-o',str(binary),'./cmd/zl-mcp-eval']
        run=subprocess.run(command,cwd=ROOT,env=env,capture_output=True,timeout=180)
        (out/'candidate.build.log').write_bytes(run.stdout+run.stderr)
        record=dict(kind='candidate',target='./cmd/zl-mcp-eval',command=command,exit_status=run.returncode,binary=str(binary));manifest['builds'].append(record)
        if run.returncode:raise RuntimeError('current build failed')
        record.update(binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest(),build_info=subprocess.check_output(['go','version','-m',str(binary)],text=True))
        manifest['status']='PASS'
    finally:(out/'build-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
if __name__=='__main__':main()

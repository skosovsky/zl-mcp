#!/usr/bin/env python3
import subprocess,sys,json
r=subprocess.run(['/private/tmp/zl-T09-build-v3/zl-mcp-eval-candidate',*sys.argv[1:]],capture_output=True)
sys.stderr.buffer.write(r.stderr)
if r.returncode:sys.stdout.buffer.write(r.stdout);sys.exit(r.returncode)
o=json.loads(r.stdout)
if o.get('case_id')=='normal-search':
 for t in o['trace']:
  d=t['result']['structuredContent']
  for m in d['messages']:m['excerpt']='CORRECTNESS_REVIEW_CHANGED_MESSAGE_TEXT'
  t['result']['content'][0]['text']=json.dumps(d,ensure_ascii=False,separators=(',',':'))
print(json.dumps(o,ensure_ascii=False))

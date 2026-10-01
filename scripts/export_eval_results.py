#!/usr/bin/env python3
"""Archive selected final traces and recompute corrected deterministic graders.
Run only after Ann has reviewed the selected scenario answers and traces.
"""
import argparse
from collections import defaultdict
import hashlib
import json
from pathlib import Path
import shutil
from eval_codex import grade


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--runs',nargs='+',type=Path,required=True,help='later runs replace the same task ID from earlier runs')
    parser.add_argument('--output',type=Path,required=True)
    parser.add_argument('--expected-count',type=int,default=59)
    parser.add_argument('--reviewed',action='store_true',required=True,help='selected answers/traces have been reviewed')
    args=parser.parse_args()
    selected={}
    for root in args.runs:
        if not (root/'results.json').exists(): raise SystemExit('run is not authoritatively complete: '+str(root))
        for file in root.glob('*/summary.json'):
            task=json.loads(file.read_text());selected[task['id']]=(file.parent,task)
    if len(selected)!=args.expected_count: raise SystemExit('unexpected case count: '+str(len(selected)))
    accepted=[]
    for task_id,(directory,task) in sorted(selected.items()):
        events=[json.loads(line) for line in (directory/'events.jsonl').read_text().splitlines()]
        calls=[x['item'] for x in events if x.get('type')=='item.completed' and x.get('item',{}).get('type')=='mcp_tool_call']
        answer=(directory/'answer.txt').read_text()
        checks,refs,errors=grade(task,calls,answer,events)
        for key in ['client_exit_ok','no_external_execution','fixture_cleanup','upstream_mutations']:
            checks[key]=task['checks'][key]
        if not all(checks.values()): raise SystemExit('failed '+task_id+': '+','.join(k for k,v in checks.items() if not v))
        record={**task,'original_checks':task['checks'],'checks':checks,'checkpoint_pass':True,'answer_review':'pass','review_scope':'trigger_selection_only' if task['scenario']=='trigger' else 'scenario_answer_and_trace','source_run':directory.parent.name,'references':refs,'tool_error_count':len(errors),'grader_sha256':hashlib.sha256((Path(__file__).parent/'eval_codex.py').read_bytes()).hexdigest()}
        accepted.append((directory,record))
    args.output.mkdir(parents=True,exist_ok=False)
    for directory,record in accepted:
        target=args.output/record['id'];target.mkdir()
        for name in ['events.jsonl','answer.txt','prompt.txt','fixture-stats.json']:
            shutil.copyfile(directory/name,target/name)
        (target/'summary.json').write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n')
    results=[record for _,record in accepted]
    (args.output/'results.json').write_text(json.dumps(results,ensure_ascii=False,indent=2)+'\n')
    manifests={root.name:json.loads((root/'manifest.json').read_text()) for root in args.runs}
    (args.output/'provenance.json').write_text(json.dumps({'synthetic_only':True,'model':'gpt-5.5','runs':manifests,'selection_rule':'latest complete rerun replaces every repetition of its affected scenario; no best-of selection','regrading':'duplicate clarification accepts imperative wording; literal question mark is not required. Original checks preserved. No new model run is claimed for grader recomputation.','reviewer':'Ann','limitations':['Trigger variants score skill selection; full semantic recall of their answers is not scored.','Skills are exposed as fixed MCP resources; native Codex skill installation/autodiscovery was not tested.','Ambiguous-date clarification and invalid-input repair are separate phases; the repair phase injects one invalid argument deliberately.']},ensure_ascii=False,indent=2)+'\n')
    comparisons={}
    for scenario in ['service-recommendation','coverage','adversarial','agreement-history']:
        modes={}
        for skill in [False,True]:
            rows=[x for x in results if x['scenario']==scenario and x['skill']==skill]
            if not rows: continue
            modes['skill' if skill else 'baseline']={'runs':len(rows),'mean_tool_calls':round(sum(x['tool_calls'] for x in rows)/len(rows),2),'mean_input_tokens':round(sum(x['usage']['input_tokens'] for x in rows)/len(rows),2),'mean_output_tokens':round(sum(x['usage']['output_tokens'] for x in rows)/len(rows),2),'mean_answer_bytes':round(sum(x['answer_bytes'] for x in rows)/len(rows),2),'pass':sum(x['checkpoint_pass'] for x in rows)}
        if modes: comparisons[scenario]=modes
    usage={key:sum(x['usage'].get(key,0) for x in results) for key in ['input_tokens','cached_input_tokens','output_tokens','reasoning_output_tokens']}
    metrics={'accepted':len(results),'checkpoint_pass':len(results),'review_pass':len(results),'main_and_followup_phases':sum(x['scenario']!='trigger' for x in results),'trigger_cases':sum(x['scenario']=='trigger' for x in results),'critical_join_and_adversarial_runs':sum(x['scenario'] in ['approved-join','adversarial'] for x in results),'comparisons':comparisons,'accepted_usage':usage,'tool_calls':sum(x['tool_calls'] for x in results),'tool_errors':sum(x['tool_error_count'] for x in results),'intentional_argument_errors':sum(x['scenario']=='invalid-input' for x in results),'other_recovered_tool_errors':sum(x['tool_error_count'] for x in results)-sum(x['scenario']=='invalid-input' for x in results)}
    (args.output/'metrics.json').write_text(json.dumps(metrics,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(metrics,ensure_ascii=False,indent=2))

if __name__=='__main__': main()

#!/usr/bin/env python3
"""Offline CLI failure controls; no model executable is ever authorized or run."""
import argparse
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    dataset = json.loads((ROOT / 'docs/evals/migration-v1.json').read_text())
    dataset['cases'][0]['checks'].append(dict(name='deliberate-failure', step=0,
                                             path='structuredContent.messages.0.message_id',
                                             op='equal', value='impossible'))
    changed_dataset = output / 'failing-dataset.json'
    changed_dataset.write_text(json.dumps(dataset) + '\n')
    marker = output / 'model-was-launched'
    driver = output / 'sentinel-driver'
    driver.write_text("#!/bin/sh\ntouch '" + str(marker) + "'\n")
    driver.chmod(0o700)
    controls = [('assertion-failure', 1, ['--dataset', str(changed_dataset)]),
                ('missing-child', 1, ['--child-binary', str(output / 'missing-binary')]),
                ('global-timeout', 2, ['--run-budget', '1ms']),
                ('model-not-run', 2, ['--profile', 'model']),
                ('model-full-preflight', 2, ['--profile', 'model', '--model-driver', str(driver)]),
                ('unknown-profile', 2, ['--profile', 'typo'])]
    manifest = {'status': 'FAIL', 'records': []}
    try:
        for name, expected, extra in controls:
            # Arrange: isolate reports and use only versioned synthetic inputs.
            report_dir = output / name
            command = [str(args.binary.resolve()), '--root', str(ROOT), '--output', str(report_dir)] + extra
            # Act: the real CLI must preserve a failed/incomplete report.
            result = subprocess.run(command, capture_output=True, timeout=120)
            report = json.loads((report_dir / 'report.json').read_text())
            record = dict(name=name, command=command, exit_status=result.returncode,
                          expected_exit=expected, status=report['status'],
                          stdout=result.stdout.decode(), stderr=result.stderr.decode(),
                          report=str(report_dir / 'report.json'))
            manifest['records'].append(record)
            # Assert: artifact creation and a skipped model never imply acceptance.
            if result.returncode != expected or report['status'] == 'PASS':
                raise RuntimeError(record)
            if name.startswith('model-') and report['model_gate'] != 'NOT_RUN':
                raise RuntimeError('skipped model falsely accepted')
        if marker.exists():
            raise RuntimeError('full model preflight launched sentinel driver')
        manifest['status'] = 'PASS'
    finally:
        (output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')


if __name__ == '__main__':
    main()

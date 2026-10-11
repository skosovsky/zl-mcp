#!/usr/bin/env python3
"""Run pinned arch-go plus production import graph and isolated negative controls."""
import argparse
import json
import os
import re
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
PREFIX = 'github.com/skosovsky/zl-mcp/'


def records(text):
    decoder = json.JSONDecoder()
    pos = 0
    output = []
    while pos < len(text):
        while pos < len(text) and text[pos].isspace():
            pos += 1
        if pos == len(text):
            break
        item, pos = decoder.raw_decode(text, pos)
        output.append(item)
    return output


def within(name, root):
    return name == root or name.startswith(root + '/')


def source_imports(root):
    # Parse every production file regardless of host, cgo or build constraints.
    files = []
    for directory, dirs, names in os.walk(root):
        dirs[:] = [d for d in dirs if not d.startswith('.') and d not in ['vendor', 'testdata']
                   and not (Path(directory) / d / 'go.mod').exists()]
        files.extend(str(Path(directory) / name) for name in names
                     if name.endswith('.go') and not name.endswith('_test.go'))
    files.sort()
    with tempfile.TemporaryDirectory(prefix='zl-import-ast-') as directory:
        helper = Path(directory) / 'imports.go'
        helper.write_bytes((ROOT / 'scripts/mcp-migration/probe/imports.go.txt').read_bytes())
        return json.loads(subprocess.check_output(['go', 'run', str(helper), '--', *files], cwd=root, text=True, timeout=60))


def source_rules(root, files):
    rules = json.loads((root / 'arch-go.yml').read_text())['dependenciesRules']
    violations = []
    for file in files:
        package = PREFIX + str(Path(file['file']).relative_to(root).parent)
        matching = [r for r in rules if re.search(r['package'], package)]
        if not matching:
            violations.append((package, 'uncovered production source package'))
        for dependency in file['imports']:
            kind = 'internal' if dependency.startswith(PREFIX) else 'standard' if '.' not in dependency.split('/')[0] or dependency.startswith('golang.org/x/') else 'external'
            for rule in matching:
                allowed = rule.get('shouldOnlyDependsOn', {}).get(kind, [])
                denied = rule.get('shouldNotDependsOn', {}).get(kind, [])
                if (allowed and not any(re.search(v, dependency) for v in allowed)) or any(re.search(v, dependency) for v in denied):
                    violations.append((file['file'], dependency, 'source dependency rule'))
        relative = str(Path(file['file']).relative_to(root))
        if relative.startswith('internal/service/') and relative != 'internal/service/bridge.go' and any(within(v, 'github.com/modelcontextprotocol/go-sdk') for v in file['imports']):
            violations.append((relative, 'SDK outside existing bridge', 'A08'))
    if violations:
        raise RuntimeError(f'production source violation: {violations}')


def graph_gate():
    packages = records(subprocess.check_output(['go', 'list', '-json', './...'], cwd=ROOT, text=True))
    exceptions = json.loads((ROOT / 'docs/mcp-migration/architecture-exceptions.json').read_text())['exceptions']
    allowed = {(r['package'], r['dependency']) for r in exceptions}
    violations = []
    file_imports = source_imports(ROOT)
    source_rules(ROOT, file_imports)
    for pkg in file_imports:
        name = PREFIX + str(Path(pkg['file']).relative_to(ROOT).parent)
        pkg = {'Imports': pkg['imports']}
        short = name.removeprefix(PREFIX)
        for dependency in pkg.get('Imports', []):
            if short.startswith('internal/app/'):
                capability = short.split('/')[2]
                own = PREFIX + 'internal/app/' + capability
                permitted = dependency == PREFIX + 'internal/domain' or dependency.startswith(PREFIX + 'internal/domain/') or dependency == own or dependency.startswith(own + '/') or dependency == PREFIX + 'internal/app/shared' or dependency.startswith(PREFIX + 'internal/app/shared/')
                infrastructure = dependency in ['database/sql', 'net', 'net/http', 'os', 'os/exec']
                if (dependency.startswith(PREFIX) or '.' in dependency.split('/')[0]) and not permitted or infrastructure:
                    violations.append((name, dependency, 'A02'))
            if dependency.startswith('github.com/skosovsky/toolsy') and not within(short, 'internal/toolruntime'):
                violations.append((name, dependency, 'A04'))
            if dependency.startswith(('github.com/skosovsky/metry', 'go.opentelemetry.io/')) and short != 'internal/service' and not within(short, 'internal/observability'):
                violations.append((name, dependency, 'A06'))
            if dependency.startswith('github.com/skosovsky/evaly') and not within(short, 'internal/evaluation'):
                violations.append((name, dependency, 'A07'))
            if within(short, 'internal/mcpserver') and (any(within(dependency, PREFIX + 'internal/' + v) for v in ['storage', 'collector', 'messaging', 'historyimport', 'zalo', 'service', 'control', 'events']) or dependency == 'database/sql') and (name, dependency) not in allowed:
                violations.append((name, dependency, 'A03'))
            if within(short, 'cmd/zl-mcp') and dependency.startswith(PREFIX) and dependency not in [PREFIX + 'internal/service', PREFIX + 'internal/config'] and (name, dependency) not in allowed:
                violations.append((name, dependency, 'A09'))
            if within(dependency, 'github.com/modelcontextprotocol/go-sdk') and not any(within(short, root) for root in ['internal/mcpserver', 'internal/evalfixture', 'internal/evaluation', 'cmd/zl-mcp-eval-fixture', 'internal/service']):
                violations.append((name, dependency, 'A08'))
    for file in file_imports:
        relative = str(Path(file['file']).relative_to(ROOT))
        if relative.startswith('internal/service/') and relative != 'internal/service/bridge.go' and any(within(v, 'github.com/modelcontextprotocol/go-sdk') for v in file['imports']):
            violations.append((relative, 'SDK outside existing bridge', 'A08'))
    dependencies = subprocess.check_output(['go', 'list', '-deps', './cmd/zl-mcp'], cwd=ROOT, text=True).splitlines()
    for dependency in dependencies:
        if dependency.startswith((PREFIX + 'internal/evaluation', PREFIX + 'internal/evalfixture', 'github.com/skosovsky/evaly')):
            violations.append(('cmd/zl-mcp transitive', dependency, 'A07'))
    if violations:
        raise RuntimeError(f'production graph violation: {violations}')
    return {'status': 'PASS', 'package_count': len(packages), 'production_dependencies': dependencies, 'file_imports': file_imports, 'exceptions': exceptions}


def run(binary, work):
    result = subprocess.run([str(binary), '--json'], cwd=work, capture_output=True, text=True, timeout=60)
    if not (work / '.arch-go/report.json').exists():
        raise RuntimeError(f'arch-go did not produce a report: exit{result.returncode}\n{result.stdout}\n{result.stderr}')
    report = json.loads((work / '.arch-go/report.json').read_text())
    return {'exit_status': result.returncode, 'stdout': result.stdout, 'stderr': result.stderr, 'report': report}


def controls(binary, config):
    tests = []
    cases = [
        ('domain-sql', 'internal/domain', 'database/sql', 'standard', True),
        ('app-sdk', 'internal/app/search', 'github.com/modelcontextprotocol/go-sdk/mcp', 'external', True),
        ('mcp-storage', 'internal/mcpserver', PREFIX + 'internal/storage', 'internal', True),
        ('production-evaluation', 'cmd/zl-mcp', PREFIX + 'internal/evaluation', 'internal', True),
        ('domain-subpackage-sql', 'internal/domain/sub', 'database/sql', 'standard', True),
        ('uncovered-package', 'internal/newboundary', '', 'coverage', True),
        ('root-package-uncovered', '.', '', 'coverage', True),
        ('shadow-package-uncovered', 'internal/foo/zl-mcp/internal/domain', '', 'coverage', True),
        ('app-root-positive', 'internal/app/search', PREFIX + 'internal/domain', 'positive', False),
        ('app-subpackage-positive', 'internal/app/search/sub', PREFIX + 'internal/domain', 'positive', False),
    ]
    for name, package, dependency, kind, should_fail in cases:
        with tempfile.TemporaryDirectory(prefix='zl-arch-control-') as directory:
            work = Path(directory)
            # Normative rule fixture has no legacy implementation and no migration
            # exceptions. The active repository still has exact documented edges.
            fixture_config = json.loads(json.dumps(config))
            for rule in fixture_config['dependenciesRules']:
                if rule['package'] == '^github\\.com/skosovsky/zl\\-mcp/internal/mcpserver(/[\\w.-]+)*$' and 'shouldOnlyDependsOn' in rule:
                    rule['shouldOnlyDependsOn']['internal'] = [v for v in rule['shouldOnlyDependsOn']['internal'] if not any(re.search(v, PREFIX + 'internal/' + name) for name in ['storage', 'control', 'events'])]
                    rule['shouldOnlyDependsOn']['standard'] = [v for v in rule['shouldOnlyDependsOn']['standard'] if not re.search(v, 'database/sql')]
            (work / 'arch-go.yml').write_text(json.dumps(fixture_config))
            (work / 'go.mod').write_text('module github.com/skosovsky/zl-mcp\n\ngo 1.27.1\n')
            target = work / package
            target.mkdir(parents=True, exist_ok=True)
            (target / 'fixture.go').write_text('package fixture\n' + (f'import _ "{dependency}"\n' if dependency else '') + 'func Marker() {}\n')
            if dependency.startswith(PREFIX):
                port = work / dependency.removeprefix(PREFIX)
                port.mkdir(parents=True, exist_ok=True)
                (port / 'fixture.go').write_text('package fixture\nfunc Marker() {}\n')
            result = run(binary, work)
            if should_fail:
                if result['exit_status'] == 0:
                    raise RuntimeError(f'negative control accepted: {name}')
                if kind == 'coverage':
                    if result['report']['coverage']['pass']:
                        raise RuntimeError(f'negative control did not fail coverage: {name}')
                elif dependency not in result['stdout'] or result['report']['compliance']['pass']:
                    raise RuntimeError(f'negative control failed for wrong reason: {name}')
            elif result['exit_status'] != 0 or not result['report']['summary']['pass']:
                raise RuntimeError(f'positive glob control failed: {name}: {result["stdout"]}')
            tests.append({'name': name, 'expected': 'nonzero' if should_fail else 'zero', 'status': 'PASS', **result})
    return tests


def source_controls(config):
    results = []
    cases = [
        ('platform-sdk', 'internal/service/illegal_windows.go', '', 'github.com/modelcontextprotocol/go-sdk/mcp', 'SDK outside existing bridge'),
        ('tagged-sdk', 'internal/service/illegal.go', '//go:build architecture_hidden\n\n', 'github.com/modelcontextprotocol/go-sdk/mcp', 'SDK outside existing bridge'),
        ('cgo-sql', 'internal/domain/illegal.go', '//go:build cgo\n\n', 'database/sql', 'source dependency rule'),
        ('platform-uncovered', 'internal/newboundary/illegal_windows.go', '', '', 'uncovered production source package'),
    ]
    for name, relative, constraint, dependency, reason in cases:
        with tempfile.TemporaryDirectory(prefix='zl-source-control-') as directory:
            work = Path(directory)
            (work / 'go.mod').write_text('module github.com/skosovsky/zl-mcp\n\ngo 1.27.1\n')
            (work / 'arch-go.yml').write_text(json.dumps(config))
            target = work / relative
            target.parent.mkdir(parents=True)
            target.write_text(constraint + 'package fixture\n' + (f'import _ "{dependency}"\n' if dependency else ''))
            try:
                source_rules(work, source_imports(work))
            except RuntimeError as error:
                if reason not in str(error):
                    raise RuntimeError(f'source control failed for wrong reason: {name}: {error}') from error
                results.append({'name': name, 'status': 'PASS', 'expected': reason, 'rejection': str(error)})
            else:
                raise RuntimeError(f'source negative control accepted: {name}')
    return results


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', required=True, type=Path)
    parser.add_argument('--evidence', type=Path)
    args = parser.parse_args()
    output = {'status': 'FAIL'}
    try:
        binary = args.binary.resolve()
        build_info = subprocess.check_output(['go', 'version', '-m', str(binary)], text=True)
        if 'github.com/arch-go/arch-go/v2\tv2.1.2' not in build_info:
            raise RuntimeError('arch-go binary module pin mismatch')
        output['arch_go_build_info'] = build_info
        output['active'] = run(binary, ROOT)
        if output['active']['exit_status'] != 0 or not output['active']['report']['summary']['pass']:
            raise RuntimeError('active architecture rules failed')
        output['production_graph'] = graph_gate()
        output['controls'] = controls(binary, json.loads((ROOT / 'arch-go.yml').read_text()))
        output['source_controls'] = source_controls(json.loads((ROOT / 'arch-go.yml').read_text()))
        output['status'] = 'PASS'
        print('architecture PASS: active compliance/coverage100%, production graph, 10 isolated controls plus all-build source controls')
    finally:
        if args.evidence:
            args.evidence.write_text(json.dumps(output, indent=2) + '\n')


if __name__ == '__main__':
    main()

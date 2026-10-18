#!/usr/bin/env python3
"""Build a deterministic, allowlisted Agent Plugins archive; never read runtime state."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SKILLS = ('researching-zalo-groups', 'handling-zalo-message-events', 'setting-up-zalo')


def encode(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode()


def source(root, relative):
    """Reject symlinks in every path component before any source read."""
    root = root.resolve(strict=True)
    candidate = root
    for component in Path(relative).parts:
        if component in ('..', '/'):
            raise ValueError('source traversal')
        candidate = candidate / component
        if candidate.is_symlink():
            raise ValueError('symlinks are not packaged')
    candidate.resolve(strict=True).relative_to(root)
    return candidate


def files(root=ROOT, instance=None, binary=None, config=None, apps=None):
    """Produce public portable, private Codex binding, or private ChatGPT binding."""
    if instance and not re.fullmatch(r'[a-z][a-z0-9-]{0,31}', instance):
        raise ValueError('instance must be a short lowercase non-personal identifier')
    if bool(binary) != bool(config) or (binary and (not instance or apps)):
        raise ValueError('Codex binding requires instance, binary and config; cannot combine with apps')
    manifest = json.loads(source(root, 'plugin.json').read_text())
    mcp = json.loads(source(root, 'mcp.json').read_text())
    if instance:
        manifest['name'] += '-' + instance
        manifest['extensions']['com.openai']['interface']['displayName'] += ' (' + instance + ')'
        mcp['mcpServers']['zalo-' + instance] = mcp['mcpServers'].pop('zalo')
    if binary:
        for value in [binary, config]:
            if not Path(value).is_absolute() or any(c in value for c in '\r\n\x00'):
                raise ValueError('binary/config must be absolute paths without newlines')
        mcp['mcpServers']['zalo-' + instance]['env'] = {'ZL_MCP_BINARY': binary, 'ZL_MCP_CONFIG': config}
    output = {'plugin.json': encode(manifest), 'mcp.json': encode(mcp),
              'LICENSE': source(root, 'LICENSE').read_bytes(),
              'scripts/bridge.sh': source(root, 'scripts/plugin/bridge.sh').read_bytes()}
    for name in SKILLS:
        for path in sorted(source(root, 'skills/' + name).rglob('*')):
            path = source(root, path.relative_to(root))
            if path.is_symlink():
                raise ValueError('symlinks are not packaged')
            if path.is_file() and path.suffix in ('.md', '.yaml'):
                output[path.relative_to(root).as_posix()] = path.read_bytes()
    if apps:
        if not instance or not all(re.fullmatch(r'asdk_app_[a-zA-Z0-9]+', x) for x in apps.values()):
            raise ValueError('private ChatGPT binding requires instance and verified app IDs')
        manifest['extensions']['com.openai']['apps'] = './.app.json'
        output['plugin.json'] = encode(manifest)
        output['mcp.json'] = encode({'$schema': mcp['$schema'], 'mcpServers': {}})
        output['.app.json'] = encode({'apps': {name: {'id': app, 'required': True} for name, app in apps.items()}})
        del output['scripts/bridge.sh']
    validate(output, root)
    return output


def validate(output, root=ROOT):
    from jsonschema import Draft202012Validator
    import yaml
    for name in ['plugin', 'mcp']:
        schema = json.loads((root / 'scripts/plugin/schemas' / (name + '.schema.json')).read_text())
        Draft202012Validator(schema).validate(json.loads(output[name + '.json']))
    manifest = json.loads(output['plugin.json'])
    extension = manifest['extensions']['com.openai']
    if extension['onboardingSkill'].removeprefix('./') not in output:
        raise ValueError('onboarding skill is missing')
    if len(extension['interface']['shortDescription']) > 30:
        raise ValueError('subtitle exceeds 30 characters')
    for name, data in output.items():
        if name.startswith('/') or '..' in Path(name).parts:
            raise ValueError('archive traversal')
        if name.endswith('/SKILL.md'):
            text = data.decode()
            frontmatter = yaml.safe_load(text.split('---', 2)[1])
            if frontmatter['name'] != Path(name).parent.name or not frontmatter['description'].strip():
                raise ValueError('skill frontmatter mismatch')
            if '[TODO' in text:
                raise ValueError('unfinished skill scaffold')
        if name.endswith('.md'):
            for link in re.findall(r'\]\(([^)]+)\)', data.decode()):
                if '://' in link or link.startswith('#'):
                    continue
                target = os.path.normpath(str(Path(name).parent / link.split('#')[0]))
                if target not in output:
                    raise ValueError('broken packaged reference: ' + name + ' -> ' + link)
    return True


def archive(output, destination):
    # Exclusive creation protects an existing reviewed archive from accidental overwrite.
    with os.fdopen(os.open(destination, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'wb') as stream, \
            zipfile.ZipFile(stream, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for name, data in sorted(output.items()):
            entry = zipfile.ZipInfo(name, date_time=(2026, 1, 1, 0, 0, 0))
            entry.create_system = 3
            mode = 0o755 if name == 'scripts/bridge.sh' else 0o644
            entry.external_attr = (stat.S_IFREG | mode) << 16
            entry.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(entry, data, compresslevel=9)
    return hashlib.sha256(Path(destination).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--check', action='store_true')
    parser.add_argument('--instance')
    parser.add_argument('--binary')
    parser.add_argument('--config')
    parser.add_argument('--app', action='append', default=[], metavar='NAME=VERIFIED_ID')
    args = parser.parse_args()
    apps = dict(x.split('=', 1) for x in args.app)
    output = files(instance=args.instance, binary=args.binary, config=args.config, apps=apps)
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        digest = archive(output, args.output)
        if args.instance:
            args.output.chmod(0o600)
        print(json.dumps({'files': len(output), 'sha256': digest, 'private_binding': bool(args.instance)}))
    elif args.check:
        print(json.dumps({'files': len(output), 'skills': len(SKILLS), 'validated': True}))
    else:
        parser.error('choose --check or --output')


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""Probe an archive through a real, isolated Codex host without a model/API turn.

Only temporary host state is written. Optional binary/config bind the existing
service read-only; this script never starts a collector, logs in, or subscribes.
Evidence deliberately excludes paths, credentials and message contents.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import selectors
import socket
import subprocess
import tempfile
import time
import zipfile

SKILLS = {'researching-zalo-groups', 'handling-zalo-message-events', 'setting-up-zalo'}


class Host:
    def __init__(self, env, cwd):
        self.log = tempfile.TemporaryFile()
        self.process = subprocess.Popen(['codex', 'app-server', '--stdio'], cwd=cwd,
                                        env=env, stdin=subprocess.PIPE,
                                        stdout=subprocess.PIPE, stderr=self.log)
        self.selector = selectors.DefaultSelector()
        self.selector.register(self.process.stdout, selectors.EVENT_READ)
        self.buffer = b''
        self.sequence = 0
        self.rpc('initialize', {'clientInfo': {'name': 'zl-mcp-plugin-probe', 'version': '1'},
                                'capabilities': {'experimentalApi': True}})
        self.send({'method': 'initialized'})

    def send(self, item):
        self.process.stdin.write(json.dumps(item).encode() + b'\n')
        self.process.stdin.flush()

    def rpc(self, method, params, timeout=40):
        self.sequence += 1
        self.send({'id': self.sequence, 'method': method, 'params': params})
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            while b'\n' in self.buffer:
                line, self.buffer = self.buffer.split(b'\n', 1)
                item = json.loads(line)
                if item.get('id') == self.sequence:
                    if 'error' in item:
                        raise RuntimeError(method + ' returned a JSON-RPC error')
                    return item['result']
            if self.selector.select(min(1, max(0, deadline - time.monotonic()))):
                chunk = os.read(self.process.stdout.fileno(), 65536)
                if not chunk:
                    raise RuntimeError('Codex host closed before ' + method)
                self.buffer += chunk
        raise TimeoutError(method)

    def close(self):
        self.process.terminate()
        try:
            self.process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait(timeout=5)
        self.selector.close()
        self.log.close()


def command(args, env, cwd):
    result = subprocess.run(['codex'] + args, cwd=cwd, env=env, text=True,
                            capture_output=True, timeout=40)
    if result.returncode:
        raise RuntimeError('Codex ' + ' '.join(args[:3]) + ' failed')
    return result.stdout


def extract(archive, destination):
    with zipfile.ZipFile(archive) as source:
        for entry in source.infolist():
            name = Path(entry.filename)
            mode = entry.external_attr >> 16
            if name.is_absolute() or '..' in name.parts or mode & 0o170000 == 0o120000:
                raise ValueError('unsafe archive member')
            target = destination / name
            if entry.is_dir():
                target.mkdir(parents=True, exist_ok=True)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(source.read(entry))
                target.chmod(mode & 0o777 or 0o644)


def probe(archive, binary=None, config=None):
    if bool(binary) != bool(config):
        raise ValueError('binary and config must be supplied together')
    if binary:
        binary, config = str(Path(binary).resolve(strict=True)), str(Path(config).resolve(strict=True))
    with tempfile.TemporaryDirectory(prefix='zl-mcp-codex-probe-') as temporary:
        root = Path(temporary)
        home, marketplace = root / 'home', root / 'marketplace'
        home.mkdir(mode=0o700)
        package = marketplace / 'plugins' / 'zalo'
        package.mkdir(parents=True)
        extract(archive, package)
        manifest = json.loads((package / 'plugin.json').read_text())
        name = manifest['name']
        catalog = marketplace / '.agents' / 'plugins' / 'marketplace.json'
        catalog.parent.mkdir(parents=True)
        catalog.write_text(json.dumps({'name': 'zl-mcp-probe', 'plugins': [{
            'name': name, 'source': {'source': 'local', 'path': './plugins/zalo'},
            'policy': {'installation': 'AVAILABLE', 'authentication': 'ON_USE'},
            'category': 'Productivity'}]}))
        env = dict(os.environ, CODEX_HOME=str(home))
        # Prevent inherited host-specific binding or helper settings from steering the probe.
        for key in ('ZL_MCP_BINARY', 'ZL_MCP_CONFIG', 'PLUGIN_ROOT', 'PLUGIN_DATA'):
            env.pop(key, None)
        version = command(['--version'], env, marketplace).strip()
        command(['plugin', 'marketplace', 'add', str(marketplace), '--json'], env, marketplace)
        installed = json.loads(command(['plugin', 'add', name + '@zl-mcp-probe', '--json'], env, marketplace))
        host = Host(env, marketplace)
        try:
            details = host.rpc('plugin/read', {'pluginName': name, 'marketplacePath': str(catalog)})['plugin']
            found = {s['name'].split(':')[-1] for s in details['skills']}
            assert found == SKILLS and all(s['enabled'] for s in details['skills'])
            assert details['onboardingSkill']['name'].endswith(':setting-up-zalo')
            listed = host.rpc('skills/list', {'cwds': [str(marketplace)], 'forceReload': True})
            all_skills = [s for group in listed['data'] for s in group['skills']]
            assert SKILLS <= {s['name'].split(':')[-1] for s in all_skills}
            thread = host.rpc('thread/start', {'cwd': str(marketplace), 'ephemeral': True,
                                              'approvalPolicy': 'never'})['thread']['id']
            inventory = host.rpc('mcpServerStatus/list', {'threadId': thread})
        finally:
            host.close()
        result = {
            'checked_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'archive_sha256': hashlib.sha256(Path(archive).read_bytes()).hexdigest(),
            'codex_version': version, 'plugin_name': name,
            'plugin_version': installed['version'], 'isolated_home': True,
            'marketplace_install': 'PASS', 'portable_manifest_loaded': 'PASS',
            'plugin_read_skills': sorted(found), 'skills_list': 'PASS',
            'onboarding_discovery': 'PASS',
            'onboarding_host_turn': 'NOT_RUN_NO_MODEL_OR_API_TURN',
            'chatgpt_work_host_install': 'NOT_TESTED_BY_CODEX_APP_SERVER',
            'production_installation_changed': False,
        }
        if binary:
            data = list((home / 'plugins' / 'data' / 'agent-plugins').iterdir())
            assert len(data) == 1
            connection = data[0] / 'connection'
            connection.write_text(binary + '\n' + config + '\n')
            connection.chmod(0o600)
            host = Host(env, marketplace)
            try:
                thread = host.rpc('thread/start', {'cwd': str(marketplace), 'ephemeral': True,
                                                  'approvalPolicy': 'never'})['thread']['id']
                inventory = host.rpc('mcpServerStatus/list', {'threadId': thread})
                servers = [x for x in inventory['data'] if x.get('pluginId') == name + '@zl-mcp-probe']
                assert len(servers) == 1 and servers[0]['tools'] and not servers[0].get('toolsError')
                server = servers[0]
                assert 'zalo_get_status' in server['tools']
                status = host.rpc('mcpServer/tool/call', {'threadId': thread, 'server': server['name'],
                                                        'tool': 'zalo_get_status', 'arguments': {}})
                assert not status.get('isError', False)
                payload = status['structuredContent']
                assert isinstance(payload['authenticated'], bool) and isinstance(payload['collector_state'], str)
                result.update({'plugin_data_binding': 'PASS', 'mcp_tool_discovery': 'PASS',
                               'discovered_tool_count': len(server['tools']),
                               'read_only_status_call': 'PASS',
                               'collector_state': payload['collector_state'],
                               'authenticated': payload['authenticated'],
                               'binary_sha256': hashlib.sha256(Path(binary).read_bytes()).hexdigest()})
            finally:
                host.close()
        else:
            result['mcp_tool_discovery'] = 'NOT_BOUND'
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--archive', type=Path, required=True)
    parser.add_argument('--binary')
    parser.add_argument('--config')
    parser.add_argument('--synthetic-service', type=Path,
                        help='start this native binary with fresh temporary state and no credentials')
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    if args.synthetic_service:
        if args.binary or args.config:
            parser.error('synthetic-service cannot be combined with binary/config')
        with tempfile.TemporaryDirectory(prefix='zl-mcp-probe-service-') as temporary:
            root = Path(temporary)
            with socket.socket() as listener:
                listener.bind(('127.0.0.1', 0))
                port = listener.getsockname()[1]
            config = root / 'config.toml'
            config.write_text('state_dir = ' + json.dumps(str(root / 'state')) +
                              '\n[collection]\nmode = "all"\n[logging]\nfile = ' +
                              json.dumps(str(root / 'service.log')) +
                              '\n[mcp]\nlisten = "127.0.0.1:' + str(port) + '"\n')
            config.chmod(0o600)
            binary = args.synthetic_service.resolve(strict=True)
            with tempfile.TemporaryFile() as log:
                service = subprocess.Popen([str(binary), '-config', str(config), 'service'],
                                           stdout=log, stderr=log)
                try:
                    for attempt in range(100):
                        if service.poll() is not None:
                            raise RuntimeError('synthetic service exited during startup')
                        try:
                            with socket.create_connection(('127.0.0.1', port), timeout=0.1):
                                break
                        except OSError:
                            time.sleep(0.05)
                    else:
                        raise TimeoutError('synthetic service listener')
                    result = probe(args.archive, str(binary), str(config))
                    assert result['authenticated'] is False
                    assert result['collector_state'] == 'auth_required'
                    result['service_fixture'] = 'fresh_temporary_native_no_session'
                finally:
                    service.terminate()
                    try:
                        service.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        service.kill()
                        service.wait(timeout=5)
    else:
        result = probe(args.archive, args.binary, args.config)
    rendered = json.dumps(result, indent=2) + '\n'
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered)
    print(rendered, end='')


if __name__ == '__main__':
    main()

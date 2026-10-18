#!/usr/bin/env python3
"""Exercise two account-instance packages and lifecycle operations in isolated Codex.

No model turns, user credentials, production configuration, or ChatGPT plugin
settings are accessed. A supported remove/add sequence performs replacement and
rollback; deleting an installed cache by hand is deliberately not used.
"""
import argparse
import contextlib
import datetime
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import time

import package as builder
from codex_probe import Host, command, extract


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def tree_digest(root):
    result = hashlib.sha256()
    for path in sorted(root.rglob('*')):
        if path.is_file():
            result.update(path.relative_to(root).as_posix().encode() + b'\0' + path.read_bytes())
    return result.hexdigest()


@contextlib.contextmanager
def service(binary, root):
    root.mkdir()
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    config = root / 'config.toml'
    config.write_text('state_dir = ' + json.dumps(str(root / 'state')) +
                      '\n[collection]\nmode = "all"\n[logging]\nfile = ' +
                      json.dumps(str(root / 'service.log')) +
                      '\n[mcp]\nlisten = "127.0.0.1:' + str(port) + '"\n')
    config.chmod(0o600)
    with tempfile.TemporaryFile() as log:
        process = subprocess.Popen([str(binary), '-config', str(config), 'service'],
                                   stdout=log, stderr=log)
        try:
            for attempt in range(100):
                if process.poll() is not None:
                    raise RuntimeError('synthetic service exited')
                try:
                    with socket.create_connection(('127.0.0.1', port), timeout=0.1):
                        break
                except OSError:
                    time.sleep(0.05)
            else:
                raise TimeoutError('synthetic service listener')
            yield config, process
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)


def write_package(binary, config, instance, version, destination, archive):
    files = builder.files(instance=instance, binary=str(binary), config=str(config))
    manifest = json.loads(files['plugin.json'])
    manifest['version'] = version
    files['plugin.json'] = builder.encode(manifest)
    builder.validate(files)
    archive_hash = builder.archive(files, archive)
    # Replace the source package, not the host's installed cache.
    if destination.exists():
        import shutil
        shutil.rmtree(destination)
    destination.mkdir(parents=True)
    extract(archive, destination)
    return manifest['name'], archive_hash


def inspect(env, marketplace, catalog, expected):
    """Start the real host, read both packages, and call each scoped server."""
    host = Host(env, marketplace)
    try:
        for name, version in expected.items():
            plugin = host.rpc('plugin/read', {'pluginName': name,
                                              'marketplacePath': str(catalog)})['plugin']
            assert plugin['summary']['installed'] and plugin['summary']['enabled']
            assert plugin['summary']['localVersion'] == version
            assert len(plugin['skills']) == 3
        thread = host.rpc('thread/start', {'cwd': str(marketplace), 'ephemeral': True,
                                          'approvalPolicy': 'never'})['thread']['id']
        inventory = host.rpc('mcpServerStatus/list', {'threadId': thread})['data']
        scoped = {entry['pluginId']: entry for entry in inventory if entry.get('pluginId')}
        wanted = {name + '@zl-lifecycle' for name in expected}
        assert set(scoped) == wanted, 'scoped servers: ' + repr(sorted(scoped)) + '; expected: ' + repr(sorted(wanted))
        assert len({entry['name'] for entry in scoped.values()}) == len(wanted)
        counts = {}
        for name in expected:
            entry = scoped[name + '@zl-lifecycle']
            assert not entry.get('toolsError') and 'zalo_get_status' in entry['tools']
            status = host.rpc('mcpServer/tool/call', {'threadId': thread,
                                                     'server': entry['name'],
                                                     'tool': 'zalo_get_status', 'arguments': {}})
            assert not status.get('isError', False)
            assert status['structuredContent']['authenticated'] is False
            assert status['structuredContent']['collector_state'] == 'auth_required'
            counts[name] = len(entry['tools'])
        return counts
    finally:
        host.close()


def run(binary):
    binary = binary.resolve(strict=True)
    # Help is checked on the executable actually used for all operations.
    subprocess.run(['codex', 'plugin', 'remove', '--help'], check=True,
                   capture_output=True, timeout=20)
    subprocess.run(['codex', 'plugin', 'add', '--help'], check=True,
                   capture_output=True, timeout=20)
    with tempfile.TemporaryDirectory(prefix='zl-codex-lifecycle-') as temporary:
        root = Path(temporary)
        home, marketplace = root / 'home', root / 'marketplace'
        home.mkdir(mode=0o700)
        marketplace.mkdir()
        env = dict(os.environ, CODEX_HOME=str(home))
        for key in ('ZL_MCP_BINARY', 'ZL_MCP_CONFIG', 'PLUGIN_ROOT', 'PLUGIN_DATA'):
            env.pop(key, None)
        version = command(['--version'], env, marketplace).strip()
        with service(binary, root / 'a') as (config_a, process_a), \
                service(binary, root / 'b') as (config_b, process_b):
            name_a, archive_a = write_package(binary, config_a, 'account-a', '0.1.0',
                                             marketplace / 'plugins/a', root / 'a-v1.zip')
            name_b, archive_b = write_package(binary, config_b, 'account-b', '0.1.0',
                                             marketplace / 'plugins/b', root / 'b-v1.zip')
            catalog = marketplace / '.agents/plugins/marketplace.json'
            catalog.parent.mkdir(parents=True)
            catalog.write_text(json.dumps({'name': 'zl-lifecycle', 'plugins': [
                {'name': name, 'source': {'source': 'local', 'path': './plugins/' + suffix},
                 'policy': {'installation': 'AVAILABLE', 'authentication': 'ON_USE'},
                 'category': 'Productivity'} for name, suffix in [(name_a, 'a'), (name_b, 'b')]]}))
            command(['plugin', 'marketplace', 'add', str(marketplace), '--json'], env, marketplace)
            for name in (name_a, name_b):
                command(['plugin', 'add', name + '@zl-lifecycle', '--json'], env, marketplace)
            expected = {name_a: '0.1.0', name_b: '0.1.0'}
            counts = inspect(env, marketplace, catalog, expected)
            b_cache = home / 'plugins/cache/zl-lifecycle' / name_b
            unchanged = (tree_digest(b_cache), digest(config_b),
                         digest(config_b.parent / 'state/mcp-token'), process_b.pid)

            def check_b():
                assert process_b.poll() is None
                assert (tree_digest(b_cache), digest(config_b),
                        digest(config_b.parent / 'state/mcp-token'), process_b.pid) == unchanged

            # Official remove/add is an explicit replacement, with both service
            # processes and state retained throughout. No hidden cache editing.
            command(['plugin', 'remove', name_a + '@zl-lifecycle', '--json'], env, marketplace)
            _, archive_upgrade = write_package(binary, config_a, 'account-a', '0.1.1',
                                                marketplace / 'plugins/a', root / 'a-v2.zip')
            installed = json.loads(command(['plugin', 'add', name_a + '@zl-lifecycle', '--json'],
                                           env, marketplace))
            assert installed['version'] == '0.1.1'
            expected[name_a] = '0.1.1'
            inspect(env, marketplace, catalog, expected)
            check_b()

            command(['plugin', 'remove', name_a + '@zl-lifecycle', '--json'], env, marketplace)
            # Restore the exact prior reviewed bytes, not a new guessed version.
            import shutil
            shutil.rmtree(marketplace / 'plugins/a')
            extract(root / 'a-v1.zip', marketplace / 'plugins/a')
            rolled_back = json.loads(command(['plugin', 'add', name_a + '@zl-lifecycle', '--json'],
                                             env, marketplace))
            assert rolled_back['version'] == '0.1.0'
            expected[name_a] = '0.1.0'
            inspect(env, marketplace, catalog, expected)
            check_b()
            command(['plugin', 'remove', name_a + '@zl-lifecycle', '--json'], env, marketplace)
            inspect(env, marketplace, catalog, {name_b: '0.1.0'})
            check_b()
            assert process_a.poll() is None
            public = builder.files()
            public_hash = builder.archive(public, root / 'public.zip')
            return {
                'checked_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                'codex_version': version, 'native_binary_sha256': digest(binary),
                'probe_script_sha256': digest(__file__),
                'builder_script_sha256': digest(builder.__file__),
                'public_archive_sha256': public_hash,
                'private_fixture_archive_sha256': {'a_0.1.0': archive_a, 'b_0.1.0': archive_b,
                                                  'a_0.1.1': archive_upgrade},
                'source_input_tree_sha256': hashlib.sha256(b''.join(
                    path.encode() + b'\0' + data for path, data in sorted(public.items()))).hexdigest(),
                'two_real_native_services_without_session': 'PASS',
                'two_scoped_mcp_servers_and_status_calls': 'PASS',
                'discovered_tool_counts': counts,
                'upgrade_a_0.1.0_to_0.1.1': 'PASS',
                'rollback_a_0.1.1_to_0.1.0': 'PASS',
                'replacement_flow': 'codex plugin remove then add from selected source artifact',
                'remove_a_b_still_callable': 'PASS',
                'b_cache_config_token_and_service_pid_unchanged': 'PASS',
                'removing_a_did_not_stop_either_service': 'PASS',
                'production_installation_changed': False,
                'chatgpt_plugin_changed': False,
                'model_api_turns': 0,
                'limitation': 'Codex lifecycle only; no ChatGPT installation or onboarding turn proof',
            }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    result = run(args.binary)
    text = json.dumps(result, indent=2) + '\n'
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(text)
    print(text, end='')


if __name__ == '__main__':
    main()

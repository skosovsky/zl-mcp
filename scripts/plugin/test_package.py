"""Behavioral negative controls for distributable package boundaries (AAA)."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile

SPEC = importlib.util.spec_from_file_location('package', Path(__file__).with_name('package.py'))
PACKAGE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PACKAGE)


class PackageTests(unittest.TestCase):
    def test_reproducible_allowlisted_archive(self):
        # Arrange
        output = PACKAGE.files()
        with tempfile.TemporaryDirectory() as temp:
            a, b = Path(temp) / 'a.zip', Path(temp) / 'b.zip'
            # Act
            digest = PACKAGE.archive(output, a)
            repeated = PACKAGE.archive(output, b)
            # Assert
            self.assertEqual(digest, repeated)
            with zipfile.ZipFile(a) as z:
                self.assertEqual(set(z.namelist()), set(output))
                self.assertEqual((z.getinfo('scripts/bridge.sh').external_attr >> 16) & 0o777, 0o755)
            self.assertNotIn('.app.json', output)
            self.assertNotIn('config.toml', output)

    def test_invalid_binding_and_missing_reference_fail_closed(self):
        # Arrange / Act / Assert
        for args in [{'instance': '../other'}, {'binary': '/bin/true'},
                     {'instance': 'a', 'binary': '/bin/true', 'config': 'relative'},
                     {'instance': 'a', 'binary': '/bin/true\nother', 'config': '/config'}]:
            with self.assertRaises(ValueError):
                PACKAGE.files(**args)
        output = PACKAGE.files()
        del output['skills/setting-up-zalo/references/recovery.md']
        with self.assertRaisesRegex(ValueError, 'broken packaged reference'):
            PACKAGE.validate(output)

    def test_root_and_parent_symlinks_are_rejected(self):
        # Arrange: the contents are harmless; the boundary itself must be enforced.
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            external = root / 'external'
            external.mkdir()
            (root / 'skills').symlink_to(external, target_is_directory=True)
            (root / 'plugin.json').symlink_to(PACKAGE.ROOT / 'plugin.json')
            # Act / Assert
            with self.assertRaisesRegex(ValueError, 'symlinks'):
                PACKAGE.files(root=root)
            with self.assertRaisesRegex(ValueError, 'symlinks'):
                PACKAGE.source(root, 'skills/setting-up-zalo')

    def test_codex_binding_has_only_explicit_paths(self):
        # Arrange / Act
        output = PACKAGE.files(instance='work', binary='/opt/local/bin/zl-mcp', config='/opt/private/work.toml')
        # Assert
        descriptor = json.loads(output['mcp.json'])['mcpServers']['zalo-work']
        self.assertEqual(descriptor['command'], './scripts/bridge.sh')
        self.assertEqual(descriptor['env'], {'ZL_MCP_BINARY': '/opt/local/bin/zl-mcp',
                                             'ZL_MCP_CONFIG': '/opt/private/work.toml'})

    def test_portable_data_bindings_have_disjoint_server_keys(self):
        # Arrange: two private package names use host PLUGIN_DATA, without env bindings.
        a = PACKAGE.files(instance='personal')
        b = PACKAGE.files(instance='work')
        # Act
        servers_a = json.loads(a['mcp.json'])['mcpServers']
        servers_b = json.loads(b['mcp.json'])['mcpServers']
        # Assert: Codex must not suppress the neighbor through a duplicate server key.
        self.assertTrue(set(servers_a).isdisjoint(servers_b))
        self.assertNotIn('env', next(iter(servers_a.values())))

    def test_bridge_is_literal_and_never_starts_service(self):
        # Arrange
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            binary = root / 'fake Go binary'
            binary.write_text('#!/bin/sh\nprintf "%s\\n" "$@"\n')
            binary.chmod(0o755)
            config = root / 'config $(false).toml'
            config.touch()
            (root / 'connection').write_text(f'{binary}\n{config}\n')
            # Act
            result = subprocess.run([str(PACKAGE.ROOT / 'scripts/plugin/bridge.sh')],
                                    env={'PLUGIN_DATA': str(root)}, text=True, capture_output=True)
            # Assert
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stdout.splitlines(), ['-config', str(config), 'serve'])
            self.assertNotIn('service', result.stdout)
            (root / 'connection').write_text(f'{binary}\n{config}\nextra\n')
            rejected = subprocess.run([str(PACKAGE.ROOT / 'scripts/plugin/bridge.sh')],
                                      env={'PLUGIN_DATA': str(root)}, text=True, capture_output=True)
            self.assertNotEqual(rejected.returncode, 0)
            self.assertEqual(rejected.stdout, '')

    def test_chatgpt_variant_does_not_ship_local_process_or_public_binding(self):
        # Arrange / Act
        output = PACKAGE.files(instance='private', apps={'zalo': 'asdk_app_syntheticfixture'})
        # Assert
        self.assertEqual(json.loads(output['mcp.json'])['mcpServers'], {})
        self.assertNotIn('scripts/bridge.sh', output)
        self.assertEqual(json.loads(output['.app.json'])['apps']['zalo']['id'], 'asdk_app_syntheticfixture')
        self.assertNotIn('.app.json', PACKAGE.files())


if __name__ == '__main__':
    unittest.main()

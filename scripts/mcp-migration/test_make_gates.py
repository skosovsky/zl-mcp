"""Exercise failure propagation with a successfully created child executable."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class MakeGateTests(unittest.TestCase):
    def test_failed_build_does_not_execute_candidate(self):
        # Arrange: a late build failure after executable creation, as can happen
        # when provenance extraction/writing fails after go build succeeds.
        with tempfile.TemporaryDirectory() as directory:
            stub = Path(directory) / 'python3'
            marker = Path(directory) / 'candidate-executed'
            stub.write_text('''#!/bin/sh
mkdir -p "$3"
printf '#!/bin/sh\\ntouch "$ZL_GATE_PROBE_MARKER"\\nexit 0\\n' > "$3/zl-mcp-eval-candidate"
chmod +x "$3/zl-mcp-eval-candidate"
exit 1
''')
            stub.chmod(0o755)
            env = dict(os.environ, PATH=directory + os.pathsep + os.environ['PATH'],
                       ZL_GATE_PROBE_MARKER=str(marker))
            # Act.
            result = subprocess.run(['make', 'eval-paired'], cwd=ROOT, env=env,
                                    capture_output=True, text=True, timeout=10)
            # Assert.
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(marker.exists(), 'candidate executed despite build failure')


if __name__ == '__main__':
    unittest.main()

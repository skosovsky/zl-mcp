import os
from pathlib import Path
import signal
import sys
import tempfile
import time
import unittest

from bench_mcp import trial


class DescendantCleanupTest(unittest.TestCase):
    def test_successful_worker_cannot_leave_descendant(self):
        # Arrange: a valid worker result cannot substitute for process cleanup.
        with tempfile.TemporaryDirectory(prefix='zl-bench-negative-') as directory:
            output = Path(directory)
            pidfile = output / 'pid'
            worker = output / 'worker'
            worker.write_text('#!' + sys.executable + '\nimport subprocess,json\n'
                              + 'p=subprocess.Popen(["sleep","60"],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)\n'
                              + 'open(' + repr(str(pidfile)) + ',"w").write(str(p.pid))\n'
                              + 'print(json.dumps(dict(calls=1000,warmup=50,errors=0,cleanup=True)))\n')
            worker.chmod(0o700)
            pid = None
            try:
                # Act
                result = trial(worker, output, 1, 'read', time.monotonic() + 5)
                pid = int(pidfile.read_text())
                # Assert: allow OS reap latency; a zombie already cannot execute.
                self.assertEqual(result['status'], 'PASS')
                deadline = time.monotonic() + 2
                while time.monotonic() < deadline:
                    state = os.popen(f'ps -o stat= -p {pid}').read().strip()
                    if not state or state.startswith('Z'):
                        break
                    time.sleep(0.02)
                self.assertTrue(not state or state.startswith('Z'), state)
            finally:
                if pid is not None:
                    try:
                        os.kill(pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass


if __name__ == '__main__':
    unittest.main()

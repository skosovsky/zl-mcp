import os
import subprocess
import sys
import time
import unittest

from subprocess_reader import BoundedJSONReader


class WatchdogTests(unittest.TestCase):
    def test_partial_line_does_not_bypass_deadline(self):
        # Arrange
        child = subprocess.Popen([sys.executable, '-c', "import sys,time; sys.stdout.write('{'); sys.stdout.flush(); time.sleep(10)"], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        started = time.monotonic()
        reader = BoundedJSONReader(child.stdout, child.stderr)
        try:
            # Act / Assert
            with self.assertRaisesRegex(RuntimeError, 'wall-clock budget'):
                reader.read(time.monotonic() + 0.2)
            self.assertLess(time.monotonic() - started, 2)
        finally:
            child.kill()
            child.wait(timeout=5)
            child.stdout.close()
            child.stderr.close()
        self.assertIsNotNone(child.returncode)

    def test_stderr_flood_is_bounded(self):
        # Arrange
        child = subprocess.Popen([sys.executable, '-c', "import sys,time; sys.stderr.write('x'*20000); sys.stderr.flush(); time.sleep(10)"], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        reader = BoundedJSONReader(child.stdout, child.stderr, limit=1024)
        try:
            # Act / Assert
            with self.assertRaisesRegex(RuntimeError, 'output budget'):
                reader.read(time.monotonic() + 2)
            self.assertLessEqual(len(reader.errors), 1024)
        finally:
            child.kill()
            child.wait(timeout=5)
            child.stdout.close()
            child.stderr.close()


if __name__ == '__main__':
    unittest.main()

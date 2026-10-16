"""Regression checks for missing SDK source resolution, without filesystem copying."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('check-sdk-integration.sh')

class IntegrationHarnessTests(unittest.TestCase):
    def test_empty_sdk_directory_is_rejected_before_copy(self):
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            go = work / 'go'
            go.write_text('#!/bin/sh\nexit 0\n')
            go.chmod(0o700)
            cp = work / 'cp'
            marker = work / 'copy-attempted'
            cp.write_text('#!/bin/sh\ntouch "$COPY_MARKER"\nexit 91\n')
            cp.chmod(0o700)
            env = dict(os.environ, GO=str(go), ZELPORT_RELAY_VERSION='v0.0.0-test',
                       PATH=str(work)+os.pathsep+os.environ['PATH'], COPY_MARKER=str(marker))
            result = subprocess.run(['sh', str(SCRIPT)], env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('refusing integration copy', result.stderr)
            self.assertFalse(marker.exists(), 'cp must never run without a resolved SDK directory')

if __name__ == '__main__':
    unittest.main()

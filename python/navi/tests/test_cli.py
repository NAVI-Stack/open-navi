"""Tests for the pip-installed NAVI CLI wrapper."""
from __future__ import annotations

import sys
import unittest
from pathlib import Path

_PYTHON_DIR = Path(__file__).resolve().parents[2]
if str(_PYTHON_DIR) not in sys.path:
    sys.path.insert(0, str(_PYTHON_DIR))

from navi import cli  # noqa: E402


class CliWrapperTests(unittest.TestCase):
    def test_build_native_env_marks_daemon_control_as_pip_distributed(self):
        env = cli.build_native_env({"PATH": "x", "NAVI_DISTRIBUTION_CHANNEL": "dev"})

        self.assertEqual(env["PATH"], "x")
        self.assertEqual(env["NAVI_DISTRIBUTION_CHANNEL"], "pip")

    def test_resolve_native_binary_honors_override(self):
        override = str(Path("C:/navi/bin/navi.exe"))

        self.assertEqual(cli.resolve_native_binary({"NAVI_NATIVE_BIN": override}), override)


if __name__ == "__main__":
    unittest.main()

"""pip-installed NAVI CLI wrapper.

The Python package is the supported pip entrypoint for local daemon lifecycle
control. It delegates to the packaged native ``navi`` executable after stamping
the distribution channel expected by the Go CLI.
"""
from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path
from typing import Mapping, MutableMapping, Sequence

CHANNEL = "pip"


def executable_name(platform: str = sys.platform) -> str:
    return "navi.exe" if platform == "win32" else "navi"


def resolve_native_binary(env: Mapping[str, str] | None = None) -> str:
    env = env or os.environ
    override = (env.get("NAVI_NATIVE_BIN") or "").strip()
    if override:
        return override

    bundled = Path(__file__).resolve().parent / "bin" / executable_name()
    if bundled.exists():
        return str(bundled)

    raise FileNotFoundError(
        "NAVI native binary was not found. Reinstall the navi pip package, "
        "or set NAVI_NATIVE_BIN to a built navi executable."
    )


def build_native_env(base: Mapping[str, str] | None = None) -> MutableMapping[str, str]:
    env = dict(base or os.environ)
    env["NAVI_DISTRIBUTION_CHANNEL"] = CHANNEL
    return env


def main(argv: Sequence[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    env = build_native_env()
    try:
        bin_path = resolve_native_binary(env)
    except FileNotFoundError as exc:
        print(str(exc), file=sys.stderr)
        return 1

    completed = subprocess.run([bin_path, *args], env=env)
    return int(completed.returncode)


if __name__ == "__main__":
    raise SystemExit(main())

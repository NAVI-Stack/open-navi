import contextlib
import importlib.util
import json
import os
import sys
import time
import traceback


def load_module(entrypoint_path: str):
    module_name = "navi_skill_runtime_module"
    spec = importlib.util.spec_from_file_location(module_name, entrypoint_path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load module from {entrypoint_path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def emit(status: str, output, err_type: str | None, err_message: str | None, started_ms: int):
    payload = {
        "status": status,
        "output": output,
        "duration_ms": int(time.time() * 1000) - started_ms,
    }
    if err_type and err_message:
        payload["error"] = {
            "type": err_type,
            "message": err_message,
        }
    sys.stdout.write(json.dumps(payload))
    sys.stdout.flush()


def main():
    started_ms = int(time.time() * 1000)
    try:
        if len(sys.argv) < 3:
            emit("error", None, "RuntimeError", "usage: runner.py <entrypoint_path> <function_name>", started_ms)
            return

        entrypoint_path = os.path.abspath(sys.argv[1])
        function_name = sys.argv[2]
        raw = sys.stdin.read()
        params = {}
        if raw.strip():
            params = json.loads(raw)

        module = load_module(entrypoint_path)
        fn = getattr(module, function_name, None)
        if fn is None:
            emit("error", None, "AttributeError", f"function {function_name!r} not found", started_ms)
            return

        with contextlib.redirect_stdout(sys.stderr):
            output = fn(params)
        emit("success", output, None, None, started_ms)
    except Exception as exc:  # noqa: BLE001
        traceback.print_exc(file=sys.stderr)
        emit("error", None, type(exc).__name__, str(exc), started_ms)


if __name__ == "__main__":
    main()

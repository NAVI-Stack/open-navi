"""Minimal Python subprocess skill fixture for testing the subprocess_python transport."""
import json
import sys
import time


def main():
    start = time.time()
    try:
        raw = sys.stdin.read()
        payload = json.loads(raw) if raw.strip() else {}
        interface = payload.get("interface") or payload.get("action", "")

        if interface == "health_check":
            result = {
                "status": "success",
                "output": {"status": "ok"},
                "duration_ms": int((time.time() - start) * 1000),
            }
        else:
            result = {
                "status": "error",
                "error": {"type": "ValueError", "message": f"unknown interface: {interface}"},
                "duration_ms": int((time.time() - start) * 1000),
            }
    except Exception as exc:
        result = {
            "status": "error",
            "error": {"type": type(exc).__name__, "message": str(exc)},
            "duration_ms": int((time.time() - start) * 1000),
        }

    sys.stdout.write(json.dumps(result))
    sys.stdout.flush()


if __name__ == "__main__":
    main()

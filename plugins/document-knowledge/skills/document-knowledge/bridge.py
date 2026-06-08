from __future__ import annotations

import json
import os
import pathlib
import re
import sys
import time


def _emit_result(status: str, output: dict | None, error_message: str | None, start_ms: int) -> None:
    payload = {
        "status": status,
        "output": output,
        "duration_ms": int(time.time() * 1000) - start_ms,
    }
    if error_message:
        payload["error"] = {"type": "RuntimeError", "message": error_message}
    print(json.dumps(payload), flush=True)


def _decode_pdf_literal(value: str) -> str:
    value = value.replace("\\(", "(").replace("\\)", ")").replace("\\n", "\n")
    return value.replace("\\r", "\r").replace("\\t", "\t").replace("\\\\", "\\")


def _extract_simple_pdf_text(path: pathlib.Path) -> tuple[str, int]:
    raw = path.read_bytes().decode("latin1", errors="ignore")
    matches = [_decode_pdf_literal(m) for m in re.findall(r"\(([^()]*)\)\s*Tj", raw)]
    for block in re.findall(r"\[(.*?)\]\s*TJ", raw, flags=re.S):
        for piece in re.findall(r"\(([^()]*)\)", block):
            matches.append(_decode_pdf_literal(piece))
    text = "\n".join(part.strip() for part in matches if part.strip())
    page_count = raw.count("/Type /Page")
    return text, max(page_count, 1 if text else 0)


def _extract_pdf_text(path: pathlib.Path) -> tuple[str, int]:
    try:
        from pypdf import PdfReader  # type: ignore

        reader = PdfReader(str(path))
        parts = [(page.extract_text() or "").strip() for page in reader.pages]
        text = "\n\n".join(part for part in parts if part)
        return text, len(reader.pages)
    except Exception:
        return _extract_simple_pdf_text(path)


def _extract_text(path: pathlib.Path) -> tuple[str, str, int]:
    suffix = path.suffix.lower()
    if suffix == ".pdf":
        text, page_count = _extract_pdf_text(path)
        return text, "pdf", page_count
    text = path.read_text(encoding="utf-8", errors="ignore")
    return text, "text", 1 if text else 0


def _extract_entities_from_text(text: str) -> list[dict]:
    seen: set[tuple[str, str]] = set()
    entities: list[dict] = []

    def add(value: str, kind: str) -> None:
        value = value.strip().strip(".,;:")
        if not value:
            return
        key = (value.lower(), kind)
        if key in seen:
            return
        seen.add(key)
        entities.append({"text": value, "type": kind})

    for email in re.findall(r"\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b", text, flags=re.I):
        add(email, "email")
    for url in re.findall(r"https?://[^\s)>\]]+", text, flags=re.I):
        add(url, "url")
    for phrase in re.findall(r"\b(?:[A-Z][a-z]+(?:\s+[A-Z][a-z]+){0,3})\b", text):
        add(phrase, "proper_noun")

    return entities[:20]


def extract_entities(params: dict) -> dict:
    raw_path = params.get("path", "")
    if not raw_path:
        raise ValueError("path is required")
    path = pathlib.Path(raw_path)
    if not path.is_absolute():
        workspace_dir = os.environ.get("NAVI_WORKSPACE_DIR", "").strip()
        if not workspace_dir:
            raise ValueError("relative document paths require NAVI_WORKSPACE_DIR")
        path = pathlib.Path(workspace_dir) / path
    path = path.resolve()
    if not path.exists():
        raise ValueError(f"document not found: {raw_path}")

    text, document_type, page_count = _extract_text(path)
    if not text.strip():
        raise ValueError("document contains no extractable text")

    preview = text[:2000].strip()
    if len(text) > 2000:
        preview += "\n... [truncated]"

    return {
        "path": str(path),
        "document_type": document_type,
        "page_count": page_count,
        "text": text,
        "text_preview": preview,
        "entities": _extract_entities_from_text(text),
    }


def main() -> None:
    start_ms = int(time.time() * 1000)
    try:
        raw = sys.stdin.read()
        if not raw.strip():
            _emit_result("error", None, "No input", start_ms)
            return
        data = json.loads(raw)
        result = extract_entities(data)
        _emit_result("success", result, None, start_ms)
    except Exception as exc:  # pragma: no cover - emitted for runtime visibility
        _emit_result("error", None, f"{type(exc).__name__}: {exc}", start_ms)


if __name__ == "__main__":
    main()

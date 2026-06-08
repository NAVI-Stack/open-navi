#!/usr/bin/env python3
"""NAVI Context Intake Pipeline — Python extraction + resolution worker (CIP P3).

This is the Python side of the synthesis seam (docs/design/intake-synthesis-seam.md
§9). It performs two jobs and *only* these two jobs:

  1. Extraction  — parse a distilled chunk into entity candidates (proper-noun
                   spans, emails, handles, identifiers, dates) with span-level
                   provenance and deterministic blocking keys.
  2. Resolution  — decide whether each candidate matches an existing World Model
                   entity. Deterministic blocking keys first; token-similarity for
                   the fuzzy cases; ambiguity is reported honestly, never guessed.

The dataflow is strictly Python -> Go. This process **never** writes the World
Model and **never** decides authority — it produces structured envelopes
(ExtractionResult, ResolutionResult). Go's Governor decides the outcome.

IPC reuses the JSON-RPC-over-stdio contract used by the skill subprocess runtime
(internal/navi/skill/subprocess_runtime.go): a single JSON-RPC 2.0 request object
is read from stdin and a single JSON-RPC 2.0 response object is written to stdout.
Stdlib only — no third-party dependencies, so no venv provisioning is required.
"""

import json
import re
import sys

# ---- Extraction -----------------------------------------------------------

_EMAIL_RE = re.compile(r"[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}")
_HANDLE_RE = re.compile(r"(?<![A-Za-z0-9_])@([A-Za-z0-9_]{2,})")
_URL_RE = re.compile(r"https?://[^\s)>\]]+")
_DATE_RE = re.compile(
    r"\b(\d{4}-\d{2}-\d{2}|\d{1,2}/\d{1,2}/\d{2,4}|"
    r"(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+\d{1,2},?\s+\d{4})\b"
)
_IDENT_RE = re.compile(r"\b([A-Z]{2,}[-_]?\d{2,}|[A-Za-z]+\d{3,})\b")
# Proper-noun span: one or more capitalized words (allows internal lowercase
# connectors like "de", "van"). Heuristic, deterministic, no NLP model.
_PROPER_NOUN_RE = re.compile(r"\b([A-Z][a-z]+(?:\s+(?:[A-Z][a-z]+|de|van|von|la|le)){0,3})\b")

# Common leading words that get capitalized at sentence start but are not names.
_STOPWORDS = {
    "The", "This", "That", "These", "Those", "There", "Here", "When", "Where",
    "What", "Which", "Who", "Why", "How", "And", "But", "Or", "If", "Then",
    "Hello", "Hi", "Hey", "Thanks", "Thank", "Please", "Dear", "Regards", "Best",
    "I", "We", "You", "He", "She", "It", "They",
}


def normalize_name(name):
    return re.sub(r"\s+", " ", name.strip().lower())


def _add(candidates, occupied, name, kind, value, start, end, confidence, blocking):
    # Suppress overlaps with an already-claimed (higher-priority) span.
    for s, e in occupied:
        if start < e and end > s:
            return
    occupied.append((start, end))
    candidates.append({
        "name": name,
        "kind": kind,
        "value": value,
        "start_offset": start,
        "end_offset": end,
        "confidence": confidence,
        "blocking_keys": blocking,
    })


def extract(chunk_id, content):
    """Extract entity candidates from a distilled chunk.

    Higher-priority kinds (email, handle, url, date, identifier) claim their spans
    first so a lower-priority proper-noun match cannot overlap them.
    """
    candidates = []
    occupied = []

    for m in _EMAIL_RE.finditer(content):
        email = m.group(0).lower()
        _add(candidates, occupied, m.group(0), "email", email, m.start(), m.end(),
             0.95, {"email": email})
    for m in _HANDLE_RE.finditer(content):
        handle = m.group(1).lower()
        _add(candidates, occupied, "@" + m.group(1), "handle", handle, m.start(), m.end(),
             0.9, {"handle": handle})
    for m in _URL_RE.finditer(content):
        _add(candidates, occupied, m.group(0), "url", m.group(0), m.start(), m.end(),
             0.85, {})
    for m in _DATE_RE.finditer(content):
        _add(candidates, occupied, m.group(0), "date", m.group(0), m.start(), m.end(),
             0.8, {})
    for m in _IDENT_RE.finditer(content):
        _add(candidates, occupied, m.group(0), "identifier", m.group(0), m.start(), m.end(),
             0.75, {"source_anchored_id": m.group(0).lower()})
    for m in _PROPER_NOUN_RE.finditer(content):
        name = m.group(1).strip()
        first = name.split()[0]
        if first in _STOPWORDS and " " not in name:
            continue
        _add(candidates, occupied, name, "proper_noun", name, m.start(), m.end(),
             0.6, {"normalized_name": normalize_name(name)})

    candidates.sort(key=lambda c: c["start_offset"])
    return {"chunk_id": chunk_id, "candidates": candidates}


# ---- Resolution -----------------------------------------------------------

def _name_tokens(name):
    return set(t for t in normalize_name(name).split() if t)


def _jaccard(a, b):
    ta, tb = _name_tokens(a), _name_tokens(b)
    if not ta or not tb:
        return 0.0
    return len(ta & tb) / len(ta | tb)


def resolve_one(candidate, existing):
    """Resolve a single candidate against existing entities.

    Pass 1 — deterministic blocking keys (email, handle, normalized name,
    source-anchored id). An exact key hit is a high-confidence match.
    Pass 2 — token similarity for the fuzzy cases. A single strong unique match is
    a match; multiple comparable matches or a mid-band similarity is *ambiguous*,
    which Go's Risk rubric maps to Modified (low-confidence Create with
    possible_merge_with) — never a guessed merge.
    """
    cname = candidate.get("name", "")
    ckeys = candidate.get("blocking_keys", {}) or {}

    # Pass 1: deterministic blocking keys (email, handle, source-anchored id).
    det_matches = []
    for ent in existing:
        ekeys = ent.get("blocking_keys", {}) or {}
        for key in ("email", "handle", "source_anchored_id"):
            cv = ckeys.get(key)
            if cv and ekeys.get(key) == cv:
                det_matches.append(ent)
                break
    if len(det_matches) == 1:
        ent = det_matches[0]
        return {
            "candidate_name": cname,
            "matched_id": ent.get("id", ""),
            "matched_type": ent.get("type", ""),
            "confidence": 0.97,
            "reason": "deterministic blocking key match",
            "ambiguous": False,
        }
    if len(det_matches) > 1:
        # The same strong identity is shared by several existing entities: they
        # are confident duplicates that should merge. This is NOT ambiguous — it
        # is a confident structural merge, which Go hard-floors into a Proposal.
        return {
            "candidate_name": cname,
            "matched_id": "",
            "confidence": 0.95,
            "reason": "shared deterministic key across multiple entities — confident merge",
            "ambiguous": False,
            "possible_merge_ids": [e.get("id", "") for e in det_matches],
        }
    cnorm = ckeys.get("normalized_name") or normalize_name(cname)
    exact_name_matches = [e for e in existing
                          if (e.get("blocking_keys", {}) or {}).get("normalized_name") == cnorm
                          or normalize_name(e.get("name", "")) == cnorm]
    if len(exact_name_matches) == 1:
        ent = exact_name_matches[0]
        return {
            "candidate_name": cname,
            "matched_id": ent.get("id", ""),
            "matched_type": ent.get("type", ""),
            "confidence": 0.9,
            "reason": "exact normalized-name match",
            "ambiguous": False,
        }
    if len(exact_name_matches) > 1:
        return {
            "candidate_name": cname,
            "confidence": 0.5,
            "reason": "multiple exact-name matches",
            "ambiguous": True,
            "possible_merge_ids": [e.get("id", "") for e in exact_name_matches],
        }

    # Pass 2: similarity.
    scored = []
    for ent in existing:
        sim = _jaccard(cname, ent.get("name", ""))
        if sim > 0:
            scored.append((sim, ent))
    scored.sort(key=lambda x: x[0], reverse=True)

    strong = [e for sim, e in scored if sim >= 0.85]
    mid = [e for sim, e in scored if 0.5 <= sim < 0.85]

    if len(strong) == 1 and not mid:
        ent = strong[0]
        return {
            "candidate_name": cname,
            "matched_id": ent.get("id", ""),
            "matched_type": ent.get("type", ""),
            "confidence": 0.85,
            "reason": "high token similarity",
            "ambiguous": False,
        }
    if len(strong) > 1 or mid:
        possible = [e.get("id", "") for e in strong] + [e.get("id", "") for e in mid]
        return {
            "candidate_name": cname,
            "confidence": 0.5,
            "reason": "ambiguous similarity match",
            "ambiguous": True,
            "possible_merge_ids": possible,
        }

    # No match — a brand-new entity.
    return {
        "candidate_name": cname,
        "confidence": 0.0,
        "reason": "no match",
        "ambiguous": False,
    }


def process(params):
    chunk_id = params.get("chunk_id", "")
    content = params.get("content", "")
    existing = params.get("existing_entities", []) or []
    extraction = extract(chunk_id, content)
    resolutions = [resolve_one(c, existing) for c in extraction["candidates"]]
    return {"extraction": extraction, "resolutions": resolutions}


def main():
    raw = sys.stdin.read()
    try:
        req = json.loads(raw)
    except Exception as exc:  # noqa: BLE001
        json.dump({"jsonrpc": "2.0", "id": None,
                   "error": {"code": -32700, "message": "parse error: %s" % exc}},
                  sys.stdout)
        return
    req_id = req.get("id")
    method = req.get("method", "")
    params = req.get("params", {}) or {}
    try:
        if method in ("process", "extract_resolve"):
            result = process(params)
        elif method == "extract":
            result = {"extraction": extract(params.get("chunk_id", ""), params.get("content", ""))}
        elif method == "resolve":
            existing = params.get("existing_entities", []) or []
            result = {"resolutions": [resolve_one(c, existing) for c in params.get("candidates", [])]}
        else:
            json.dump({"jsonrpc": "2.0", "id": req_id,
                       "error": {"code": -32601, "message": "unknown method %r" % method}},
                      sys.stdout)
            return
    except Exception as exc:  # noqa: BLE001
        json.dump({"jsonrpc": "2.0", "id": req_id,
                   "error": {"code": -32000, "message": "worker error: %s" % exc}},
                  sys.stdout)
        return
    json.dump({"jsonrpc": "2.0", "id": req_id, "result": result}, sys.stdout)


if __name__ == "__main__":
    main()

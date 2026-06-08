# Document Knowledge Ingestion Skill

OMN-35 adds the first real document-to-knowledge path in NAVI.

## Shipped Scope

- Builtin `document-knowledge` skill
- `extract_entities(path)` via `subprocess_python`
- `summarize_document(path)` via internal handler + LLM
- `ingest_document(path)` via internal handler + LLM + fact/memory persistence

## Runtime Shape

The Python bridge handles document parsing and lightweight entity extraction.
The Go-side internal handlers then:

- resolve the workspace-relative document path safely
- reuse the Python extraction result
- ask the configured LLM for a structured summary plus durable facts
- persist those facts into the memory store
- persist a session memory checkpoint when a session context exists

## Current Limits

- PDF parsing is dependency-free and intentionally basic; it prefers `pypdf`
  when available and otherwise falls back to simple text-stream extraction for
  uncomplicated PDFs.
- The first slice is optimized for local workspace ingestion, not remote URLs or
  full RAG indexing.

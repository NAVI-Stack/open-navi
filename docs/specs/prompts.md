# Prompt templates

NAVI loads Markdown templates for system prompts and worker prompts from disk (under `prompts_dir`) with embedded copies as fallback. Operators can customize files, enable filesystem watching, reload without restart, or override the chat system prompt via environment.

## Layout

Default root directory is `./prompts` (config: `navi.prompts_dir`, env: `NAVI_PROMPTS_DIR`). Files mirror template kinds, for example:

- `chat/system.md` — main conversational system prompt (also referred to in older docs as the chat template path).
- `orchestrator/directive.md`, `orchestrator/implement.md`, `orchestrator/decompose.md`
- `summarizer/system.md`, `summarizer/user.md`
- `heartbeat/cycle.md`
- `agents/coder/system.md`, `agents/critic/system.md`, `agents/scout/system.md`, `agents/strategist/system.md`

On first run, missing files are seeded from embedded defaults. Whitespace-only on-disk files are rewritten from the embedded default so empty placeholders do not linger.

## Environment variables

| Variable | Effect |
|----------|--------|
| `NAVI_PROMPTS_DIR` | Directory for prompt `.md` files (overrides YAML `navi.prompts_dir`). |
| `NAVI_PROMPT_WATCH` | When `1` / `true` / `yes`, watch the prompts directory for changes; `0` / `false` disables. |
| `NAVI_CHAT_SYSTEM_PROMPT` | See **Chat runtime override** below. YAML: `navi.chat_system_prompt_override`. |

### Chat runtime override

`NAVI_CHAT_SYSTEM_PROMPT` (and the YAML field) is **not** layered on top of `chat/system.md`. When set to a non-empty value after trimming, it becomes the **entire** system message sent to the model for the agent chat loop: the `chat/system.md` template is not executed, so persona block, skills catalog, facts block, session summary block, and time/session fields from that template are **not** auto-injected. Use `chat/system.md` when you want those pieces; use the env override only when you want full manual control of the system string.

## Hot reload

- **API:** Authenticated `POST /api/prompts/reload` reparses templates into the in-process prompt manager (same auth pattern as other `/api/*` routes). Returns `{"status":"ok"}` on success. If NAVI is not configured on the gateway, responds with `503`.
- **Watch:** When prompt watch is enabled, file changes under `prompts_dir` trigger reload automatically.

### Recovering from startup fallback

If startup logs either:

- `prompts: seeding defaults failed, using embedded templates until disk becomes available`
- `prompts: loading from disk failed, using embedded templates until files are fixed`

the daemon keeps serving bundled prompt templates from memory, but it **still keeps the configured `prompts_dir` attached**. After you fix the directory or the invalid `.md` files, either:

- save the files and let the watcher reload them, or
- call `POST /api/prompts/reload`

No restart is required for prompt recovery.

## Introspection

`prompts.Manager.Snapshot(kind)` returns raw template bytes and a `source` field. It does not execute the template.

- `disk`: the on-disk file was read directly.
- `embed`: the manager is operating in embed-only mode.
- `empty_fallback`: the on-disk file existed but was whitespace-only, so embed content was used.
- `missing_fallback`: the on-disk file was missing, so embed content was used.
- `oversized_fallback`: the on-disk file exceeded the prompt size limit, so embed content was used.

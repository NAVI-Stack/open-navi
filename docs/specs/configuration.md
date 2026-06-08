# Runtime Configuration

> [!NOTE]
> Part of the [NAVI Systems Map](../architecture/navi-systems-map.md).


**Status:** Active
**Last Updated:** 2026-06-01
**Source of truth:** `internal/config/config.go`, `config/runtime.yaml`, `cmd/navid/main.go`

This document describes the configuration that the daemon actually uses today.

## Load Order

At daemon startup, configuration is assembled in this order:

1. hard-coded defaults from `internal/config/config.go`
2. values from `config/runtime.yaml` when the file exists
3. environment variable overrides from `applyEnvOverrides()`
4. selected persisted settings loaded from SQLite in `cmd/navid/main.go`

Step 4 matters because onboarding and operator flows can persist runtime choices such as:

- active LLM keys and models
- Brave Search API key
- gateway shared secret
- Telegram account settings

## Validation

Startup fails when these conditions are not met:

- at least one LLM provider is configured through an API key or `NAVI_OLLAMA_URL`
- `navi.prompts_dir` exists or can be created
- the parent directory of `sqlite.path` exists and is writable

## Top-Level YAML Shape

The current config struct contains these sections:

| Section | Purpose |
|---------|---------|
| `gateway` | HTTP listen address, static dir, shared secret, allowed browser origins |
| `nats` | NATS URL, typically `embedded` for local development |
| `sqlite` | SQLite file path |
| `connectors` | Built-in connector configuration for Telegram and Slack |
| `lsp` | LSP workspace root |
| `llm` | Provider credentials, model defaults, provider catalog, routing |
| `navi` | Workspace paths, prompt and heartbeat settings, plus experience profile overrides |
| `governor` | Hard limits and retention knobs |
| `autonomy` | Global and per-domain autonomy presets and dimension overrides |

## Current Defaults

The defaults below reflect `defaults()` in `internal/config/config.go`.

```yaml
gateway:
  addr: ":6284"
  static_dir: "web"

nats:
  url: "embedded"

sqlite:
  path: "navi.db"

llm:
  ollama_url: "http://localhost:11434/v1"
  ollama_model: "llama3:latest"
  ollama_discuss_model: "llama3:latest"
  orchestrator_model: "llama3:latest"
  coder_model: "qwen3-coder:30b"
  chat_model: "llama3:latest"

navi:
  workspace_dir: "workspace"
  experience_profiles_dir: "config/personas"
  artifacts_dir: "artifacts"
  prompts_dir: "prompts"
  prompt_watch_enabled: true
  initial_experience_mode: "navi"
  heartbeat_enabled: true
  heartbeat_interval: "30m"
  reflection_consolidation_interval: "30m"
  llm_call_timeout: "5m"
  max_response_tokens: 4096

governor:
  max_action_budget: 1000
  max_retries: 5
  cost_ceiling: 50.0
  autonomous_duration: "72h"

autonomy:
  global_preset: "balanced"
```

The checked-in `config/runtime.yaml` then overrides some of these defaults for local development.

## Important Fields

### `gateway`

- `addr`: listen address for the HTTP server
- `shared_secret`: shared secret accepted by the gateway for connector compatibility
- `origin_patterns`: explicit browser origins allowed for CORS
- `static_dir`: directory served at `/`

### `connectors.telegram`

- `bot_token`
- `token_file`
- `owner_chat_id`
- `allow_from`
- `pairing_code`
- `gateway_url`
- `api_url`
- `webhook_url`
- `webhook_secret`
- `accounts`: optional multi-account configuration; when present, the legacy single-account fields are ignored by the daemon

### `connectors.slack`

- `bot_token`
- `app_token`
- `gateway_url`
- `allowed_user_ids`

### `llm`

- provider credentials: `anthropic_key`, `openai_key`, `openrouter_key`
- Ollama endpoint: `ollama_url`
- default model slots: `ollama_model`, `ollama_discuss_model`, `orchestrator_model`, `coder_model`, `chat_model`
- optional provider catalog: `providers`
- optional logical routes: `routes`
- external search credential: `brave_search_key`

### `navi`

- filesystem roots: `workspace_dir`, `artifacts_dir`, `prompts_dir`
- optional experience profile override dir: `experience_profiles_dir`
- onboarding/default experience selector: `initial_experience_mode`
- prompt watch: `prompt_watch_enabled`
- heartbeat controls: `heartbeat_enabled`, `heartbeat_interval`, quiet-hours fields, DND flag
- background reflection cadence: `reflection_consolidation_interval`
- LLM execution caps: `llm_call_timeout`, `max_response_tokens`
- optional prompt override: `chat_system_prompt_override`

### `governor`

- `max_action_budget`
- `max_retries`
- `cost_ceiling`
- `autonomous_duration`
- `execution_outcome_retention_days`

### `autonomy`

- `global_preset`
- `domain_overrides`
- `domain_dimension_overrides`

The supported domain keys match the autonomy model used by governor validation, such as `messaging`, `scheduling`, `coding`, `memory`, `plugins`, `workflow`, `delegation`, and `configuration`.

## Environment Overrides

These `NAVI_*` variables are recognized directly by `internal/config/config.go`:

| Variable | Maps to |
|----------|---------|
| `NAVI_GATEWAY_ADDR` | `gateway.addr` |
| `NAVI_GATEWAY_SHARED_SECRET` | `gateway.shared_secret` |
| `NAVI_TELEGRAM_BOT_TOKEN` | `connectors.telegram.bot_token` |
| `TELEGRAM_BOT_TOKEN` | compatibility alias for `connectors.telegram.bot_token` |
| `NAVI_TELEGRAM_OWNER_CHAT_ID` | `connectors.telegram.owner_chat_id` |
| `NAVI_TELEGRAM_ALLOW_FROM` | `connectors.telegram.allow_from` |
| `NAVI_TELEGRAM_PAIRING_CODE` | `connectors.telegram.pairing_code` |
| `NAVI_TELEGRAM_API_URL` | `connectors.telegram.api_url` |
| `NAVI_SLACK_BOT_TOKEN` | `connectors.slack.bot_token` |
| `NAVI_ORCHESTRATOR_MODEL` | `llm.orchestrator_model` |
| `NAVI_OLLAMA_URL` | `llm.ollama_url` |
| `NAVI_ANTHROPIC_KEY` | `llm.anthropic_key` |
| `NAVI_OPENAI_KEY` | `llm.openai_key` |
| `NAVI_OPENROUTER_KEY` | `llm.openrouter_key` |
| `NAVI_BRAVE_SEARCH_KEY` | `llm.brave_search_key` |
| `NAVI_SQLITE_PATH` | `sqlite.path` |
| `NAVI_NATS_URL` | `nats.url` |
| `NAVI_WORKSPACE_DIR` | `navi.workspace_dir` |
| `NAVI_EXPERIENCE_PROFILES_DIR` | `navi.experience_profiles_dir` (primary experience profile override dir) |
| `NAVI_ARTIFACTS_DIR` | `navi.artifacts_dir` |
| `NAVI_PROMPTS_DIR` | `navi.prompts_dir` |
| `NAVI_CHAT_SYSTEM_PROMPT` | `navi.chat_system_prompt_override` |
| `NAVI_PROMPT_WATCH` | `navi.prompt_watch_enabled` |
| `NAVI_INITIAL_EXPERIENCE_MODE` | `navi.initial_experience_mode` (only `wizard` remains distinct; all other values normalize to `navi`) |
| `NAVI_HEARTBEAT_INTERVAL` | `navi.heartbeat_interval` |
| `NAVI_HEARTBEAT_QUIET_HOURS_START` | `navi.heartbeat_quiet_hours_start` |
| `NAVI_HEARTBEAT_QUIET_HOURS_END` | `navi.heartbeat_quiet_hours_end` |
| `NAVI_HEARTBEAT_DND` | `navi.heartbeat_dnd_enabled` |
| `NAVI_REFLECTION_CONSOLIDATION_INTERVAL` | `navi.reflection_consolidation_interval` |
| `NAVI_LLM_CALL_TIMEOUT` | `navi.llm_call_timeout` |
| `NAVI_MAX_RESPONSE_TOKENS` | `navi.max_response_tokens` |
| `NAVI_DEBUG` | `navi.debug` |
| `NAVI_GOVERNOR_MAX_ACTION_BUDGET` | `governor.max_action_budget` |
| `NAVI_GOVERNOR_MAX_RETRIES` | `governor.max_retries` |
| `NAVI_GOVERNOR_COST_CEILING` | `governor.cost_ceiling` |
| `NAVI_GOVERNOR_AUTONOMOUS_DURATION` | `governor.autonomous_duration` |
| `NAVI_GOVERNOR_EXECUTION_OUTCOME_RETENTION_DAYS` | `governor.execution_outcome_retention_days` |

Some `HELM_*` aliases are still accepted for compatibility. New documentation should prefer the `NAVI_*` names.

## Operational Notes

- The daemon persists some onboarding-managed settings to SQLite and reapplies them at boot.
- The CLI has its own local config file at `~/.navi/config.json`; that file controls CLI defaults such as gateway URL and API key, not the daemon's runtime config.
- `navi.prompts_dir` is writable and can be hot-reloaded when `prompt_watch_enabled` is true.
- `nats.url: "embedded"` is the normal local-development mode.

[specs INDEX](INDEX.md) | [docs INDEX](../INDEX.md)

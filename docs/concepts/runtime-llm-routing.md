## NAVI — Runtime-Switchable LLM Routing

**Status:** Active  
**Last Updated:** 2026-03-11  

This document describes how NAVI discovers available LLMs, exposes them to the agent, and allows users to inspect and switch the active provider/model at runtime via prompt commands.

---

### Overview

- **Goal**: Make NAVI behave as a runtime-switchable LLM router where the user can:
  - **Inspect** all configured providers and models.
  - **See** which provider/model is currently active.
  - **Switch** to a different provider/model through natural-language prompts.
- **Scope**: Applies to all components that use the shared LLM provider:
  - NAVI Persona System (Experience Layer).
  - Orchestrator directive adapter.
  - Coder, Critic, Strategist, and Scout workers.

At a high level:

- Configuration defines which **providers** and **models** exist.
- A **catalog** (`LLMCatalog`) is derived from configuration.
- A **selection state** (`SelectionState`) persists the active provider/model.
- A shared **DynamicProvider** routes all LLM calls according to the selection.
- An **llm-router skill** exposes list/get/set operations as tools.
- Persona prompts within the Experience Layer instruct the LLM to use these tools when users ask about models.

---

### Central LLM Configuration & Catalog

#### LLMConfig and ProviderConfig

The canonical Go types live in `internal/config/config.go`:

```106:125:internal/config/config.go
type ProviderConfig struct {
    DisplayName string   `yaml:"display_name,omitempty"`
    Models      []string `yaml:"models,omitempty"`
}

type LLMConfig struct {
    OllamaURL          string `yaml:"ollama_url"`
    OllamaModel        string `yaml:"ollama_model"`
    OllamaDiscussModel string `yaml:"ollama_discuss_model"`
    AnthropicKey       string `yaml:"anthropic_key"`
    AnthropicModel     string `yaml:"anthropic_model"`
    OpenAIKey          string `yaml:"openai_key"`
    OpenAIModel        string `yaml:"openai_model"`
    OpenRouterKey      string `yaml:"openrouter_key"`
    OpenRouterModel    string `yaml:"openrouter_model"`
    OrchestratorModel  string `yaml:"orchestrator_model"`
    CoderModel         string `yaml:"coder_model"`
    ChatModel          string `yaml:"chat_model"`

    Providers map[string]ProviderConfig `yaml:"providers,omitempty"`
    Routes    map[string]ModelRoute     `yaml:"routes,omitempty"`
}
```

- **Legacy fields** (`OllamaModel`, `AnthropicModel`, etc.) remain the source of truth for minimum configuration and routing.
- **New `Providers` map** (optional) defines a catalog of enabled models per provider.
  - Keys are logical provider identifiers (e.g. `ollama`, `anthropic`, `openai`).
  - `DisplayName` is for human-facing listings.
  - `Models` is a free-form list of model IDs understood by the provider.

#### LLMCatalog

`internal/llm/catalog.go` derives a read-only catalog from `LLMConfig`:

```1:24:internal/llm/catalog.go
type LLMModelInfo struct {
    Name string `json:"name"`
}

type LLMProviderInfo struct {
    Key         string         `json:"key"`
    DisplayName string         `json:"display_name"`
    Models      []LLMModelInfo `json:"models"`
}

type LLMCatalog struct {
    Providers []LLMProviderInfo `json:"providers"`
}
```

Construction rules:

- If `LLMConfig.Providers` is **non-empty**, it is the canonical catalog:
  - Arbitrary provider keys and model IDs are supported.
  - Provider and model lists are normalized and deduplicated.
- If `LLMConfig.Providers` is **empty**, a catalog is synthesized from legacy fields:
  - Ollama: `OllamaURL` + `{OllamaModel, OllamaDiscussModel}`.
  - Anthropic: `AnthropicKey` + `AnthropicModel`.
  - OpenAI: `OpenAIKey` + `OpenAIModel`.
  - OpenRouter: `OpenRouterKey` + `OpenRouterModel`.

Helpers:

- **ListProviders** — returns provider keys in stable order.
- **ListModels(providerKey)** — returns all model names for the given provider key.

These helpers are the single source of truth for model discovery across the codebase.

---

### Active LLM Selection State

#### SelectionState and SettingStore

Active selection is tracked in `internal/llm/selection.go`:

```37:55:internal/llm/selection.go
type SettingStore interface {
    GetSetting(ctx context.Context, key string) (value string, found bool, err error)
    SetSetting(ctx context.Context, key, value string) error
}

type SelectionState struct {
    store SettingStore
    cfg   *config.LLMConfig
    dp    *DynamicProvider
}

type Active struct {
    Provider string
    Model    string
}
```

- `SelectionState` is responsible for:
  - Reading persisted provider/model selection from the settings table.
  - Validating new selections against `LLMCatalog`.
  - Swapping the underlying provider/model on `DynamicProvider`.
- `SettingStore` abstracts persistence, so `llm` does not depend on `database/sql`.
  - In `cmd/navid`, it is backed by `store.GetSetting` / `store.SetSetting` on the SQLite `settings` table.

#### GetActive and SetActive

Selection logic:

- **GetActive**:
  - Reads `llm_provider` and `llm_model` from settings when available.
  - Falls back to the first provider/model in the catalog.
  - As a last resort, falls back to config defaults in this priority:
    - Ollama → Anthropic → OpenAI → OpenRouter.
- **SetActive**:
  - Normalizes provider key and model name (case-insensitive match).
  - Validates the pair against `LLMCatalog`.
  - Builds a concrete provider instance for the selected provider using `LLMConfig` (keys/URLs).
  - Persists `llm_provider` and `llm_model` to settings.
  - Calls `DynamicProvider.SwapWithModel` so subsequent `Chat` / `ChatStream` calls use the new backend.

The selection state is **global** to the daemon: all components (NAVI chat, orchestrator, Coder, Critic, Strategist, Scout) share the same active provider/model.

---

### Contextual Routing

NAVI now applies a task-aware routing pass before each primary turn. The runtime classifies the current user message into a task class (`chat`, `lightweight`, `coding`, `reasoning`, or `agentic`) plus a coarse complexity tier (`low`, `medium`, `high`).

That classification feeds a seeded profile registry built from the live `LLMCatalog`. Each available provider/model pair gets a `ModelProfile` with:

- tool support and tool-call reliability
- relative task scores (agentic, coding, chat, reasoning)
- latency and cost bias scores
- tags such as `local`, `cloud`, `fast`, `coding`, and `architect`

The selector then chooses the highest-scoring available profile while applying two hard rules:

- tool-using turns never select models that are marked unreliable for tools
- per-turn or persisted user overrides are applied before normal scoring

When the selected provider/model differs from the current active pair, NAVI hot-swaps the shared `DynamicProvider` via `SwapWithModel()` and prepends a short user-visible notice such as:

```text
Switching to anthropic/claude-sonnet-4-20250514 for this coding task.
```

This means routing can shift mid-session without restarting the daemon.

---

### User Overrides And Preferences

Two override paths are supported:

- **Per-turn override**: phrases like `use opus for this`, `use anthropic for this`, or `use local for this`
- **Persisted preference**: phrases like `always use local` or operator updates through the API

Persisted routing preferences are stored in the existing SQLite `settings` table under `llm_routing_preferences`, so they survive restarts independently of the manually selected active model.

Operator endpoints:

- `GET /api/llm/profiles` — returns the seeded capability registry for the currently available catalog
- `GET /api/llm/preferences` — returns persisted routing preferences
- `PATCH /api/llm/preferences` — updates persisted routing preferences

---

### Learned Calibration

The profile registry is no longer purely static. NAVI now records the selected provider/model, task class, and complexity on execution outcomes produced during routed runs. The profiles endpoint and selector both fold recent execution outcomes back into the seeded profile scores:

- repeated `agentic` failures reduce `AgenticScore`
- repeated `coding` failures reduce `CodingScore`
- successful lightweight turns increase chat-oriented confidence
- the learned deltas are surfaced directly on each returned profile (`learned_*_delta`, `learned_evidence_count`)

This keeps the seed profiles as the baseline while making routing explainable and self-correcting from observed outcomes.

---

### Wiring Into the Provider Stack

#### FromConfig and DynamicProvider

`internal/llm/factory.go` still builds the base provider chain (`Provider` or `FallbackChain`) from `LLMConfig` and env vars. `cmd/navid/main.go` then wraps this in a `DynamicProvider` and selection state:

```245:257:cmd/navid/main.go
baseProvider, provErr := llm.FromConfig(cfg)
dp := llm.NewDynamicProvider(baseProvider)
catalog := llm.BuildCatalog(&cfg.LLM)
selectionStore := llm.NewSettingStore(
    func(ctx context.Context, key string) (string, bool, error) {
        return store.GetSetting(ctx, db, key)
    },
    func(ctx context.Context, key, value string) error {
        return store.SetSetting(ctx, db, key, value)
    },
)
selection := llm.NewSelectionState(dp, selectionStore, &cfg.LLM)
```

On startup, NAVI restores (or initializes) the selection:

```279:291:cmd/navid/main.go
if baseProvider != nil {
    if active, err := selection.GetActive(ctx, catalog); err == nil && active.Provider != "" && active.Model != "" {
        if _, err := selection.SetActive(ctx, catalog, active.Provider, active.Model); err != nil {
            dp.Swap(baseProvider)
        } else {
            // selection restored
        }
    } else {
        dp.Swap(baseProvider)
    }
}
```

All long-lived components receive `dp`:

- NAVI agent (`internal/navi`): `navi.Config.LLM = dp`.
- Orchestrator adapter: `NewLLMDirectiveAdapter(dp, "chat", db, wm, workspaceDir, promptMgr)` (returns `error`; see `cmd/navid/main.go`).
- Coder, Critic, Strategist, Scout workers: `LLM: dp, Model: "chat"`.

When the active selection changes, `dp` updates in place and **all** subsequent LLM calls across the system use the new provider/model.

---

### Prompt-Level API (llm-router Skill)

#### Skill Definition

The runtime switching interface is exposed as an OSS-27 skill in `plugins/llm-router/skills/llm-router/SKILL.yaml`:

```yaml
skill_id: llm-router
display:
  name: LLM Router
  description: Inspect and change NAVI's active LLM provider and model at runtime.
interfaces:
  - name: list
    description: List all configured LLM providers and their available models.
    ...
  - name: get_active
    description: Return the currently active LLM provider and model.
    ...
  - name: set_active
    description: Set the active LLM provider and model for subsequent interactions.
    ...
```

The skill exposes three tools:

- `llm-router_list`
- `llm-router_get_active`
- `llm-router_set_active`

#### Tool Execution in AgentLoop

`internal/navi/loop.go` recognizes these tools and delegates to host-provided callbacks:

```250:269:internal/navi/loop.go
func (l *AgentLoop) executeTool(ctx context.Context, sessionID string, tc llm.ToolCall) skill.DataEnvelope {
    if strings.HasPrefix(tc.Name, "llm-router_") {
        switch {
        case strings.HasSuffix(tc.Name, "_list") && l.cfg.ListLLMs != nil:
            payload, err := l.cfg.ListLLMs(ctx)
            ...
            return skill.SanitizeResult(tc.Name, payload)
        case strings.HasSuffix(tc.Name, "_get_active") && l.cfg.GetActiveLLM != nil:
            provider, model, err := l.cfg.GetActiveLLM(ctx)
            ...
            return skill.SanitizeResult(tc.Name, map[string]any{"provider": provider, "model": model})
        case strings.HasSuffix(tc.Name, "_set_active") && l.cfg.SetActiveLLM != nil:
            provider, _ := tc.Arguments["provider"].(string)
            model, _ := tc.Arguments["model"].(string)
            providerOut, modelOut, err := l.cfg.SetActiveLLM(ctx, provider, model)
            ...
            return skill.SanitizeResult(tc.Name, map[string]any{"provider": providerOut, "model": modelOut})
        }
    }
    ...
}
```

The NAVI host (`cmd/navid`) configures these callbacks via `navi.Config`:

```419:444:cmd/navid/main.go
naviConfig := navi.Config{
    ...
    ListLLMs: func(ctx context.Context) (any, error) {
        currentCatalog := llm.BuildCatalog(&cfg.LLM)
        return currentCatalog, nil
    },
    GetActiveLLM: func(ctx context.Context) (provider, model string, err error) {
        currentCatalog := llm.BuildCatalog(&cfg.LLM)
        active, err := selection.GetActive(ctx, currentCatalog)
        if err != nil { return "", "", err }
        return active.Provider, active.Model, nil
    },
    SetActiveLLM: func(ctx context.Context, provider, model string) (string, string, error) {
        currentCatalog := llm.BuildCatalog(&cfg.LLM)
        active, err := selection.SetActive(ctx, currentCatalog, provider, model)
        if err != nil { return "", "", err }
        return active.Provider, active.Model, nil
    },
}
```

This keeps tool executors thin and defers all routing logic to `LLMCatalog` and `SelectionState`.

---

### Experience Layer Behavior and Prompt Mapping

The supported runtime experience modes are the standard conversational mode and the onboarding experience. Both rely on the same `llm-router_*` tool surface for provider and model questions.

Examples (abridged):

- **Standard NAVI** (`config/personas/navi.yaml` or the built-in default):
  - For “What LLMs do you have access to?” → call `llm-router_list`, group by provider, and present a structured list.
  - For “Which model are you using?” → call `llm-router_get_active` and answer with provider + model.
  - For “Use Anthropic Sonnet” / “Switch to Ollama llama” / “Change model to OpenAI gpt-4”:
    - Normalize provider/model names against the catalog (case-insensitive).
    - Call `llm-router_set_active` with `{provider, model}`.
    - Confirm the new selection in the reply.

- **Onboarding Experience**:
  - Uses the same tools when setup needs authoritative provider/model state, while staying inside the onboarding flow.

Experience modes must **not** guess about provider/model availability; they always defer to `llm-router_*` tools.

#### Provider/Model state grounding (must-ground)

Questions about active provider, active model, configured providers, or available models are **authoritative state queries**. The agent loop enforces this in two ways:

1. **System prompt rule:** A fixed block is injected into every persona’s system prompt: the model MUST call the appropriate llm-router tool (`llm-router_list` or `llm-router_get_active`) before answering; it must never invent or guess a list; it must not claim to have run a tool without an actual tool result in the conversation. If a tool fails, the model must report the error exactly.

2. **Structured reply:** When every tool call in a turn is one of `llm-router_list`, `llm-router_get_active`, or `llm-router_set_active`, the loop does **not** ask the LLM for a follow-up reply. It formats the tool result(s) in Go and sends that as the final reply. So responses about provider/model state are built **only** from successful tool output; on tool failure, the reply is the explicit error message. This makes it impossible for the assistant to claim it ran a tool without an actual execution result or to fabricate a list.

---

### User-Facing Behaviors

#### Model Discovery

Example user prompt:

```text
What LLMs do you have access to?
```

Expected behavior:

- Persona calls `llm-router_list`.
- NAVI returns the current `LLMCatalog` (providers + models). For **Ollama**, when `ollama_url` is configured, the catalog is enriched from the running instance’s `/api/tags` so the list reflects actually-pulled models; otherwise it uses config-only models.
- The reply is built from the tool result only (structured reply path), e.g.:

```text
Available LLMs:

Ollama
- llama3:latest
- mistral:latest

Anthropic
- claude-3.5-sonnet

OpenAI
- gpt-4o-mini
```

#### State Awareness

Example user prompt:

```text
Which model are you using?
```

Expected behavior:

- Persona calls `llm-router_get_active`.
- Answer includes `provider` and `model` from `SelectionState.GetActive`.
- Persona may optionally note whether this is a default or user-selected override.

#### Runtime Switching

Example prompts:

```text
Use Anthropic Sonnet
Switch to Ollama llama3:latest
Change model to OpenAI gpt-4o-mini
```

Expected behavior:

- Persona:
  - Parses provider/model tokens from the request.
  - Normalizes provider key (e.g. `Anthropic` → `anthropic`).
  - Normalizes model name via a case-insensitive match against the provider’s models.
  - Calls `llm-router_set_active` with `{provider, model}`.
- Executor:
  - Validates against `LLMCatalog`:
    - If invalid, returns an error; persona can re-list valid options.
  - Updates settings and `DynamicProvider`.
- Persona:
  - Confirms the new selection, e.g. “Okay, I’ll use Anthropic / claude-3.5-sonnet for future replies.”

All subsequent LLM calls (chat, orchestrator, workers) use the newly selected provider/model.

---

### Validation, Errors, and Extensibility

#### Validation and Error Messages

`SelectionState.SetActive` enforces that:

- The provider exists in `LLMCatalog`.
- The model exists under that provider.

On invalid provider/model combinations it returns a structured error (e.g. `unknown provider/model combination "x" / "y"`). Persona prompts should:

- Surface a human-friendly message.
- Optionally call `llm-router_list` to suggest valid alternatives.

#### Extensibility

The system automatically supports new providers/models as follows:

- Add or update providers/models in `config/runtime.yaml` (or via env + persisted settings).
- `LLMConfig` and `LLMCatalog` pick up the new entries without code changes.
- `llm-router_list` immediately reflects the updated catalog.
- `llm-router_set_active` can target the new entries as soon as they appear in the catalog.

No Go code needs to be modified to:

- Add a new model under an existing provider.
- Add a new provider that maps to an existing `FromConfig`/`SelectionState.buildProviderFor` implementation.

Provider wiring changes (e.g. adding a brand-new backend type) remain an internal Go concern in `internal/llm` and `cmd/navid`; the router and skill interfaces do not change.

---

### Quick Reference

- **Config types**:
  - `internal/config/config.go` — `LLMConfig`, `ProviderConfig`, `ModelRoute`.
- **Runtime catalog**:
  - `internal/llm/catalog.go` — `LLMCatalog`, `LLMProviderInfo`, `LLMModelInfo`.
- **Selection & routing**:
  - `internal/llm/selection.go` — `SelectionState`, `SettingStore`.
  - `internal/llm/dynamic.go` — `DynamicProvider`.
  - `internal/llm/factory.go` — `FromConfig` and base provider chain.
- **Daemon wiring**:
  - `cmd/navid/main.go` — catalog construction, `SelectionState`, `DynamicProvider`, and LLM callbacks for NAVI.
- **Agent runtime**:
  - `internal/navi/config.go` / `internal/navi/navi.go` — wiring of LLM and callbacks into `AgentLoop`.
  - `internal/navi/loop.go` — `llm-router_*` tool handling.
- **Skill surface**:
  - `plugins/llm-router/skills/llm-router/SKILL.yaml` — OSS-27 skill definition for list/get/set.
- **Experience modes**:
  - `config/experience/` — prompt configurations for the standard and onboarding experience modules.

---

### Testing the Runtime LLM Router

This section outlines practical ways to validate that runtime LLM routing behaves as expected.

#### 1. Unit Tests (Go)

- **Catalog tests** (add under `internal/llm/catalog_test.go`):
  - Build `LLMCatalog` from:
    - Legacy-only config (e.g. `OllamaURL` + `OllamaModel`).
    - Explicit `Providers` map with multiple models per provider.
  - Assert:
    - `ListProviders()` returns the expected provider keys.
    - `ListModels("provider")` returns the expected models with no duplicates.
- **Selection tests** (under `internal/llm/selection_test.go`):
  - Use a fake `SettingStore` and a stub `DynamicProvider` that records the last `SwapWithModel` call.
  - Cover:
    - `GetActive` when settings are empty:
      - Returns the first provider/model derived from the catalog.
    - `GetActive` when `llm_provider` / `llm_model` are present:
      - Returns persisted values.
    - `SetActive` with a valid provider/model:
      - Writes both keys to the fake store.
      - Calls `SwapWithModel` with the correct provider + model.
    - `SetActive` with an invalid provider or model:
      - Returns an error and does not call `SwapWithModel`.

#### 2. Daemon-Level Integration Tests

1. **Configure multiple models**
   - In `config/runtime.yaml` (or via env + persisted settings), ensure at least two providers/models are usable (for example, Ollama + Anthropic or multiple Ollama models).
2. **Start NAVI**
   - Run NaviD via Docker Compose (`docker compose -f compose.yml up -d --build`).
3. **Exercise the llm-router tools**
   - Create a NAVI session via HTTP:
     - `POST /api/navi/sessions` → capture `session_id`.
   - Send messages through the NAVI session API:
     - `POST /api/navi/sessions/{id}/message` with:
       - `"What LLMs do you have access to?"`
       - `"Which model are you using?"`
       - `"Use <Provider> <Model>"` where `<Provider>/<Model>` matches your config.
   - Observe behavior using either:
     - `/api/navi/sessions/{id}` — confirm tool calls and replies are recorded.
     - `/ws/live` — watch for `llm-router_list`, `llm-router_get_active`, and `llm-router_set_active` tool events and the resulting replies.
   - Cross-check the gateway-reported LLM status:
     - Use `GetLLMStatus` (wired to `DynamicProvider.NameWithModel`) via any diagnostic endpoint that exposes it (e.g. `/api/health` if integrated) and ensure its value changes after a successful `set_active` call.

#### 3. Manual Chat Flows (End-to-End)

These flows are useful both for development and for operator runbooks.

1. **Baseline**
   - Start NAVI and open your preferred client (CLI, web console, or an OpenAI-compatible client pointed at NAVI).
   - Confirm the initial model using any diagnostic output (logs or a health endpoint that returns `GetLLMStatus`).
2. **Discovery flow**
   - Ask: `What LLMs do you have access to?`
   - Verify:
     - The response lists all providers and models that appear in your config/catalog.
     - No providers/models are reported that are not configured.
3. **State awareness flow**
   - Ask: `Which model are you using?`
   - Verify:
     - The answer matches the current `GetLLMStatus` provider/model string.
4. **Switching flow**
   - Ask: `Use <Provider> <Model>` (for example, `Use Anthropic Sonnet` or `Switch to Ollama llama3:latest`), where the pair is valid in the catalog.
   - Verify:
     - The reply confirms a switch to the requested provider/model.
     - Diagnostic status (`GetLLMStatus`) changes accordingly.
     - Subsequent replies clearly come from the new model (you can also confirm by watching which backend API is hit, if you have outbound logs).
5. **Validation and error handling**
   - Ask for a non-existent combination, such as:
     - `Use Anthropic Opus` when only Sonnet/Haiku are configured.
   - Verify:
     - NAVI responds with an explicit error indicating the provider/model is not configured.
     - The underlying provider/model in diagnostics does **not** change.
     - (Optional) Personas may follow up by re-listing valid options via `llm-router_list`.



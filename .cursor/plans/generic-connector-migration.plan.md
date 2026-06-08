---
name: generic-connector-migration
overview: Fix the Telegram connector stdin race, then complete the migration to a single metadata-driven connector setup flow; remove all hard-coded Telegram paths and make server metadata authoritative.
todos:
  - id: stdin-race
    content: Remove stdin race — runConnectTelegram takes getLine from REPL; no second Scanner
    status: pending
  - id: generic-connect
    content: Add runConnectConnector; remove or reduce runConnectTelegram to trivial wrapper
    status: pending
  - id: init-reuse
    content: navi init Phase 5 uses same generic connector flow
    status: pending
  - id: skill-executor
    content: Replace connector.telegram special case with generic connector.setup branch
    status: pending
  - id: server-metadata
    content: Server-authoritative setup schema; stable response shape; CLI fetches and uses it
    status: pending
  - id: delete-param-map
    content: Delete temporary CLI connector param map after metadata is live
    status: pending
  - id: runbooks
    content: Update runbooks/docs for generic /connect and init flow
    status: pending
isProject: true
---

# Generic connector migration (complete hard-coded Telegram replacement)

## Completion criteria — migration is only done when ALL are true

- No second stdin reader (single reader, getLine from REPL).
- No Telegram-only connect flow (`runConnectTelegram` removed or trivial wrapper).
- No Telegram-only init flow (init uses generic connector step).
- No `connector.telegram` executor special case (generic connector setup branch only).
- No CLI-owned connector param definitions (server metadata is sole source).
- Server metadata drives setup prompts (required/optional params, labels, hints from API).

**Risk:** Stopping after steps 1–4 leaves a half-migrated state: generic-looking CLI but still-hard-coded semantics and duplicate connector knowledge in CLI and server. Do not accept that as done. Step 5 + step 6 (delete param map) are required for completion.

---

## Command contract (locked now)

- **REPL:** `/connect <type>` (e.g. `/connect telegram`). Implementation is **connector configure**, not “Telegram setup.”
- **Structured CLI (later):** `navi connectors configure <type>` if desired; same semantics.

---

## 1. Remove the stdin race (immediate bug fix)

**Goal:** Single reader for stdin so `/connect` prompts receive the next lines reliably.

- **In [cmd/navi/main.go](cmd/navi/main.go):**
  - Change `runConnectTelegram(ctx, cfg)` to accept `getLine func(prompt string) string`.
  - In `runREPL`, define `getLine := func(prompt string) string { fmt.Print(prompt); return strings.TrimSpace(<-inputCh) }` and pass it when invoking the connect flow.
  - Remove the internal `bufio.Scanner` and `readLine` from the connect flow; use `getLine(...)` for each prompt.

**Result:** Next lines after prompts are consumed only by the connect flow; no race with the REPL.

---

## 2. Replace `/connect telegram` with generic connector configuration (bridge)

**Goal:** One code path for connector setup; prompts driven by type. **Step 2 is a bridge, not the destination.**

- **Add `runConnectConnector(ctx, cfg, connectorType string, getLine func(string) string)`:**
  - Uses a **temporary local param map** keyed by connector type (e.g. `telegram` → required/optional params and labels) so the CLI can prompt and build `params map[string]string`.
  - Prints a generic header (e.g. "Connector setup — ") and "Credentials are entered here and never sent to the LLM."
  - For each required/optional param, calls `getLine(label)` and sets `params[key]`.
  - POSTs `{"type": connectorType, "params": params}` to `POST /api/setup/connector`.
- `**runConnectTelegram` SHALL be removed or reduced to a trivial wrapper** after `runConnectConnector` exists. No long-lived parallel implementation. Prefer: delete `runConnectTelegram` and have the `/connect telegram` branch call `runConnectConnector(ctx, cfg, "telegram", getLine)`.
- **REPL:** `/connect telegram` or `/connect` (then prompt "Connector type?" and read one line); resolve type and call `runConnectConnector(ctx, cfg, type, getLine)`.

**Result:** Generic entry point; temporary local param map only until step 5. CLI-side connector param maps **SHALL be removed after migration** (step 6).

---

## 3. Update navi init to use the same generic connector setup path

**Goal:** No separate Telegram-only onboarding.

- **In [cmd/navi/main.go](cmd/navi/main.go) init Phase 5:** Replace the Telegram block with a single generic step.
  - Prompt, e.g. "Add a connector? [telegram / slack / none] (Enter for none):". If user chooses a type, call **the same** `runConnectConnector(ctx, cfg, type, getLine)` with init’s line reader (e.g. init’s existing `prompt()`/scanner).
  - Remove all Telegram-specific strings from init; connector-specific text comes only from the generic param map (or server metadata in step 5).

**Result:** One shared connector-setup implementation for both `/connect` and `navi init`.

---

## 4. Remove the connector.telegram special case from the skill executor

**Goal:** Generic guidance for any connector setup skill.

- **In [internal/navi/skill/executor.go](internal/navi/skill/executor.go):** Replace the `connector.telegram` + `setup` case.
  - Use a convention for now: e.g. `strings.HasPrefix(entry.Spec.SkillID, "connector.") && iface.Name == "setup"`. Derive type from skill ID (e.g. `connector.telegram` → `telegram`).
  - Return a **generic** message: "To connect this connector securely, run /connect  in the CLI so credentials stay out of chat history." If `redirect` is present and not `cli`, return "Connector setup only supports redirect=cli. Run /connect ."
- **Long term:** Connector setup should be described in metadata/capability semantics, not inferred from string prefixes. For this migration, convention is acceptable; replace with explicit metadata later.

**Result:** No Telegram-only branch in the executor.

---

## 5. Make connector metadata authoritative (stable schema)

**Goal:** Required/optional params, labels, descriptions, and validation hints come from the server. **Connector setup schema returned by the server is the sole authoritative source for required/optional params and labels.** CLI-side connector param maps SHALL be removed after migration (step 6).

- **Stable response shape** — do not return ad hoc blobs. Lock the schema, e.g.:
  - **Per connector type:** `type`, `display_name`, `required_params`, `optional_params`, `setup_hint`.
  - **Per param:** `key`, `label`, `description`, `secret`, optionally `placeholder`, optionally `validation_hint`.
- **Server:** Define setup descriptors (e.g. in [internal/connectors](internal/connectors) or navid); register per type. Expose via extended `GET /api/connectors/factories` or `GET /api/connectors/setup-schema` returning the above structure.
- **Validation lives on the server.** The CLI can do convenience checks, but the server is authoritative: reject missing required params, malformed params where rules exist, and unsupported types. Otherwise alternate clients will drift.
- **CLI and init:** Fetch setup schema from the API; drive prompts and param collection from it. Remove the temporary local param map (step 6).

**Result:** Connector types and setup params defined once on the server; CLI and init are metadata-driven.

---

## 6. Delete temporary CLI param map

- Remove the local connector param map from the CLI used in step 2. All param definitions and labels come from the server (step 5).

---

## 7. Update runbooks/docs

- Update [docs/runbooks/connector-setup-telegram.md](docs/runbooks/connector-setup-telegram.md) (or equivalent) to describe generic `/connect <type>` and init flow; remove implication that Telegram is special.

---

## Follow-up (non-blocking for this migration)

- **Secret params:** Support hidden input where practical (e.g. `bot_token` not echoed in terminal history). Put on follow-up list.

---

## Implementation order

1. Stdin race fix (getLine, single reader).
2. Generic `runConnectConnector`; remove or trivial-wrap `runConnectTelegram`.
3. Init uses same generic flow.
4. Generic skill-executor connector setup branch.
5. Server-authoritative setup schema (stable shape); API; CLI/init consume it.
6. Delete temporary CLI param map.
7. Update runbooks/docs.

---

## Files to touch (summary)


| Step | Files                                                                                                                                                                                                                                                                                                                      |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | [cmd/navi/main.go](cmd/navi/main.go) (connect flow signature + getLine; REPL call site)                                                                                                                                                                                                                                    |
| 2    | [cmd/navi/main.go](cmd/navi/main.go) (runConnectConnector, temporary param map, /connect branch; remove or wrap runConnectTelegram)                                                                                                                                                                                        |
| 3    | [cmd/navi/main.go](cmd/navi/main.go) (init Phase 5: connector choice + runConnectConnector)                                                                                                                                                                                                                                |
| 4    | [internal/navi/skill/executor.go](internal/navi/skill/executor.go) (generic connector.setup case)                                                                                                                                                                                                                          |
| 5    | [internal/connectors](internal/connectors) or navid (setup descriptor type + registration), [internal/gateway/server.go](internal/gateway/server.go) (extend factories or add setup-schema endpoint), [cmd/navi/main.go](cmd/navi/main.go) (fetch metadata), [cmd/navid/main.go](cmd/navid/main.go) (register descriptors) |
| 6    | [cmd/navi/main.go](cmd/navi/main.go) (remove local param map)                                                                                                                                                                                                                                                              |
| 7    | Runbooks (e.g. [docs/runbooks/connector-setup-telegram.md](docs/runbooks/connector-setup-telegram.md))                                                                                                                                                                                                                     |



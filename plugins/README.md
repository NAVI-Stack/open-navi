# NAVI Plugins

`plugins/` is the canonical package boundary for repo-owned installable capability implementations.

A plugin may own skills, connectors, policies, provider implementations, runtime helpers, docs, and tests. Core framework code stays outside plugin packages.

```text
plugins/<plugin-name>/
  plugin.yaml
  README.md
  skills/
  connectors/
  policies/
  providers/
  runtimes/
  docs/
  tests/
```

## Manifest Rules

`plugin.yaml` is the canonical manifest for repo-owned plugins. `plugin.json` is retained only for legacy external compatibility and fixtures.

Accepted `kind` values are:

- `llm-provider`
- `integration`
- `workflow`
- `domain`
- `agentic`
- `sensory`
- `system`

Plugin-owned skills are not discovered by raw folder scanning. The plugin loader validates `plugin.yaml`, checks the enabled/active state, and exposes only the skill paths declared in `components.skills`.

## Boundaries

- `plugins/<name>/` owns concrete capability implementations.
- `skills/` is skill-system support only; repo-owned capability skills live under plugins.
- `connectors/` contains shared public connector interfaces and types only.
- `internal/connectors` contains connector framework/runtime code only.
- `internal/navi/skill` contains skill framework/runtime code only.
- `internal/navi/plugin` contains plugin manifest, registry, lifecycle, and loader code only.
- `internal/llm` contains LLM interfaces, routing, catalog, control-plane, and registry abstractions only.

Concrete LLM providers live in `plugins/llm-*` packages and register through the provider registry. Internal packages must not import concrete plugin implementations.

## Bootstrap

Built-in plugins are linked into `navid` through one intentional import boundary:

```go
// cmd/navid/plugin_bootstrap.go
package main

import (
    _ "github.com/ceoai/navi/plugins/llm-anthropic/providers/anthropic"
    _ "github.com/ceoai/navi/plugins/llm-openai/providers/openai"
    _ "github.com/ceoai/navi/plugins/llm-ollama/providers/ollama"
    "github.com/ceoai/navi/plugins/slack/connectors/slack"
    "github.com/ceoai/navi/plugins/telegram/connectors/telegram"
)
```

Keep concrete plugin imports out of `internal/*`. If a new built-in plugin must be linked into the binary, add it to the bootstrap boundary and register it through framework registries.

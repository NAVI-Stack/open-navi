# NAVI Plugin Template

This is a copyable scaffold for a NAVI capability package.

## Design rule

A plugin is the lifecycle, permission, enablement, and packaging boundary.
Skills, connectors, policies, and runtimes remain distinct typed subcomponents inside the plugin.

## Suggested folder shape

```txt
plugins/<plugin-name>/
  plugin.yaml
  README.md
  skills/
    <skill-name>/SKILL.yaml
  connectors/
    <connector-name>/
  policies/
  runtimes/
  docs/
  tests/
```

## What belongs here

- Capabilities owned by this installable plugin.
- Skills exposed to NAVI.
- Connectors this plugin owns or adapts.
- Plugin-scoped policies and rate limits.
- Runtime workers needed by this plugin.
- Tests proving the plugin manifest and skill contracts are valid.

## What does not belong here

- Core skill registry code.
- Core connector interfaces.
- Cognitive-layer decision logic.
- World Model mutation logic.
- Global governance implementation.

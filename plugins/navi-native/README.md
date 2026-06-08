# NAVI Native

Agentic integration for native filesystem and OS dialog capabilities bridged through the PET desktop shell.

## Design rule

A plugin is the lifecycle, permission, enablement, and packaging boundary.
Skills, connectors, policies, and runtimes remain distinct typed subcomponents inside the plugin.

## Suggested folder shape

```txt
plugins/navi-native/
  plugin.yaml
  README.md
  skills/
    native-file-read/SKILL.yaml
    native-file-write/SKILL.yaml
    native-list-dir/SKILL.yaml
    native-folder-pick/SKILL.yaml
  connectors/
    petbridge/
  policies/
  runtimes/
  docs/
  tests/
```

## What belongs here

- Native filesystem capabilities bridged through PET desktop.
- Skills exposed to NAVI for host file read, write, listing, and OS dialogs.
- The PET bridge connector that relays requests to the Tauri desktop shell.
- Plugin-scoped policies and rate limits for filesystem operations.
- Tests proving the plugin manifest and skill contracts are valid.

## What does not belong here

- Core skill registry code.
- Core connector interfaces.
- Cognitive-layer decision logic.
- World Model mutation logic.
- Global governance implementation.
- Tauri plugin Rust code (that lives in `projects/pet/apps/pet-desktop/src-tauri/`).

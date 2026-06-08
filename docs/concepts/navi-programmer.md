# NAVI Programmer

Moved.

The source of truth now lives in the first-party NAVI Programmer plugin package:

- [plugins/navi-programmer/docs/concepts/navi-programmer.md](../../plugins/navi-programmer/docs/concepts/navi-programmer.md)
- [plugins/navi-programmer/PROJECT_STATE.md](../../plugins/navi-programmer/PROJECT_STATE.md) — current implementation state
- [plugins/navi-programmer/plugin.yaml](../../plugins/navi-programmer/plugin.yaml) — manifest

NAVI core should keep only integration-facing notes for how the core runtime
discovers, governs, and invokes external plugins. Do not duplicate programmer
implementation docs back into core; link to the plugin package instead.

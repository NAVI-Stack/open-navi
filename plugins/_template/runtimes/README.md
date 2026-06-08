# Runtime Notes

Use this folder only when the plugin needs an owned runtime, subprocess, MCP server, or isolated worker.

Most integration plugins should start with connectors + skills only.

Rules:

- Runtime processes must not write directly to NAVI's World Model.
- Runtime crashes must return structured failure results through the plugin boundary.
- Network and filesystem access must match plugin.yaml and skill security metadata.
- Secrets must come from NAVI's secret/config broker, not checked-in files.

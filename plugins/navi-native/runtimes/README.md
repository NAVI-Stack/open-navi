# Runtime Notes

This plugin uses a **bridge** runtime model.

The PET desktop shell (Tauri v2) is the runtime — it is not a subprocess owned by this plugin.
NaviD communicates with PET through the bridge connector over WebSocket/IPC.

The bridge is established when the PET desktop shell connects to the NaviD gateway.
When PET is not running, all native skills report `bridge_unavailable` and the plugin is marked degraded.

Rules:

- Runtime processes must not write directly to NAVI's World Model.
- Runtime crashes must return structured failure results through the plugin boundary.
- Network and filesystem access must match plugin.yaml and skill security metadata.
- Secrets must come from NAVI's secret/config broker, not checked-in files.
- The bridge relay must validate all requests against the active workspace boundary policy.

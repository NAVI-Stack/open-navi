# Generic Subprocess Skill Runtime

**Status:** Active  
**Applies to:** skills using `transport.type: subprocess`  
**Source of truth:** `internal/navi/skill/spec.go`, `internal/navi/skill/subprocess_runtime.go`

This transport lets NAVI call a worker process without embedding the worker's language runtime into the Go daemon.

The control rule is:

```text
Go invokes worker -> worker performs task -> worker returns structured result -> Go normalizes and records it
```

## Spec Shape

Use `subprocess` when a skill should be backed by a long-lived language ecosystem or external coding harness, but NAVI should still own governance, selection, and result normalization.

```yaml
interfaces:
  - name: "run_tests"
    transport:
      type: "subprocess"
      runtime: "python"
      command: ["python", "-m", "navi_coder_worker"]
      protocol: "jsonrpc_stdio"
      sandbox_profile: "local-coder-python-network-disabled"
```

Required transport fields:

| Field | Purpose |
|-------|---------|
| `runtime` | Advisory runtime label such as `python`, `node`, `shell`, or `custom`. |
| `command` | Process argv launched from the skill directory. |
| `protocol` | Wire protocol. Defaults to `jsonrpc_stdio`; this is currently the only supported value. |
| `sandbox_profile` | Optional sandbox profile ID used when `security.sandbox.required` is true. |

When `security.sandbox.required` is true, NAVI resolves the sandbox profile, mounts the workspace at the profile mount target, mounts the skill directory read-only at `/navi/skill`, sends the JSON-RPC request on stdin, and normalizes the worker response. The older `subprocess_python` transport remains supported for plugin skills that need NAVI-managed Python virtualenv provisioning and the existing Python `SkillResult` wrapper.

## JSON-RPC Stdio Contract

NAVI sends one JSON-RPC request to stdin:

```json
{
  "jsonrpc": "2.0",
  "id": "invocation-uuid",
  "method": "invoke",
  "params": {
    "skill_id": "navi.coder.repo",
    "interface": "run_tests",
    "workspace": "/workspace/repo",
    "input": {},
    "policy": {
      "network": "none",
      "filesystem": "workspace_ro",
      "secrets": []
    }
  }
}
```

The worker writes either one JSON-RPC response object or newline-delimited event objects followed by the response:

```json
{"type":"log","stream":"stdout","data":"running tests"}
{"jsonrpc":"2.0","id":"invocation-uuid","result":{"status":"success","output":{"summary":"tests passed"},"metrics":{"duration_ms":1200}}}
```

The executor normalizes the response into `SkillExecutionResult`, preserving worker statuses such as `success`, `failed`, `partial`, `timeout`, `killed`, and `policy_blocked`.

## Boundary

This transport is for protocol-stable worker processes, not arbitrary prompt shell-outs. The Cognitive Layer sees compact skill interfaces. Go validates the skill, derives the policy envelope, launches the worker, enforces timeouts and output caps, and normalizes the result.

Workers must not own NAVI governance, mutate NAVI internal state directly, or receive long-lived credentials by default.

## Related Docs

- [skills.md](skills.md)
- [skill-python-runtime.md](skill-python-runtime.md)
- [../canonical/skills.md](../canonical/skills.md)

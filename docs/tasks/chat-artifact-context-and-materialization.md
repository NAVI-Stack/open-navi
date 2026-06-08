# Chat Artifact Context And Materialization

**Status:** Implemented baseline
**Observed:** 2026-06-05
**Scope:** Chat runtime context, artifact skill surface, and durable output promotion

## Observed Issue

In chat `18d3bc90-65c0-4bb9-a1ce-59a15594e7be`, NAVI persisted the transcript but failed to resolve the follow-up request "Save it as an artifact" to the immediately previous assistant reply. The previous reply was a complete HTML/CSS/JS work product for a time/date-driven glowing star.

Evidence from local API inspection:

- `GET /api/navi/chats/18d3bc90-65c0-4bb9-a1ce-59a15594e7be` showed the prior assistant HTML/code reply was present in the durable transcript.
- `GET /api/artifacts` returned no artifacts.
- `GET /api/navi/chats/18d3bc90-65c0-4bb9-a1ce-59a15594e7be/runtime_summary` showed `artifact_ids: []`, `main_artifact_id: ""`, and `tool_calls: 0`.

## Expected Behavior

NAVI should preserve normal conversational flow while treating obvious durable work products as artifacts.

- If the user explicitly asks to save/promote "it", "that", "the previous reply", or similar as an artifact, NAVI resolves the latest suitable assistant work product in the current chat and creates an artifact linked to the chat, run, and source message.
- If NAVI generates a substantial durable payload such as HTML, code, Markdown, a table, or a diagram, it should use `skill.core-artifact.create` when available.
- If the model returns a substantial durable payload as plain Markdown without calling the artifact tool, the runtime should auto-materialize it after reply finalization.
- Short conversational replies should remain chat-only.
- Ambiguous save requests with no suitable prior assistant work product should ask one concise clarification and create no artifact.

## Implemented Baseline

- Added active `core-artifact` plugin and skill manifests:
  - `plugins/core-artifact/plugin.yaml`
  - `plugins/core-artifact/skills/core-artifact/SKILL.yaml`
- Surfaced model-callable artifact interfaces:
  - `skill.core-artifact.create`
  - `skill.core-artifact.update`
  - `skill.core-artifact.list`
  - `skill.core-artifact.search`
  - `skill.core-artifact.update_lifecycle`
- Added deterministic chat-runtime promotion for explicit "save it as an artifact" requests.
- Added auto-materialization for substantial plain chat replies when the model does not call the artifact tool.
- Persisted run artifact IDs on completion and mirrored them into assistant message metadata.
- Added prompt/runtime guidance that durable generated work products should use the artifact capability when surfaced.

## Acceptance Criteria

- A second-turn request "Save it as an artifact" compiles with the prior assistant HTML/code message present in the conversation context.
- A chat with a prior assistant HTML code block creates one artifact when the user sends "Save it as an artifact".
- The created artifact links back to the run/chat/source message through artifact references and run metadata.
- A substantial generated HTML/code reply creates an artifact even if the model returns plain Markdown without a tool call.
- Short chat replies do not create artifacts.
- Ambiguous "save it" requests with no prior assistant work product ask for clarification and create no artifact.

## Follow-Up Checklist

- Expose artifact reference UI affordances in the console chat transcript when assistant message metadata includes `mainArtifactId` or `artifactIds`.
- Add retrieval-oriented UX copy for artifact references if the console needs a friendly title/link renderer.
- Consider adding a first-class `skill.core-artifact.promote_message` interface if future model-driven promotion needs to choose a non-latest message.
- Re-run the original chat scenario against a live NaviD instance and confirm `/api/artifacts` and runtime summary now show the promoted artifact.

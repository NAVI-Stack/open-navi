**Status:** Active
**Last Updated:** 2026-04-28
**Updated By:** NAVI Programmer migration

# Concepts - Index

Domain concepts: orchestration, memory, workers, skills, autonomy, and related flows.

These documents mix current implementation explanation with broader system concepts. For code-backed operational truth, prefer [../architecture/README.md](../architecture/README.md), [../specs/INDEX.md](../specs/INDEX.md), and [../runbooks/INDEX.md](../runbooks/INDEX.md) first.

---

| Document | Purpose |
|----------|---------|
| [api-auth.md](api-auth.md) | Current gateway auth model: loopback, API keys, onboarding/setup exceptions, connector compatibility. |
| [autonomy-dimensions.md](autonomy-dimensions.md) | Four autonomy dimensions and preset mapping to config. |
| [conscious-loop-steps.md](conscious-loop-steps.md) | Conscious process step mapping to implementation. |
| [content-trust.md](content-trust.md) | Content trust model and verification. |
| [conversation-compaction.md](conversation-compaction.md) | Conversation compaction and context-window management. |
| [degradation-visibility.md](degradation-visibility.md) | Failure visibility levels and UX. |
| [memory.md](memory.md) | Memory and World Model: entity-based persistence, relationship layer, provenance. |
| [navi-programmer.md](navi-programmer.md) | Moved pointer for NAVI Programmer; source now lives in `projects/navi-plugins/navi-programmer/`. |
| [onboarding-flow.md](onboarding-flow.md) | How a fresh instance is claimed, configured, and activated. |
| [orchestration-loop.md](orchestration-loop.md) | Orchestrator loop and directive decomposition. |
| [runtime-llm-routing.md](runtime-llm-routing.md) | Runtime-switchable LLM routing and provider selection. |
| [skills.md](skills.md) | Skills system, discovery, and lifecycle. |
| [skill-python-runtime.md](skill-python-runtime.md) | Python skill runtime and metadata schema. |
| [skill-subprocess-runtime.md](skill-subprocess-runtime.md) | Generic JSON-RPC stdio subprocess worker contract. |
| [workers.md](workers.md) | Capability Layer, Command Model, Connectors, Plugins, and worker agents. |
| [onboarding-experience.md](onboarding-experience.md) | Onboarding Experience for setup and configuration. |
| [wizard-config-flows.md](wizard-config-flows.md) | Wizard-driven configuration flows beyond first-time onboarding. |

---

[docs INDEX](../INDEX.md) - [design](../design/INDEX.md) - [_meta/lifecycle-rules](../_meta/lifecycle-rules.md)

# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability within NAVI, please send an e-mail to the maintainers. All security vulnerabilities will be promptly addressed.

## Supported Versions

Only the latest version of NAVI receives security updates.

---

## AI-Specific Secret Handling

NAVI treats secrets and other sensitive data as **non-displayable** in normal conversational flows, even when the LLM is prompted or manipulated to reveal them.

High-level rules:

- **No raw secrets in responses**  
  - API keys, access tokens, owner secrets, connector credentials, and similar values must never be returned verbatim in chat or HTTP responses, regardless of user phrasing or external content instructions (prompt injection, jailbreak attempts, etc.).

- **Classification of sensitive data**  
  - Configuration entries and Artifacts that contain secrets or high-sensitivity information should be classified accordingly at the data-model level (e.g., secret vs. non-secret fields).
  - Provenance records should preserve **that something sensitive exists** without storing or re-surfacing raw secret values in free text.

- **Redaction and structured exposure**  
  - When debugging or explaining configuration, NAVI may surface:
    - Redacted forms (e.g., `sk_****abcd`).
    - Structured metadata (e.g., “OpenAI API key is configured”, “Telegram connector token present”).
  - It must not expose full tokens, passwords, or private keys.

- **Untrusted surfaces and exfiltration**  
  - Outputs routed to untrusted surfaces (web pages, third-party chat participants, logs intended for external systems) must be treated as potential exfiltration channels.
  - Governance should block any action that would serialize or transmit secret-classified values to `external_untrusted` destinations.

These rules complement the general Governance and Content Trust models: even if the LLM is manipulated by adversarial prompts, technical guardrails must prevent raw secret exfiltration.

# Contributing to NAVI

Thank you for your interest in contributing to the NAVI project!

## How to Contribute

1.  **Report Bugs**: Use GitHub Issues to report bugs.
2.  **Suggest Features**: Use GitHub Issues for feature requests.
3.  **Submit Pull Requests**: Ensure your code follows the project's style and includes tests.

## Running tests

- **Default (no Windows firewall prompts):**  
  `go test ./...`  
  This skips tests that start a real NATS server (JetStream). Use this for day-to-day runs so Windows does not prompt for network access.

- **Full coverage (including JetStream bus):**  
  `go test -tags=jetstream ./...`  
  Runs JetStream tests that start an in-process NATS server. On Windows, the first run may trigger a "Allow app to communicate?" prompt; allow it once so later runs proceed without prompting.

- **E2E:** The `test/e2e` package skips if the gateway is not reachable. Start the server first if you want those tests to run.

## Development Workflow

1.  Fork the repository.
2.  Create a feature branch.
3.  Commit your changes.
4.  Push to your fork and submit a PR.

## Workflow Surface Guidance

- Prefer reusable workflow contributions in `skills/` before adding one-off command shims or harness-specific compatibility files.
- When importing ideas from external agent bundles, port them manually into NAVI-native docs, runbooks, skills, or templates instead of copying the source surface wholesale.

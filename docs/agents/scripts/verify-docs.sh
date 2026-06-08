#!/usr/bin/env bash
# Verification gate for agent workflow (Phase 0).
# Exit 0 only if all required agent docs exist and are non-empty.
# Run from the repository root.

set -e
# Script lives in docs/agents/scripts/; repo root is three levels up.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

FAIL=0
check() {
  if [[ ! -f "$1" ]]; then
    echo "Missing: $1"
    FAIL=1
  elif [[ ! -s "$1" ]]; then
    echo "Empty: $1"
    FAIL=1
  fi
}

check PROJECT_STATE.md
check NEXT_ACTION.md
check TASK_QUEUE.md
check docs/agents/state.md
check docs/agents/INDEX.md
check docs/agents/design.md

if [[ $FAIL -eq 1 ]]; then
  exit 1
fi
exit 0

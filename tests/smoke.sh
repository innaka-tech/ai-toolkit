#!/usr/bin/env bash
# Smoke test for the core v1 workflow: init -> start -> preflight -> exec -> close.
# Runs in a throwaway git repo with an isolated HOME so it never touches real state.
# Usage: tests/smoke.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/aitk-smoke.XXXXXX")"
export HOME="$WORK/home"
export PATH="$ROOT/scripts/v1:$PATH"
export UTEKE_DISABLED=1
mkdir -p "$HOME"

pass=0
fail=0
ok() { pass=$((pass + 1)); echo "ok   - $1"; }
not_ok() { fail=$((fail + 1)); echo "FAIL - $1"; }
check() { if eval "$2" >/dev/null 2>&1; then ok "$1"; else not_ok "$1"; fi; }

PROJ="$WORK/proj"
mkdir -p "$PROJ"
cd "$PROJ"
git init -q -b main
git config user.email smoke@example.com
git config user.name smoke
echo "hello" > README.md
git add README.md
git commit -q -m "chore: init"

check "help exits 0" "ai-toolkit --help"
check "unknown command exits 2" "ai-toolkit nope; test \$? -eq 2"
check "init succeeds" "ai-toolkit init ."
check "AGENTS.md created" "[ -f AGENTS.md ]"
check "ai-state.json is valid JSON" "jq -e . ai-state.json"
check "docs/ai/current-task.md created" "[ -f docs/ai/current-task.md ]"
check "preflight fails without active task" "! ai-preflight ."
check "start succeeds" "ai-start 'T-001: smoke task'"
check "preflight passes with active task" "ai-preflight ."
check "exec without provider runs preflight only" "out=\$(ai-exec auto .) && printf '%s' \"\$out\" | grep 'Preflight passed'"
check "exec rejects unknown provider" "! ai-exec not-a-tool ."
check "close without --knowledge is rejected" "! ai-close --summary 'did it'"
check "close with knowledge succeeds" "ai-close --summary 'did it' --knowledge 'smoke finding'"
check "knowledge recorded" "grep -q 'smoke finding' docs/ai/knowledge.md"
check "handoff recorded" "grep -q 'did it' docs/ai/handoff.md"
check "ai-state.json still valid JSON" "jq -e . ai-state.json"
check "removed Fusion commands are absent" "[ ! -e '$ROOT/scripts/v1/ai-handoff' ] && [ ! -e '$ROOT/scripts/v1/ai-agent-report' ]"

echo
echo "passed: $pass  failed: $fail  (workdir: $WORK)"
[ "$fail" -eq 0 ]

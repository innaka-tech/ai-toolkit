#!/usr/bin/env bash
# Tests the ai-* compatibility shims: v1 projects run the frozen v1 scripts,
# v2 projects and new repositories forward to aitk (with flags translated).
# Usage: AITK=/path/to/aitk tests/shims.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/aitk-shims.XXXXXX")"
BIN="$WORK/bin"
mkdir -p "$BIN" "$WORK/home"
cp "${AITK:?set AITK to the aitk binary}" "$BIN/aitk"
export HOME="$WORK/home" PATH="$ROOT/scripts:$BIN:$PATH" AITK_TOOL=claude-code UTEKE_DISABLED=1 AI_TOOLKIT_ROOT="$ROOT"

pass=0; fail=0
ok() { pass=$((pass + 1)); echo "ok   - $1"; }
not_ok() { fail=$((fail + 1)); echo "FAIL - $1"; }
check() { if eval "$2" >/dev/null 2>&1; then ok "$1"; else not_ok "$1"; fi; }
# has <pattern> <command...>: run the command fully, then grep its output (no SIGPIPE under pipefail)
has() { local pat=$1 out; shift; out=$("$@" 2>&1) || true; grep -q -- "$pat" <<<"$out"; }

repo() {
  mkdir -p "$1" && cd "$1"
  git init -q -b main && git config user.email t@e.st && git config user.name t
}

# v1 project: frozen scripts run
repo "$WORK/v1"
cp -R "$ROOT/tests/fixtures/v1-project/." .
git add -A && git commit -qm "chore: v1"
check "v1: ai-resume runs the v1 script" "has '^Project:' ai-resume"
check "v1: ai-toolkit dispatcher is v1" "has 'ai-toolkit <command>' ai-toolkit --help"
check "v1: aitk is not invoked (no aitk.toml created)" "[ ! -f aitk.toml ]"

# new repository: forwards to aitk
repo "$WORK/v2"
echo hi > README.md && git add -A && git commit -qm "chore: init"
check "v2: ai-init runs aitk init" "ai-init && [ -f aitk.toml ]"
check "v2: ai-start creates and starts an aitk task" "ai-start 'Add greeting' --goal G-001 --sub-goal X && aitk --json task list | grep -q '\"status\": \"in_progress\"'"
check "v2: ai-resume shows the aitk brief" "has '^# Brief:' ai-resume"
check "v2: ai-close enforces criteria written on the task" "aitk task update --add-ac 'greeting shown' && has 'E_DOD_ACCEPTANCE' ai-close --summary 'Added greeting' --knowledge none"
check "v2: ai-close maps --status COMPLETED to a normal close" "aitk task update --ac-done 1 && has '→ done' ai-close --summary 'Added greeting' --knowledge none --status COMPLETED"
check "v2: ai-toolkit doctor forwards" "has 'errors' ai-toolkit doctor"
check "v2: v1-only command still runs v1 (ai-status)" "has 'Provider Status' ai-status"

# v1 argument shapes in a v2 project
git init -q --bare "$WORK/remote.git" && git remote add origin "$WORK/remote.git"
echo more > more.txt
check "v2: ai-commit takes the message, a path, and --push" "ai-commit 'docs: more' . --push && [ \"\$(git log -1 --format=%s)\" = 'docs: more' ] && git -C '$WORK/remote.git' log -1 --format=%s main | grep -q 'docs: more'"
check "v2: ai-commit rejects unknown options instead of committing them" "echo x > y.txt && ! ai-commit 'docs: y' --nope && [ \"\$(git log -1 --format=%s)\" = 'docs: more' ]"
check "v2: ai-commit with nothing to commit succeeds like v1" "git add -A && git commit -qm 'docs: wip' && out=\$(ai-commit 'docs: nothing') && grep -q 'Nothing to commit' <<<\"\$out\""
repo "$WORK/v2b"
echo hi > README.md && git add -A && git commit -qm "chore: init"
cd "$WORK"
check "v2: ai-init <dir> initialises that directory" "ai-init '$WORK/v2b' && [ -f '$WORK/v2b/aitk.toml' ]"

echo
echo "passed: $pass  failed: $fail  (workdir: $WORK)"
[ "$fail" -eq 0 ]

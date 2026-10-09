# shellcheck shell=sh
# Shared logic for the ai-* compatibility shims (ADR-0001).
# In a v2 project (aitk.toml) or a new repository, ai-* commands forward to `aitk`.
# In a v1 project (not migrated yet), the frozen v1 scripts in scripts/v1/ run unchanged.

SHIM_DIR=$(cd "$(dirname "$0")" && pwd)
V1_DIR="$SHIM_DIR/v1"

shim_root() {
  git rev-parse --show-toplevel 2>/dev/null || pwd
}

# shim_is_v1: true when the current project is a v1 project.
shim_is_v1() {
  root=$(shim_root)
  [ -f "$root/aitk.toml" ] && return 1
  [ -f "$root/.ai-toolkit/project.env" ] && return 0
  if [ -f "$root/ai-state.json" ] && ! grep -q '"schema_version"' "$root/ai-state.json"; then
    return 0
  fi
  return 1
}

shim_v1() { # shim_v1 <script> [args...]
  name=$1
  shift
  exec "$V1_DIR/$name" "$@"
}

shim_aitk() { # shim_aitk [aitk args...]
  if ! command -v aitk >/dev/null 2>&1; then
    echo "aitk is not installed. Install: curl -fsSL https://raw.githubusercontent.com/innaka-tech/ai-toolkit/main/install.sh | sh" >&2
    exit 127
  fi
  exec aitk "$@"
}

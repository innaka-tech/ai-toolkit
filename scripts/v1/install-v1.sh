#!/usr/bin/env bash
set -euo pipefail

# Portable installer for macOS, Linux, WSL, and Git Bash.
# Usage from a cloned checkout: ./install.sh
# Usage from elsewhere:       curl .../install.sh | bash (when published)

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${AI_TOOLKIT_DIR:-$HOME/.ai-toolkit}"
REPO_URL="${AI_TOOLKIT_REPO:-https://github.com/innaka-tech/ai-toolkit.git}"

if [ "$SOURCE_DIR" != "$INSTALL_DIR" ]; then
  if [ -d "$INSTALL_DIR/.git" ]; then
    echo "Updating existing toolkit at $INSTALL_DIR"
    git -C "$INSTALL_DIR" pull --ff-only
  elif [ -e "$INSTALL_DIR" ] && [ "$(find "$INSTALL_DIR" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null)" ]; then
    echo "Install directory is not empty: $INSTALL_DIR" >&2
    echo "Set AI_TOOLKIT_DIR to a new path or move the existing directory." >&2
    exit 1
  else
    echo "Cloning toolkit into $INSTALL_DIR"
    mkdir -p "$(dirname "$INSTALL_DIR")"
    git clone "$REPO_URL" "$INSTALL_DIR"
  fi
fi

chmod +x "$INSTALL_DIR/scripts"/* 2>/dev/null || true

SHELL_NAME="$(basename "${SHELL:-bash}")"
case "$SHELL_NAME" in
  zsh) RC_FILE="${ZDOTDIR:-$HOME}/.zshrc" ;;
  fish) RC_FILE="$HOME/.config/fish/config.fish" ;;
  *) RC_FILE="$HOME/.bashrc" ;;
esac

mkdir -p "$(dirname "$RC_FILE")"
PATH_LINE="export PATH=\"$INSTALL_DIR/scripts:\$PATH\""
if [ "$SHELL_NAME" = "fish" ]; then
  PATH_LINE="set -gx PATH $INSTALL_DIR/scripts \$PATH"
fi

if ! grep -Fq "$INSTALL_DIR/scripts" "$RC_FILE" 2>/dev/null; then
  {
    printf '\n# ai-toolkit\n'
    printf '%s\n' "$PATH_LINE"
  } >> "$RC_FILE"
fi

"$INSTALL_DIR/scripts/ai-aliases" install >/dev/null 2>&1 || true

echo "AI Toolkit installed at $INSTALL_DIR"
echo "Reload your shell: source $RC_FILE"
echo "Then run: ai-toolkit --help"

# AI Toolkit Directory

Central hub for multi-AI orchestration on this MacBook.

## Structure
- `config.yaml` — Master configuration
- `context/` — Registry for tracked projects
- `bridge/` — Inter-agent handoff queue
- `state/` — Provider state (accounts, status)
- `logs/` — Usage logs
- `scripts/` — `ai-*` command wrappers
- `conventions/` — Shared conventions

## Project Protocol

Every project should keep shared context inside the repository:

- `AGENTS.md`
- `ai-state.json`
- `docs/ai/project-context.md`
- `docs/ai/operating-model.md`
- `docs/ai/current-task.md`
- `docs/ai/decisions.md`
- `docs/ai/handoff.md`
- `.ai-toolkit/project.env`

Use Markdown for human-plus-AI context, `ai-state.json` for machine-readable
state, and `.ai-toolkit/project.env` for shell-safe automation config.

OpenCode note: it discovers `AGENTS.md` automatically, but does not
recursively load `docs/ai/*` on its own. `ai-init` and `ai-fix` maintain an
`ai-toolkit` block in the project's `AGENTS.md` that explicitly points to the
shared context files. Run either command once in an existing project, then
start a new OpenCode session.

## Quick Start
```bash
./install.sh                 # install/update toolkit on macOS/Linux/WSL
ai-toolkit --help           # show all commands
ai-init                      # Bootstrap current project
ai-resume                    # Show current context
ai-doctor                    # Audit project and toolkit health
ai-fix                       # Repair safe project/tooling issues
ai-sync                      # Push project summary to Uteke
ai-recall                    # Recall project memory from Uteke
ai-session                   # Build one briefing for the next AI session
ai-map                       # Query codebase-memory for this project
ai-start "refactor auth"     # Start active task
ai-close --summary "..."     # Save handoff summary
ai-commit "feat: auth"       # Auto-sync context and commit
ai-push                      # Push current branch
ai-deploy production         # Run configured deploy command
```

## Installing on Another Device

The toolkit runs natively on macOS and Linux. Windows has a PowerShell
installer and launcher, but still requires Git for Windows/Git Bash because
the automation engine is Bash-based. WSL is also supported.

From a terminal:

```bash
git clone https://github.com/innaka-tech/ai-toolkit.git ~/.ai-toolkit
~/.ai-toolkit/install.sh
source ~/.zshrc              # macOS/zsh
# or: source ~/.bashrc       # Linux/bash/WSL
ai-toolkit --help
```

On Windows PowerShell:

```powershell
.\install.ps1
ai-toolkit.ps1 --help
```

If PowerShell blocks local scripts, enable them for the current user:

```powershell
Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
```

For a private repository, authenticate Git first (SSH or HTTPS). To install
to another location, set `AI_TOOLKIT_DIR` before running the installer:

```bash
AI_TOOLKIT_DIR="$HOME/tools/ai-toolkit" ./install.sh
```

Full usage guide: [USAGE.md](/Users/anasfikri/.ai-toolkit/USAGE.md:1)

## Automation

- `ai-init` also installs lightweight git hooks for context reminders.
- `ai-commit` stages all changes, updates handoff context, and commits.
- `ai-push` pushes to the configured upstream or sets `origin/<branch>` on first push.
- `ai-deploy` reads commands from `.ai-toolkit/project.env`.

Example `.ai-toolkit/project.env`:

```bash
PROJECT_NAME="my-app"
DEFAULT_BRANCH="main"
DEPLOY_DEFAULT="npm run deploy"
DEPLOY_STAGING="npm run deploy:staging"
DEPLOY_PRODUCTION="npm run deploy:prod"
POST_DEPLOY_CHECK="npm run smoke"
```

# Project context

aitk: a single Go binary that keeps an AI-assisted project's state (tasks, handoffs, knowledge, standards) in git and refuses "done" until a machine verified it.
Users: developers and every AI coding agent (Claude Code, Codex, Cursor, Gemini, …) working in a repository that has aitk.toml.
Constraints: offline by default, plain-text files in git, atomic writes validated by schemas in schemas/, exit codes 0–4.

## Stack
- Go (go.mod), cobra CLI, MCP server via modelcontextprotocol/go-sdk; embedded assets: commands/ (slash commands, personas), standards/, skills/, schemas/.

## Layout
- `cmd/aitk` entrypoint · `internal/cli` commands · `internal/ops` workflow logic · focused packages under `internal/`.
- Spec `docs/spec/`, ADRs `docs/adr/`, how-to `docs/how-to/`, generated CLI reference `docs/reference/cli.md`.

## Non-negotiables
- No network except `aitk audit install` and plugins the user enables.
- Agents cannot accept UAT or mark a task done themselves; gates run in hooks, CI, MCP and `aitk run`.
- Sensitive paths (aitk.toml `[risk]`) make a task strict: risk analysis + independent review.

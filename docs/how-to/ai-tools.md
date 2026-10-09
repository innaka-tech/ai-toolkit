# Set up AI tools

```bash
aitk adapters doctor          # which tools are installed and whether they are set up
aitk adapters sync            # set up every installed tool (project files, idempotent)
aitk adapters sync --tool claude-code --dry-run
aitk adapters sync --global   # also user-level config (Codex); backed up first
```

What gets written is listed in [the integrations spec](../spec/integrations.md#tool-adapters). Commit the project files (`.mcp.json`, `.claude/settings.json`, `CLAUDE.md`, `opencode.json`, `.gemini/settings.json`, …) so teammates get the same setup. `aitk` must be on PATH for the tools to start the MCP server.

- **Claude Code** asks once to approve the project MCP server. After that, every session starts with the brief (SessionStart hook) and the `mcp__aitk__*` tools are available.
- **Codex** has no project-level MCP config; use `--global`.
- A config file that is not plain JSON (for example JSON with comments) is never rewritten; aitk tells you what to add by hand.

The tool that ran a command is recorded in handoffs and commit trailers. It is detected from the MCP client name or the environment; set `AITK_TOOL` to override.

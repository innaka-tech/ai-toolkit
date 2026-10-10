# Set up AI tools

Once per machine, so that every AI tool you use recognises aitk projects without being told:

```bash
aitk setup --dry-run          # what would change in your tools' user-level config
aitk setup                    # Claude Code, Codex, OpenCode, Gemini CLI, Qwen Code, Kiro, Windsurf (installed ones)
aitk setup --remove           # take it all out again
```

Per repository, for the tools your team uses (commit the result):

```bash
aitk adapters doctor          # which tools are installed and whether they are set up
aitk adapters sync            # set up every installed tool (project files, idempotent)
aitk adapters sync --tool claude-code --dry-run
aitk adapters sync --tool all # every supported tool, for mixed teams
aitk adapters sync --global   # also user-level config (Codex); backed up first
```

What gets written is listed in [the integrations spec](../spec/integrations.md#tool-adapters). Commit the project files (`.mcp.json`, `.claude/settings.json`, `CLAUDE.md`, `opencode.json`, `.gemini/settings.json`, …) so teammates get the same setup. `aitk` must be on PATH for the tools to start the MCP server.

- **Claude Code** asks once to approve the project MCP server. After that, every session starts with the brief (SessionStart hook) and the `mcp__aitk__*` tools are available.
- **Codex** has no project-level MCP config; `aitk setup` registers it for your user.
- **Antigravity CLI (`agy`)**: `aitk setup` registers the server with `agy mcp add`. Headless runs (`agy -p`) refuse MCP tools until you allow them: add `mcp(aitk)` to `permissions.allow` in `~/.gemini/antigravity-cli/settings.json` (aitk does not grant permissions for you).
- **jcode**: the server is added to `~/.jcode/mcp.json`; **pi** has no MCP support by design and uses the aitk CLI, guided by the skill and `~/.pi/agent/AGENTS.md`.
- **Copilot, Windsurf, Cline, Roo Code, Aider, Junie, Qwen Code**: a rule or instruction file points the agent to AGENTS.md; Copilot (VS Code), Roo Code, and Qwen Code also get the MCP server.
- To check that an agent picks the workflow up on its own, open a session in an aitk repository and ask for a small change without mentioning aitk: it should run `aitk brief`, take a task, and finish with `aitk close`.
- A config file that is not plain JSON (for example JSON with comments) is never rewritten; aitk tells you what to add by hand.

The tool that ran a command is recorded in handoffs and commit trailers. It is detected from the MCP client name or the environment; set `AITK_TOOL` to override.

# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [1.0.0] - 2026-10-09

Final release of the bash implementation. v2 (`aitk`) will be a rewrite; `ai-*` commands will keep working as shims.

### Removed
- Private agent-coordination layer (`ai-agent-report`, `ai-agent-report-jaka`, `ai-handoff`, `ai-handoff-remote`, `FUSION_AI_PROTOCOL.md`, bridge queue). It now lives in a separate private repository.
- Account rotation (`ai-rotate-ag`) and placeholder provider/cost settings in `config.yaml`.
- Keyword-based provider auto-selection in `ai-exec`.

### Fixed
- `ai-loop` sent an empty prompt to every agent: the prompt variable was not visible inside `bash -c`. Agents now receive the prompt as a single argument.
- `ai-loop` never ran the agent when neither `timeout` nor `gtimeout` was installed. A portable perl fallback now enforces the timeout (exit 124).
- `ai-exec --auto` / `--agent NAME` (as documented) are now accepted; unknown providers exit 2 instead of 0.
- Duplicate `mcp` entry in `ai-toolkit` help and dispatcher.
- Absolute personal path in README.

### Added
- `tests/smoke.sh`: end-to-end check of init → start → preflight → exec → close in an isolated HOME.
- CI: ShellCheck, syntax check, smoke test on Linux and macOS, gitleaks.

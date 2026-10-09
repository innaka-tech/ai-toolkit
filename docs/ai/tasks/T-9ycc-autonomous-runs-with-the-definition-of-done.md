---
id: T-9ycc
title: Autonomous runs with the Definition of Done
status: in_review
profile: standard
tags:
  - autopilot
created: "2026-10-09T16:54:39Z"
updated: "2026-10-09T17:46:49Z"
created_by: claude-code
workers:
  - claude-code
branch: feat/autopilot
review:
  passes:
    - "n": 1
      by: claude-code
      at: "2026-10-09T17:44:32Z"
      findings: 25
    - "n": 2
      by: claude-code
      at: "2026-10-09T17:44:33Z"
      findings: 11
    - "n": 3
      by: claude-code
      at: "2026-10-09T17:44:33Z"
      findings: 2
    - "n": 4
      by: codex
      at: "2026-10-09T17:46:49Z"
      findings: 1
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 38632
    summary: |-
      … test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/checkrun	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/claims	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/cli	37.336s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/compat	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/conv	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doc	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doctor	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/fsx	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/gates	27.908s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/gitx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/handoff	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/heal	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/knowledge	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver	3.505s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/migrate	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/ops	2.416s
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/plugins	4.819s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/profile	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/project	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/release	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/schema	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/secrets	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/session	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/task	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/textx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/schemas	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/skills	[no test files]
    at: "2026-10-09T17:43:35Z"
    tree: 57ce56853d13aedd416856ef0c8b68029f57997194726062f0e05c46c38e602c
  audit:
    at: "2026-10-09T17:44:18Z"
    passed: true
    tree: ecd778de925a4ac4d5569d3c967f0028f3c8e6356a3efa66e3a3767960272d4b
    results:
      - name: osv-scanner (dependencies)
        exit_code: 0
        summary: |-
          Scanning dir .
          Starting filesystem walk for root: /
          Scanned /Users/anasfikri/.ai-toolkit/go.mod file and found 18 packages
          End status: 350 dirs visited, 1687 inodes visited, 1 Extract calls, 11.466625ms elapsed, 11.466ms wall time

          No issues found
      - name: aitk secret scan (tracked files)
        exit_code: 0
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [x] Given ready tasks, when aitk run runs a headless agent, then tasks finish only through the agent's aitk close
- [x] Given dependencies and claims, when aitk task next runs, then only startable tasks are listed in order
- [x] Given a failed audit, when aitk audit --tasks runs, then one fix task per vulnerable dependency exists without duplicates
- [x] Given a task tagged bug, when it closes without a changed test file, then close refuses
- [x] Given a Spec Kit tasks.md, when tasks are imported and closed, then phases become dependencies and lines are checked off
- [x] Given a change, when aitk impact runs, then referencing files and covering tests are listed

## Risk
Impact: new commands (run, task next, impact, audit --tasks/install) and a new DoD rule for tasks tagged bug; the AGENTS.md block gains a revision line, so projects see an outdated block until doctor --fix; the pre-commit gate now accepts other releases' blocks.
Security (STRIDE): Spoofing/Elevation — agents started by aitk run could forge done, UAT, reviews, profile, tags, or handoffs; mitigated by re-deriving the outcome from a pre-attempt snapshot plus aitk's own check/audit rerun, and AITK_RUN blocks uat accept. Tampering — --commit could sweep unrelated or secret files; mitigated by a clean-tree requirement, index reset, aitk-only staging, and stash. Denial of service — orphaned agent processes; process group killed on timeout and after each attempt (setsid escape documented). Information disclosure — prompt passed as a file (0600) in the git dir, not argv. osv-scanner download verified by SHA-256 from one pinned release.
Rollback: revert the merge commit; projects keep working with 2.3 (they may need doctor --fix to restore the old block); [run]/[quality] settings must be removed for older binaries.
Validation: unit and e2e tests for every review finding (forged done/UAT/profile/criteria, stash with renames, timeouts, locks, Spec Kit phases, audit IDs, regression gate scope); full suite with -race on macOS; Windows build/vet; live runs with Claude Code and Codex on a sample project including a bug task; three independent review rounds.

## Notes

## Security checklist (OWASP ASVS 4.0.3)
<!-- Check each item when verified, or write "N/A: <reason>" after the text and check it. -->
- [x] V1 Architecture: trust boundaries and the components handling sensitive data are identified for this change — boundary: agents started by aitk run are untrusted writers of the repo; decide() re-derives outcomes (see Risk)
- [x] V2 Authentication: credentials are verified server-side; no default, hard-coded, or weak secrets — N/A: no authentication; no secrets added (gitleaks clean)
- [x] V3 Session management: session tokens are random, protected, invalidated on logout and expiry — N/A: no sessions or tokens
- [x] V4 Access control: every endpoint/action checks authorization server-side, deny by default (no IDOR) — UAT accept refuses agents (AITK_RUN added to AgentInEnv); run refuses nesting and a second run (kernel lock)
- [x] V5 Validation, sanitization and encoding: all input validated; output encoded; no injection (SQL, OS, template) — prompt passed as a file (no shell/.cmd parsing); git pathspecs literal; syncSource rejects absolute/../non-md paths and symlinks leaving the repo; osv JSON parsed with typed structs
- [x] V6 Stored cryptography: sensitive data at rest uses approved algorithms; keys are not in the code — N/A: nothing encrypted; sha256 only for download integrity and ID hashing
- [x] V7 Error handling and logging: errors reveal no internals; security events are logged without secrets — typed E_* errors with fix lines; scanner output in tasks passes the credential guard (falls back to no output)
- [x] V8 Data protection: personal/sensitive data is minimized, not cached or logged, and removable — prompt file 0600 in the git dir; --commit refuses a dirty tree so local secrets are never swept into commits
- [x] V9 Communication: TLS for all external traffic; certificates validated — osv-scanner download over HTTPS from GitHub with default certificate validation, SHA-256 verified
- [x] V10 Malicious code: no backdoors, time bombs, or unexplained network calls; dependencies are trusted — only network call is aitk audit install (explicit); no new Go dependencies; govulncheck in CI
- [x] V11 Business logic: flows cannot be abused (limits, ordering, replay, race conditions) — forged done/UAT/profile/criteria/handoff all tested; max tasks/attempts, stop after two unfinished, lock against concurrent runs
- [x] V12 Files and resources: uploads are type/size limited and stored safely; no path traversal or SSRF — no uploads; download size capped (512 MB) and written via a temp file then renamed; path traversal checks in syncSource
- [x] V13 API and web service: API inputs/outputs are schema-validated; CORS and rate limits are deliberate — MCP inputs typed; config and task files schema-validated (new run/quality/regression_tests fields in schemas)
- [x] V14 Configuration: secure defaults, security headers, no debug in production, dependencies up to date — narrow default agent permissions; regression gate on by default; extra permissions only via explicit [run].args

---
id: T-bq2a
title: Fix aitk close when docs/ is a symlink
status: in_review
profile: strict
created: "2026-11-28T02:09:55Z"
updated: "2026-11-28T06:55:16Z"
created_by: claude-code
workers:
  - claude-code
branch: claude/epic-franklin-wwvbv3
base: 781f9b4a6f130828994f0f1fa20253c581af2946
paths:
  - CHANGELOG.md
  - internal/audit/audit.go
  - internal/audit/legacy_test.go
  - internal/cli/symlink_docs_test.go
  - internal/compat/gitattributes.go
  - internal/gates/gates.go
  - internal/gates/run_e2e_test.go
  - internal/heal/heal.go
  - internal/knowledge/knowledge.go
  - internal/migrate/migrate.go
  - internal/ops/impact.go
  - internal/ops/import_frameworks.go
  - internal/ops/release.go
  - internal/ops/run.go
  - internal/profile/profile.go
  - internal/project/gitrel_test.go
  - internal/project/project.go
review:
  passes:
    - "n": 1
      by: claude-code
      at: "2026-11-28T02:19:51Z"
      findings: 4
    - "n": 2
      by: claude-code
      at: "2026-11-28T02:32:20Z"
      findings: 3
    - "n": 3
      by: claude-code
      at: "2026-11-28T06:49:39Z"
      findings: 3
    - "n": 4
      by: claude-code
      at: "2026-11-28T06:54:35Z"
      findings: 0
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 1083
    summary: |-
      …al/conv	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doc	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doctor	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/fsx	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/gates	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/gitx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/handoff	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/heal	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/knowledge	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/migrate	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/ops	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/plugins	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/proc	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/profile	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/project	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/release	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/schema	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/secrets	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/session	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/slash	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/slop	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/standards	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/task	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/textx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/schemas	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/skills	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/standards	[no test files]
    at: "2026-11-28T06:54:28Z"
    tree: d62d7586511865f911ff3871fd0ad727a75492e850f7266198ed0b7730703106
  commits:
    - 76d18c2
    - 9f30046
    - ed6b80b
  acceptance:
    total: 2
    done: 2
  audit:
    at: "2026-11-28T06:55:10Z"
    passed: true
    tree: d62d7586511865f911ff3871fd0ad727a75492e850f7266198ed0b7730703106
    results:
      - name: aitk secret scan (tracked files)
        exit_code: 0
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [x] Given a repo whose docs/ is a symlink inside the repo, when the agent runs aitk check then aitk close, then close succeeds
- [x] aitk run can commit a handover in such a repo

## Risk
Impact: path mapping for aitk's own files in the fingerprint, change list, pre-commit/ci schema gate, audit secret scan, doctor, impact, import bmad, release and run handover. In a repository without symlinks GitRel/AitkRel return paths unchanged, so behaviour there is identical (full suite passes; a .gitattributes byte-for-byte check in a plain repo). Performance: GitRel is cached per Project; aitk check on 40k untracked files 0.57 s (main) vs 0.60 s.
Security (STRIDE): Tampering/Elevation — the pre-commit and ci schema gates now see task files behind a link that they previously skipped (a gap closed, not opened); a link pointing outside the repository, to the root, or unresolvable is never mapped, so aitk does not start excluding or reading paths outside the repo. Information disclosure — the secret scan still scans everything except _legacy; the _legacy skip only matches inside the repo. No new network, no new writes outside the repo.
Rollback: revert the PR; no data or schema migration (no new fields in task, handoff or config files).
Validation: e2e tests for close, run --commit handover, pre-commit schema rejection, symlinked CLAUDE.md, impact/import/release behind the link; unit tests for GitRel/AitkRel (plain, missing, docs link, docs/ai link, outside link, link to root) and the audit _legacy skip; each fix mutation-checked; aitk check passes.

## Notes

## Security checklist (OWASP ASVS 4.0.3)
<!-- Check each item when verified, or write "N/A: <reason>" after the text and check it. -->
- [x] V1 Architecture: trust boundaries and the components handling sensitive data are identified for this change — the boundary is the repository root: GitRel maps only paths that resolve inside it
- [x] V2 Authentication: credentials are verified server-side; no default, hard-coded, or weak secrets — N/A: no authentication in this change
- [x] V3 Session management: session tokens are random, protected, invalidated on logout and expiry — N/A: no sessions
- [x] V4 Access control: every endpoint/action checks authorization server-side, deny by default (no IDOR) — N/A: no endpoints or actions with authorization
- [x] V5 Validation, sanitization and encoding: all input validated; output encoded; no injection (SQL, OS, template) — mapped paths go to git as separate arguments after `--` (no shell); they come from the filesystem layout and aitk.toml, as before
- [x] V6 Stored cryptography: sensitive data at rest uses approved algorithms; keys are not in the code — N/A: no cryptography
- [x] V7 Error handling and logging: errors reveal no internals; security events are logged without secrets — unchanged; an unresolvable path falls back to its literal name
- [x] V8 Data protection: personal/sensitive data is minimized, not cached or logged, and removable — N/A: no personal data
- [x] V9 Communication: TLS for all external traffic; certificates validated — N/A: no network traffic
- [x] V10 Malicious code: no backdoors, time bombs, or unexplained network calls; dependencies are trusted — no new dependencies or network calls
- [x] V11 Business logic: flows cannot be abused (limits, ordering, replay, race conditions) — the pre-commit/ci schema gates now see task files behind a link (a gap closed); GitRel results are cached per Project (one command), so a layout change mid-command is not seen until the next command
- [x] V12 Files and resources: uploads are type/size limited and stored safely; no path traversal or SSRF — links resolving outside the repository, to the root, or unresolvable are never mapped (unit-tested)
- [x] V13 API and web service: API inputs/outputs are schema-validated; CORS and rate limits are deliberate — N/A: no API or web service
- [x] V14 Configuration: secure defaults, security headers, no debug in production, dependencies up to date — no configuration changes

## Evidence
<!-- Generated by aitk close. -->
- Check: `go vet ./... && go test -short ./...` → exit 0 at 2026-11-28T06:54:28Z (1083 ms)
- Acceptance criteria: 2/2 satisfied
- Commits: 76d18c2, 9f30046, ed6b80b
- Review pass 1 by claude-code: 4 findings
- Review pass 2 by claude-code: 3 findings
- Review pass 3 by claude-code: 3 findings
- Review pass 4 by claude-code: 0 findings

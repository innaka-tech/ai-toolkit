---
id: T-khk3
title: Fix empty GitHub release notes
status: in_progress
profile: standard
tags:
  - bug
created: "2026-11-26T15:46:59Z"
updated: "2026-11-26T15:53:40Z"
created_by: claude-code
workers:
  - claude-code
branch: fix/release-notes
base: ef8eecdf068776b29cb6ed701bc3c7a57ad2d304
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 1398
    summary: |-
      …st files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/conv	[no test files]
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
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/profile	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/project	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/release	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/schema	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/secrets	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/session	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/slash	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/standards	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/task	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/textx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/schemas	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/skills	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/standards	[no test files]
    at: "2026-11-26T15:52:25Z"
    tree: cf9a01c6f8b1e98057bc2e483f5a5e2ea963df95a891fe7b01ae8307905d0826
  audit:
    at: "2026-11-26T15:53:37Z"
    passed: false
    tree: cf9a01c6f8b1e98057bc2e483f5a5e2ea963df95a891fe7b01ae8307905d0826
    results:
      - name: govulncheck
        exit_code: 1
        summary: 'govulncheck: fetching vulnerabilities: Get "https://vuln.go.dev/index/modules.json.gz": Forbidden (agent proxy: no rule or network setting allows this host)'
      - name: aitk secret scan (tracked files)
        exit_code: 0
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [x] Given a v2.* tag, when release.yml runs GoReleaser with --release-notes from CHANGELOG.md, then the GitHub release body is that CHANGELOG section
- [x] Given .goreleaser.yaml, when goreleaser check runs, then the configuration is valid

## Risk
Impact: release pipeline only (.goreleaser.yaml changelog block). Binaries, archives, checksums, SBOM and cosign signing are untouched; only the GitHub release body changes from empty to the CHANGELOG.md section that release.yml already extracts.
Security (STRIDE): no new input or credential; release notes come from CHANGELOG.md in the tagged commit (tampering requires a merged commit, same as today). No change to token scope or signing.
Rollback: restore `changelog: disable: true` (one line) before the next tag; already published releases are unaffected.
Validation: regression test cmd/aitk/release_config_test.go (fails on the old config); goreleaser v2.20.3 local release with --skip=publish,sign,sbom,validate: old config skips the changelog pipe (0 'generating changelog'), new config runs it and loads --release-notes; `goreleaser check` validates the config; aitk check passes.

## Notes

## Security checklist (OWASP ASVS 4.0.3)
<!-- Check each item when verified, or write "N/A: <reason>" after the text and check it. -->
- [x] V1 Architecture: trust boundaries and the components handling sensitive data are identified for this change Verified: the only boundary is CHANGELOG.md (tagged commit) → GoReleaser → GitHub release body; no sensitive data handled.
- [x] V2 Authentication: credentials are verified server-side; no default, hard-coded, or weak secrets N/A: no authentication change; GITHUB_TOKEN usage in release.yml unchanged.
- [x] V3 Session management: session tokens are random, protected, invalidated on logout and expiry N/A: no sessions.
- [x] V4 Access control: every endpoint/action checks authorization server-side, deny by default (no IDOR) N/A: no endpoints; release permissions in release.yml unchanged.
- [x] V5 Validation, sanitization and encoding: all input validated; output encoded; no injection (SQL, OS, template) Verified: release notes are markdown from CHANGELOG.md passed as a file, not interpolated into a shell command or template.
- [x] V6 Stored cryptography: sensitive data at rest uses approved algorithms; keys are not in the code N/A: no stored cryptography; cosign signing config unchanged.
- [x] V7 Error handling and logging: errors reveal no internals; security events are logged without secrets N/A: no error handling or logging change.
- [x] V8 Data protection: personal/sensitive data is minimized, not cached or logged, and removable N/A: no personal data; release notes are public changelog text.
- [x] V9 Communication: TLS for all external traffic; certificates validated N/A: no new network traffic.
- [x] V10 Malicious code: no backdoors, time bombs, or unexplained network calls; dependencies are trusted Verified: config-only deletion plus a test; no new dependency (test uses go.yaml.in/yaml/v3 already in go.mod).
- [x] V11 Business logic: flows cannot be abused (limits, ordering, replay, race conditions) N/A: no business flow.
- [x] V12 Files and resources: uploads are type/size limited and stored safely; no path traversal or SSRF N/A: no file uploads or paths from users.
- [x] V13 API and web service: API inputs/outputs are schema-validated; CORS and rate limits are deliberate N/A: no API.
- [x] V14 Configuration: secure defaults, security headers, no debug in production, dependencies up to date Verified: goreleaser check passes; only the changelog block changed.

---
id: T-7m0j
title: 'Development assistant: standards, SDLC personas, lean character, debt ledger'
status: done
profile: standard
created: "2026-11-26T14:00:45Z"
updated: "2026-11-26T14:08:39Z"
closed: "2026-11-26T14:08:39Z"
created_by: claude-code
workers:
  - claude-code
branch: claude/epic-franklin-wwvbv3
base: 67dfc2079ef5accca842cf0c27a6f060c5d3281c
paths:
  - .claude/skills/aitk/SKILL.md
  - CHANGELOG.md
  - README.md
  - commands/aitk-analyst.md
  - commands/aitk-architect.md
  - commands/aitk-auditor.md
  - commands/aitk-dba.md
  - commands/aitk-devops.md
  - commands/aitk-fix.md
  - commands/aitk-hunt.md
  - commands/aitk-lean.md
  - commands/aitk-pm.md
  - commands/aitk-qa.md
  - commands/aitk-review.md
  - commands/aitk-security.md
  - commands/aitk-team.md
  - commands/aitk-ux.md
  - commands/aitk-writer.md
  - docs/how-to/commands.md
  - docs/how-to/standards.md
  - docs/reference/cli.md
  - docs/spec/cli.md
  - docs/v1/conventions/README.md
  - docs/v1/conventions/RISK_ANALYSIS_PROTOCOL.md
  - docs/v1/conventions/STANDARD_DEVELOPMENT.md
  - docs/v1/conventions/templates/CONVENTIONS_TEMPLATE.md
  - docs/v1/conventions/templates/START_HERE_TEMPLATE.md
  - docs/v1/conventions/templates/TASK_TEMPLATE.md
  - internal/brief/brief.go
  - internal/cli/cli.go
  - internal/cli/standards.go
  - internal/cli/standards_test.go
  - internal/mcpserver/server.go
  - internal/mcpserver/server_test.go
  - internal/ops/debt.go
  - internal/slash/slash_test.go
  - internal/standards/standards.go
  - skills/aitk/SKILL.md
  - standards/ai-agents.md
  - standards/api.md
  - standards/character.md
  - standards/database.md
  - standards/documentation.md
  - standards/embed.go
  - standards/engineering.md
  - standards/git.md
  - standards/operations.md
  - standards/review.md
  - standards/sdlc.md
  - standards/security.md
  - standards/testing.md
  - standards/ux.md
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 26665
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
    at: "2026-11-26T14:08:32Z"
    tree: 3fcb963742379cfa8f0db3fca9d738f05ea7670f701c685a29d32565a5ba3d00
  acceptance:
    total: 4
    done: 4
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [x] Given any project, when an agent runs aitk standards, then it lists embedded development standards and prints one by topic, with project overrides in docs/ai/standards/
- [x] Given aitk prompt, when listed, then SDLC personas (pm, analyst, architect, security, auditor, qa, devops, dba, writer) are available
- [x] Given code with 'shortcut:' comments, when aitk debt runs, then it lists each with file:line
- [x] Given aitk brief, when printed, then its rules carry the lean character (smallest change, never cut safety, end with skipped/risk)

## Risk
(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)

## Notes

## Evidence
<!-- Generated by aitk close. -->
- Check: `go vet ./... && go test -short ./...` → exit 0 at 2026-11-26T14:08:32Z (26665 ms)
- Acceptance criteria: 4/4 satisfied

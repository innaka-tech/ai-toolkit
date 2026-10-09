---
id: T-9ycc
title: Autonomous runs with the Definition of Done
status: in_progress
profile: standard
tags:
  - autopilot
created: "2026-10-09T16:54:39Z"
updated: "2026-10-09T16:55:16Z"
created_by: claude-code
workers:
  - claude-code
branch: feat/autopilot
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 36531
    summary: |-
      …ef	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/checkrun	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/claims	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/cli	34.936s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/compat	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/conv	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doc	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doctor	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/fsx	2.763s
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/gates	14.919s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/gitx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/handoff	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/heal	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit	1.278s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/knowledge	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver	4.100s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/migrate	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/ops	1.719s
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/plugins	6.557s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/profile	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/project	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/release	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/schema	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/secrets	3.387s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/session	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/task	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/textx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/schemas	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/skills	[no test files]
    at: "2026-10-09T16:55:16Z"
    tree: 0d59192aa33b0578d0559ff30f850b45da2a0e8d24aa6365c74fcb8e3a105591
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [ ] Given ready tasks, when aitk run runs a headless agent, then tasks finish only through the agent's aitk close
- [ ] Given dependencies and claims, when aitk task next runs, then only startable tasks are listed in order
- [ ] Given a failed audit, when aitk audit --tasks runs, then one fix task per vulnerable dependency exists without duplicates
- [ ] Given a task tagged bug, when it closes without a changed test file, then close refuses
- [ ] Given a Spec Kit tasks.md, when tasks are imported and closed, then phases become dependencies and lines are checked off
- [ ] Given a change, when aitk impact runs, then referencing files and covering tests are listed

## Risk
(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)

## Notes

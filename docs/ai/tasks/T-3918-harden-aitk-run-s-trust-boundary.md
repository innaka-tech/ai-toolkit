---
id: T-3918
title: Harden aitk run's trust boundary
status: in_review
profile: standard
tags:
  - security
created: "2026-11-27T14:10:15Z"
updated: "2026-11-27T15:10:19Z"
created_by: claude-code
workers:
  - claude-code
branch: feat/run-hardening
base: 433cacd17e17a77f4cd24431779ee176f1512fb9
review:
  passes:
    - "n": 1
      by: claude-code
      at: "2026-11-27T14:20:42Z"
      findings: 4
      tree: ebebdd696041516019d271f45c37a52eb737308d1b45d4ade97052b7253d421a
    - "n": 2
      by: claude-code
      at: "2026-11-27T15:03:41Z"
      findings: 7
      tree: d83ebff804fc9aa20a764f18d7aa0250a1e3639667d960b5246e731d110021ba
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 39619
    summary: |-
      …al/compat	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/conv	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doc	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doctor	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/fsx	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/gates	37.918s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/gitx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/handoff	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/heal	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/knowledge	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver	0.864s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/migrate	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/ops	0.023s
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/plugins	1.702s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/proc	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/profile	0.782s
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
    at: "2026-11-27T15:10:18Z"
    tree: 42f63aacecdd91658d8cb0b72978f254a5f9a2a8374db92df133fd7f45731c4b
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [ ] Given an agent that blocks or cancels its task inside aitk run, when the attempt ends, then the agent is recorded as a worker
- [ ] Given an agent that edits aitk.toml during an attempt, when the attempt ends, then aitk.toml is restored and the run reports it
- [ ] Given an agent that changes another existing task's file during an attempt, when the attempt ends, then that file is restored and the run reports it; new task files are kept

## Risk
(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)

## Notes

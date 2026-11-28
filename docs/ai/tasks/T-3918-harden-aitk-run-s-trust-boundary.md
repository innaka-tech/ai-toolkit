---
id: T-3918
title: Harden aitk run's trust boundary
status: done
profile: standard
tags:
  - security
created: "2026-11-27T14:10:15Z"
updated: "2026-11-28T09:49:11Z"
closed: "2026-11-28T09:49:11Z"
created_by: claude-code
workers:
  - claude-code
branch: feat/run-hardening
base: 433cacd17e17a77f4cd24431779ee176f1512fb9
paths:
  - CHANGELOG.md
  - README.en.md
  - README.md
  - docs/how-to/autopilot.md
  - docs/how-to/quality.md
  - docs/reference/cli.md
  - docs/spec/cli.md
  - docs/spec/workflow.md
  - internal/brief/assistant.go
  - internal/brief/brief.go
  - internal/cli/autopilot_test.go
  - internal/cli/cli.go
  - internal/cli/review_loop_test.go
  - internal/gates/run_e2e_test.go
  - internal/mcpserver/server.go
  - internal/ops/ops.go
  - internal/ops/quality.go
  - internal/ops/run.go
  - internal/ops/work.go
  - internal/project/project.go
  - internal/task/task.go
  - schemas/config.schema.json
  - schemas/task.schema.json
  - standards/sdlc.md
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
    - "n": 3
      by: claude-code
      at: "2026-11-27T15:41:14Z"
      findings: 4
      tree: 42f63aacecdd91658d8cb0b72978f254a5f9a2a8374db92df133fd7f45731c4b
    - "n": 4
      by: claude-code
      at: "2026-11-27T15:59:44Z"
      findings: 3
      tree: d8eeb13ac7d2cc56358a50bdd58bcc5d69fc22aa8668e0f93e9feb8a41190899
    - "n": 5
      by: claude-code
      at: "2026-11-27T16:12:12Z"
      findings: 3
      tree: 7dc1160d753294dc51318e899e2d0d58bfa43a267e67346b01aff73251fe1b0a
    - "n": 6
      by: claude-code
      at: "2026-11-28T00:49:42Z"
      findings: 3
      tree: d9a75c43401ea1302d34385169d04c9c4ff3a073e6cfc211135b15903393e733
    - "n": 7
      by: claude-code
      at: "2026-11-28T01:38:07Z"
      findings: 2
      tree: 327a68363a2c718824711832d021a540d202d06d66fa0733f65cf1b7c44451e8
    - "n": 8
      by: claude-code
      at: "2026-11-28T02:22:38Z"
      findings: 3
      tree: 46de57dea57dacd084ec2c2f8e5146d766610be6af47c84af5de9aa6b4de8444
    - "n": 9
      by: claude-code
      at: "2026-11-28T07:17:49Z"
      findings: 3
      tree: b3869d98da86a782ba63281c353cc2988024c33974236d3de87894f3a9f837a6
    - "n": 10
      by: claude-code
      at: "2026-11-28T09:49:02Z"
      findings: 0
      tree: 80e08487a736eef779d4a0a4c3ee4dc9c9d3b1ae4e783337e33af846d0cf7f2c
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 50069
    summary: |-
      …al/compat	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/conv	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doc	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doctor	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/fsx	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/gates	48.257s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/gitx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/handoff	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/heal	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/knowledge	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver	0.655s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/migrate	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/ops	0.012s
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/plugins	1.516s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/proc	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/profile	0.803s
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
    at: "2026-11-28T09:48:36Z"
    tree: 80e08487a736eef779d4a0a4c3ee4dc9c9d3b1ae4e783337e33af846d0cf7f2c
  commits:
    - 0221cc0
    - 0ca960f
    - 32d94da
    - 469218a
    - 614e445
    - 7ddb2c2
    - 823ee55
    - 88582ea
    - 8bdb7f4
    - 93a44bc
    - 961f947
    - 965a3d9
    - b3aa2d8
    - b3ab982
    - c107372
    - d5ea5b1
    - db37267
    - ddf02aa
    - ea96777
  acceptance:
    total: 3
    done: 3
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [x] Given an agent that blocks or cancels its task inside aitk run, when the attempt ends, then the agent is recorded as a worker
- [x] Given an agent that edits aitk.toml during an attempt, when the attempt ends, then aitk.toml is restored and the run reports it
- [x] Given an agent that changes another existing task's file during an attempt, when the attempt ends, then that file is restored and the run reports it; new task files are kept

## Risk
(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)

## Notes

## Evidence
<!-- Generated by aitk close. -->
- Check: `go vet ./... && go test -short ./...` → exit 0 at 2026-11-28T09:48:36Z (50069 ms)
- Acceptance criteria: 3/3 satisfied
- Commits: 0221cc0, 0ca960f, 32d94da, 469218a, 614e445, 7ddb2c2, 823ee55, 88582ea, 8bdb7f4, 93a44bc, 961f947, 965a3d9, b3aa2d8, b3ab982, c107372, d5ea5b1, db37267, ddf02aa, ea96777
- Review pass 1 by claude-code: 4 findings
- Review pass 2 by claude-code: 7 findings
- Review pass 3 by claude-code: 4 findings
- Review pass 4 by claude-code: 3 findings
- Review pass 5 by claude-code: 3 findings
- Review pass 6 by claude-code: 3 findings
- Review pass 7 by claude-code: 2 findings
- Review pass 8 by claude-code: 3 findings
- Review pass 9 by claude-code: 3 findings
- Review pass 10 by claude-code: 0 findings

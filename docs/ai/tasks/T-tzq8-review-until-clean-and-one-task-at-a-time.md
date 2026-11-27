---
id: T-tzq8
title: Review until clean and one task at a time
status: in_review
profile: standard
created: "2026-11-27T10:55:05Z"
updated: "2026-11-27T11:23:36Z"
created_by: claude-code
workers:
  - claude-code
branch: feat/review-until-clean
base: f6c44b00e872b304a654175b6fd418a53c64a2ca
review:
  passes:
    - "n": 1
      by: claude-code
      at: "2026-11-27T11:04:33Z"
      findings: 4
    - "n": 2
      by: claude-code
      at: "2026-11-27T11:13:06Z"
      findings: 5
    - "n": 3
      by: claude-code
      at: "2026-11-27T11:23:36Z"
      findings: 5
      tree: 4dbaff41dd608407049dc78b6ee7b877479650140f99f424bfe8bbe0ca41c7e9
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 30419
    summary: |-
      …/compat	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/conv	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doc	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doctor	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/fsx	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/gates	18.923s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/gitx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/handoff	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/heal	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/knowledge	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver	0.756s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/migrate	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/ops	0.014s
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/plugins	1.659s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/proc	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/profile	(cached)
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
    at: "2026-11-27T11:16:34Z"
    tree: 4dbaff41dd608407049dc78b6ee7b877479650140f99f424bfe8bbe0ca41c7e9
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [ ] Given [quality] review = standard and a standard task with no clean review pass, when aitk close runs, then the task goes to in_review with a note to review until a pass finds 0; after aitk review pass --findings 0 and close it is done
- [ ] Given an active task that is in_progress or in_review, when an agent starts another task (task start, task next --start, task new --start), then it fails with E_TASK_ACTIVE and says to finish or block it; blocking it or finishing it allows the switch
- [ ] Given review passes that stop decreasing, when aitk brief runs, then it says the review is not converging and to ask the user
- [ ] Given a project without [quality] review, when a standard task closes, then behaviour is unchanged (only strict needs reviews); aitk init writes review = standard for new projects

## Risk
(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)

## Notes

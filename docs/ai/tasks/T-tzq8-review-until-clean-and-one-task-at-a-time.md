---
id: T-tzq8
title: Review until clean and one task at a time
status: in_review
profile: standard
created: "2026-11-27T10:55:05Z"
updated: "2026-11-27T15:02:01Z"
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
    - "n": 4
      by: claude-code
      at: "2026-11-27T11:43:37Z"
      findings: 5
      tree: 57257ed974a0b4bb1eb645e49b93febcf31b35f9215b382fd02dc6ea63bcdcc9
    - "n": 5
      by: claude-code
      at: "2026-11-27T12:00:00Z"
      findings: 4
      tree: ac550f1c577ca29bbb713dbf2e68ccecd18b7eff0afd5001c30e0cb2dc580b49
    - "n": 6
      by: claude-code
      at: "2026-11-27T12:13:43Z"
      findings: 4
      tree: 4da3d3a0bc3d5763db1e6546dee0e85e5cdb60f0163ebe0ce6924f135f53055a
    - "n": 7
      by: claude-code
      at: "2026-11-27T14:29:20Z"
      findings: 6
      tree: fd9e160d37ebba67e703f801de1645521c05b2b4b08ef028127783e1d5630f19
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 30627
    summary: |-
      …	[no test files]
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
    at: "2026-11-27T15:02:01Z"
    tree: 74afbce16cc3c6988a3a77344a6193c8682e41b520385fc60ca870064e63066e
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

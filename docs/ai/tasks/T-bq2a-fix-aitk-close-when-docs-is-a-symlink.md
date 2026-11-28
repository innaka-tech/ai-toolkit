---
id: T-bq2a
title: Fix aitk close when docs/ is a symlink
status: in_review
profile: standard
created: "2026-11-28T02:09:55Z"
updated: "2026-11-28T02:22:18Z"
created_by: claude-code
workers:
  - claude-code
branch: claude/epic-franklin-wwvbv3
base: 781f9b4a6f130828994f0f1fa20253c581af2946
review:
  passes:
    - "n": 1
      by: claude-code
      at: "2026-11-28T02:19:51Z"
      findings: 4
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 34276
    summary: |-
      …t/v2/internal/conv	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doc	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doctor	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/fsx	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/gates	23.151s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/gitx	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/handoff	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/heal	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/knowledge	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver	0.964s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/migrate	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/ops	0.018s
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/plugins	2.059s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/proc	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/profile	0.812s
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/project	0.181s
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
    at: "2026-11-28T02:22:18Z"
    tree: e17609d4d93b3509d126ec7e3cf670993545b4f0e10d8c369154d22a3f926afe
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [ ] Given a repo whose docs/ is a symlink inside the repo, when the agent runs aitk check then aitk close, then close succeeds
- [ ] aitk run can commit a handover in such a repo

## Risk
(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)

## Notes

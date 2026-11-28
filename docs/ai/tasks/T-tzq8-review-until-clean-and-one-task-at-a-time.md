---
id: T-tzq8
title: Review until clean and one task at a time
status: done
profile: standard
created: "2026-11-27T10:55:05Z"
updated: "2026-11-28T10:19:33Z"
closed: "2026-11-28T10:19:33Z"
created_by: claude-code
workers:
  - claude-code
branch: feat/review-until-clean
base: f6c44b00e872b304a654175b6fd418a53c64a2ca
paths:
  - CHANGELOG.md
  - README.en.md
  - README.md
  - commands/aitk-review.md
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
  - internal/handoff/handoff.go
  - internal/handoff/handoff_test.go
  - internal/mcpserver/server.go
  - internal/ops/ops.go
  - internal/ops/quality.go
  - internal/ops/run.go
  - internal/ops/work.go
  - internal/project/project.go
  - internal/task/task.go
  - schemas/config.schema.json
  - schemas/task.schema.json
  - standards/review.md
  - standards/sdlc.md
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
    - "n": 8
      by: claude-code
      at: "2026-11-27T15:10:59Z"
      findings: 4
      tree: 74afbce16cc3c6988a3a77344a6193c8682e41b520385fc60ca870064e63066e
    - "n": 9
      by: claude-code
      at: "2026-11-27T15:21:07Z"
      findings: 2
      tree: 4d0d9c5a2208f50242cbd657193092f3d7b419a925ffe22f6707d1198abcc390
    - "n": 10
      by: claude-code
      at: "2026-11-27T15:28:45Z"
      findings: 1
      tree: 94f996cf974fd85207c0f0c815b054d099f9f506cfed88ca7d477fccc2d0bdf8
    - "n": 11
      by: claude-code
      at: "2026-11-27T15:36:08Z"
      findings: 1
      tree: 71ba2ace3c9fc27eda12e4a9910ba080e52125b182b832f79aeb08436cbe28aa
    - "n": 12
      by: claude-code
      at: "2026-11-27T15:43:56Z"
      findings: 2
      tree: 3e6e42dbe7e1d80b77d83d92c7e75bbd2dbdc6f986485513de8644b45d9b2657
    - "n": 13
      by: claude-code
      at: "2026-11-27T15:49:29Z"
      findings: 4
      tree: 24385ab01a76114d04c98e378b92026244b1d8cae2dab168c882da11d9941ecc
    - "n": 14
      by: claude-code
      at: "2026-11-28T00:43:51Z"
      findings: 2
      tree: f5a7911889b30a86015deef27b3bac63046ad26c682281fc3fb35a012c811aa0
    - "n": 15
      by: claude-code
      at: "2026-11-28T00:49:29Z"
      findings: 2
      tree: f7c5d09d9b59c6497bdee90bbcff852337219faaead2f7d4401a2b1dc766cfbf
    - "n": 16
      by: claude-code
      at: "2026-11-28T01:32:40Z"
      findings: 1
      tree: 92b29080330469043f9f36d2ae7c5d2644b3c0f7445f954d07633f05ef0df250
    - "n": 17
      by: claude-code
      at: "2026-11-28T02:05:51Z"
      findings: 2
      tree: 7460f2abb09be5b632bd8307a7b92482efd05aaf0da110f12244413e43837957
    - "n": 18
      by: claude-code
      at: "2026-11-28T02:34:32Z"
      findings: 1
      tree: 542021ad8e3bb3de2862854e9fa01c94068d602929b6eb87b252fb1ed52db664
    - "n": 19
      by: claude-code
      at: "2026-11-28T06:49:39Z"
      findings: 1
      tree: bb1c56a934525b9a2597f62678eab36654b6c7f7be8fee0e8664d079ff679972
    - "n": 20
      by: claude-code
      at: "2026-11-28T06:59:31Z"
      findings: 2
      tree: 9908a744be1e43e890a001ca6cde4c2c4d4917f447996cf0e1d4dea67fb38833
    - "n": 21
      by: claude-code
      at: "2026-11-28T07:53:44Z"
      findings: 1
      tree: c4633a5d6ab4f6f71289dc96eb8a06344a63b39b02a610cf67a1448d4fa92209
    - "n": 22
      by: claude-code
      at: "2026-11-28T09:44:58Z"
      findings: 2
      tree: 0b162c8729b65b5442a60d711069e63ba4affc3a56686aba5240980e466458de
    - "n": 23
      by: claude-code
      at: "2026-11-28T09:52:07Z"
      findings: 3
      tree: 0b162c8729b65b5442a60d711069e63ba4affc3a56686aba5240980e466458de
    - "n": 24
      by: claude-code
      at: "2026-11-28T10:00:01Z"
      findings: 1
      tree: ca058570454c9bcd1bf74149501985e34513c163f922b713500f6d73407ca938
    - "n": 25
      by: claude-code
      at: "2026-11-28T10:05:43Z"
      findings: 1
      tree: e0e87b400b9a6c388dc3df7bc137c9f7428ef18c2ec59d505b5236021429794f
    - "n": 26
      by: claude-code
      at: "2026-11-28T10:11:18Z"
      findings: 1
      tree: b0b9a8d3f73cab4e42979ee63b0b9eb1a3710d6ecbded286f36855e7452973a0
    - "n": 27
      by: claude-code
      at: "2026-11-28T10:19:24Z"
      findings: 0
      tree: e9342343e5efd7e74ea5f39505c5f84c953661b4d2278a304379ba6e64d1373b
evidence:
  check:
    cmd: go vet ./... && go test -short ./...
    exit_code: 0
    duration_ms: 32391
    summary: |-
      …ternal/compat	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/conv	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doc	[no test files]
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/doctor	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/fsx	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/gates	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/gitx	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/handoff	0.441s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/heal	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/jsonedit	(cached)
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/knowledge	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/mcpserver	0.709s
      ?   	github.com/innaka-tech/ai-toolkit/v2/internal/migrate	[no test files]
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/ops	(cached)
      ok  	github.com/innaka-tech/ai-toolkit/v2/internal/plugins	1.566s
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
    at: "2026-11-28T10:12:03Z"
    tree: e9342343e5efd7e74ea5f39505c5f84c953661b4d2278a304379ba6e64d1373b
  commits:
    - 0ca2b01
    - 0ca960f
    - 1a7016d
    - 1d9ae2e
    - 1f1bcac
    - 1fdc93e
    - 2496d40
    - 433cacd
    - 469218a
    - 4ed1ca1
    - 55cf91d
    - 6650ce0
    - 6ae4851
    - 7b4bd3b
    - 823ee55
    - 835cf8a
    - 88582ea
    - 892a156
    - 8ac10dc
    - 8bb70ee
    - 8fd4b18
    - 961f947
    - 9aa0399
    - b3aa2d8
    - c2e801e
    - c7e68ea
    - d5ea5b1
    - dad3cf3
    - eb4bc02
    - f0f9f97
    - f11e69b
    - f15ea21
    - ff1cf8c
  acceptance:
    total: 4
    done: 4
---

## Objective
(describe the outcome in one paragraph)

## Acceptance criteria
- [x] Given [quality] review = standard and a standard task with no clean review pass, when aitk close runs, then the task goes to in_review with a note to review until a pass finds 0; after aitk review pass --findings 0 and close it is done
- [x] Given an active task that is in_progress or in_review, when an agent starts another task (task start, task next --start, task new --start), then it fails with E_TASK_ACTIVE and says to finish or block it; blocking it or finishing it allows the switch
- [x] Given review passes that stop decreasing, when aitk brief runs, then it says the review is not converging and to ask the user
- [x] Given a project without [quality] review, when a standard task closes, then behaviour is unchanged (only strict needs reviews); aitk init writes review = standard for new projects

## Risk
(Required for strict profile. Impact:, Security (STRIDE):, Rollback:, Validation:)

## Notes

## Evidence
<!-- Generated by aitk close. -->
- Check: `go vet ./... && go test -short ./...` → exit 0 at 2026-11-28T10:12:03Z (32391 ms)
- Acceptance criteria: 4/4 satisfied
- Commits: 0ca2b01, 0ca960f, 1a7016d, 1d9ae2e, 1f1bcac, 1fdc93e, 2496d40, 433cacd, 469218a, 4ed1ca1, 55cf91d, 6650ce0, 6ae4851, 7b4bd3b, 823ee55, 835cf8a, 88582ea, 892a156, 8ac10dc, 8bb70ee, 8fd4b18, 961f947, 9aa0399, b3aa2d8, c2e801e, c7e68ea, d5ea5b1, dad3cf3, eb4bc02, f0f9f97, f11e69b, f15ea21, ff1cf8c
- Review pass 1 by claude-code: 4 findings
- Review pass 2 by claude-code: 5 findings
- Review pass 3 by claude-code: 5 findings
- Review pass 4 by claude-code: 5 findings
- Review pass 5 by claude-code: 4 findings
- Review pass 6 by claude-code: 4 findings
- Review pass 7 by claude-code: 6 findings
- Review pass 8 by claude-code: 4 findings
- Review pass 9 by claude-code: 2 findings
- Review pass 10 by claude-code: 1 findings
- Review pass 11 by claude-code: 1 findings
- Review pass 12 by claude-code: 2 findings
- Review pass 13 by claude-code: 4 findings
- Review pass 14 by claude-code: 2 findings
- Review pass 15 by claude-code: 2 findings
- Review pass 16 by claude-code: 1 findings
- Review pass 17 by claude-code: 2 findings
- Review pass 18 by claude-code: 1 findings
- Review pass 19 by claude-code: 1 findings
- Review pass 20 by claude-code: 2 findings
- Review pass 21 by claude-code: 1 findings
- Review pass 22 by claude-code: 2 findings
- Review pass 23 by claude-code: 3 findings
- Review pass 24 by claude-code: 1 findings
- Review pass 25 by claude-code: 1 findings
- Review pass 26 by claude-code: 1 findings
- Review pass 27 by claude-code: 0 findings

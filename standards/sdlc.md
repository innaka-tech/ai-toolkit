# SDLC: phases, gates, and personas

Every piece of work moves through the same phases; each phase has an entry gate, an owner persona, and evidence that lets the next phase start. Small work moves through them quickly, but none is skipped.

| Phase | Persona | Output (evidence) | Gate to leave |
|---|---|---|---|
| Discover | `/aitk-pm` | Problem, users, outcome metric, `aitk goal add` | The outcome is measurable |
| Analyse | `/aitk-analyst` | Requirements, edge cases, Given/when/then criteria | Definition of Ready (below) |
| Design | `/aitk-architect`, `/aitk-dba`, `/aitk-ux` | ADR (`aitk adr new`), data model, API contract, threat model | Decisions recorded; risks have mitigations |
| Plan | `/aitk-plan` | Tasks with criteria and dependencies | Each task fits one session |
| Build | `/aitk-next`, `/aitk-fix` | Code + tests, `aitk check` green, `aitk impact` reviewed | Definition of Done for the risk profile |
| Verify | `/aitk-review`, `/aitk-qa`, `/aitk-security`, `/aitk-hunt` | Review passes, findings as tasks, `aitk audit` | 0 open high findings; last review pass 0 findings |
| Accept | `/aitk-uat` | UAT script; the user runs `aitk uat accept` | Accepted by a person, never by an agent |
| Release | `/aitk-release`, `/aitk-devops` | Version, changelog, rollback plan, deploy check | Rollback tested or explicitly accepted as a risk |
| Operate | `/aitk-devops`, `/aitk-auditor` | Monitoring, backups verified, audit report, debt ledger | Incidents and debt become tasks |

## Definition of Ready (before Build)
- Objective in one paragraph; who benefits and how we know it worked.
- 1–4 acceptance criteria as observable behaviour (Given/when/then).
- Dependencies and open questions listed; no unverified high-impact assumption.
- Risk level stated (see `aitk standards security` and the task's Risk section for strict work).

## Definition of Done (enforced by `aitk close`)
- Every criterion satisfied; `aitk check` passes on the final code.
- Reviewed until clean: record each pass (`aitk review pass --findings N`), fix, repeat until a pass finds 0 (`[quality] review`). One task at a time: finish or block it before starting another.
- Bug fixes carry a regression test; `aitk impact` shows no uncovered reference left unexplained.
- Strict profile: risk analysis filled, `aitk audit` clean or accepted, two review passes (last with 0 findings, one by a different tool).
- Handoff written with `--summary` and `--knowledge`.

## Change classes (ITIL-style)
- Standard: pre-approved, low risk, repeatable (dependency patch, copy change). Lite profile.
- Normal: anything else. Standard or strict profile, review required.
- Emergency: production down. Fix, then backfill the test, the review, and the post-incident review within one working day.

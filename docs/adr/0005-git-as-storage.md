---
status: accepted
date: 2026-10-09
---

# ADR-0005: Git is the storage; one file per entity; session state stays local

## Context and Problem Statement

aitk must work offline, without servers or accounts, and with several agents and humans working in parallel branches and worktrees. v1 stored the active task in committed files (`current-task.md`, `ai-state.json`), which conflicts across branches and drifted from reality in practice (observed: state said COMPLETED while the task file said IN_PROGRESS).

## Decision Outcome

* Project knowledge is committed as plain files: one file per task, per handoff, and per ADR, so parallel branches rarely touch the same file.
* Session state (which task this worktree is working on, claim locks) is stored in the per-worktree git directory (`$(git rev-parse --git-dir)/aitk/`), never committed.
* All writes are atomic (write temp file, fsync, rename) and serialized by a lock file.

### Consequences

* Good: merge-friendly; no shared mutable pointer in the repository.
* Good: no server, works offline, reviewable in pull requests.
* Bad: "what is everyone working on" requires reading task files across branches (`aitk task list --all-branches`).

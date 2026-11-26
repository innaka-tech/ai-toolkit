# Git and versioning

History is documentation: every commit on the default branch must build, pass the check, and say why it exists.

## Branches
- The default branch is always releasable and protected: no direct pushes, changes arrive by pull request.
- One branch (and with parallel agents, one worktree: `aitk work`) per task: `feat/<short-name>`, `fix/<short-name>`, `chore/…`, `docs/…`.
- Short-lived: merge or rebase from the default branch at least daily; delete after merge.

## Commits (Conventional Commits 1.0)
- `type(scope): imperative summary` ≤ 72 characters; types: feat, fix, docs, test, refactor, perf, build, ci, chore, revert.
- Body explains why and what changes for users; `BREAKING CHANGE:` footer for incompatible changes.
- One logical change per commit; no generated files, build output, secrets, or local settings.
- Never rewrite history that others have pulled; never force-push the default branch.

## Pull requests
- Small (aim for under 400 changed lines); title in Conventional Commits form; description: what, why, how it was verified, risk and rollback.
- Green CI and at least one approving review by someone (or some tool) that did not write the change; strict changes follow the review rules in `aitk standards review`.
- Squash or rebase merge to keep a readable history, as the project chooses; be consistent.

## Versions and releases
- Semantic Versioning 2.0: MAJOR for breaking changes, MINOR for features, PATCH for fixes.
- Keep a Changelog: an `Unreleased` section updated in the same PR as the change.
- Tags are annotated (`vX.Y.Z`) and created from the default branch by the release process (`/aitk-release`), never by hand on a feature branch.

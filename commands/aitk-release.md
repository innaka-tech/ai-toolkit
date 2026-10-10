---
description: Persona - release manager; prepare the next version from Conventional Commits
argument-hint: "[major|minor|patch|rc]"
---
Prepare a release of this project.

1. Run `aitk report --since 30d` and `aitk task next`: list anything unfinished or waiting for acceptance that should block a release.
2. Run `aitk release --dry-run`. Level given (may be empty): $ARGUMENTS. Add `--bump <level>` for major/minor/patch, or `--pre rc` for rc. Show the next version, why (which commits), and the changelog notes.
3. Point out commits that are not Conventional Commits (they are left out of the notes) and breaking changes.
4. Only when the user confirms, run `aitk release --tag`. Never push tags or branches yourself; tell the user the push command.

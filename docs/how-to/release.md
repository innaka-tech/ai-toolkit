# Version and release your project

aitk versions the projects it manages from their Conventional Commits.

```bash
aitk release --dry-run      # next version + changelog section, nothing written
aitk release                # write CHANGELOG.md, manifest versions, ai-state.json; review the diff
aitk release --tag          # … and commit "chore(release): vX.Y.Z" + annotated tag
git push --follow-tags      # publishing stays your decision
```

| Commits since the last tag | Bump |
|---|---|
| `feat!:` or `BREAKING CHANGE:` | major (minor while the version is 0.x) |
| `feat:` | minor |
| `fix:`, `perf:`, `revert:` | patch |
| only `docs`, `test`, `chore`, `ci`, … | nothing to release (or force with `--bump`) |

Pre-releases: `aitk release --tag --pre rc` → `1.4.0-rc.1`, then `-rc.2`; a plain `aitk release --tag` afterwards gives `1.4.0`.

The changelog follows [Keep a Changelog](https://keepachangelog.com) (Added, Changed, Fixed, Security, ⚠ Breaking); each line carries the commit hash and the `AI-Task` id from the commit trailer, so every change traces back to its task, handoff, and evidence. A release is refused with uncommitted code or a failing check, and warns about tasks still in progress or awaiting user acceptance.

Settings:

```toml
[release]
tag_prefix = "v"
changelog = "CHANGELOG.md"
initial_version = "0.1.0"
```

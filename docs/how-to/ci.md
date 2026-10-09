# Gates and CI

Locally:

```bash
aitk hooks install     # pre-commit, commit-msg, pre-push; existing hooks keep running
aitk hooks uninstall
```

- **pre-commit** blocks secrets in added lines and invalid aitk files. Mark a false positive with a trailing `aitk:allow-secret` comment.
- **commit-msg** requires [Conventional Commits](https://www.conventionalcommits.org) and adds `AI-Task:` / `AI-Tool:` trailers while a task is active.
- **pre-push** runs the check when a task is active.

In CI, where hooks cannot be skipped:

```yaml
# .github/workflows/aitk.yml
on: [pull_request]
jobs:
  aitk:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: innaka-tech/ai-toolkit/action@v2
```

Other CI systems: install aitk and run `aitk ci` (it reads `GITHUB_BASE_REF` or `CI_MERGE_REQUEST_TARGET_BRANCH_NAME`, or use `--base <ref>`).

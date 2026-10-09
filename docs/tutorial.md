# Tutorial: your first project with aitk

Time: about 10 minutes. You need git and aitk ([install](../README.md#install)).

## 1. Initialize

```bash
mkdir greeter && cd greeter && git init -q
printf 'test:\n\t@test -f hello.txt\n' > Makefile
git add -A && git commit -qm "chore: start"
aitk init
git add -A && git commit -qm "chore: aitk init"
```

`init` detects `make test` as the check command and creates `aitk.toml`, `ai-state.json`, `AGENTS.md` (with the aitk block), `.gitattributes`, and `docs/ai/`. Running it again changes nothing.

## 2. Plan a task

```bash
aitk task new "Say hello" --ac "Given the repo, when make test runs, then it passes" --start
aitk brief
```

The brief is what every agent reads first: the active task, its open criteria, the last handoff, relevant knowledge, the rules for this task's risk profile, and the next commands.

## 3. Do the work and verify it

```bash
aitk close --summary "Added hello.txt" --knowledge none
# error[E_DOD_CHECK_STALE]: no check has been recorded for this task
# fix: aitk check

aitk check                 # fails: hello.txt does not exist yet
echo hello > hello.txt
aitk check                 # passes; evidence is recorded on the task
aitk close --summary "Added hello.txt" --knowledge "make test requires hello.txt"
# error[E_DOD_ACCEPTANCE]: acceptance criteria not yet satisfied: 1
# fix: aitk task update T-xxxx --ac-done 1

aitk task update --ac-done 1
aitk close --summary "Added hello.txt" --knowledge "make test requires hello.txt"
# T-xxxx → done (profile lite)
```

`close` wrote evidence into the task file, a handoff in `docs/ai/handoff/`, and the knowledge entry. Commit it all:

```bash
git add -A && git commit -qm "feat: say hello"
```

## 4. Let your AI tools use it

```bash
aitk adapters sync     # registers the MCP server and session hooks for installed tools
aitk hooks install     # secret scan, schema checks, Conventional Commits, pre-push check
```

Open Claude Code, Codex, OpenCode, or Gemini CLI in the repository. They read `AGENTS.md`, start from `aitk brief`, and finish with `aitk close`, through the CLI or the MCP tools.

## 5. Next

- Hand a task to another tool: `aitk switch codex`
- Work on several tasks at once: [parallel work](how-to/parallel.md)
- See what happened this week: `aitk report --since 7d`

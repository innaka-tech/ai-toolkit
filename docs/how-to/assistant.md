# Automatic assistant: character, talk, and personas

Every agent in every tool starts a session with `aitk brief` (AGENTS.md, the SessionStart hook, and the MCP server's instructions make sure of it). aitk puts the working character, the reply style, and the personas the task needs into that brief, so they apply without anyone typing a slash command.

```toml
# aitk.toml
[assistant]
character = "full"    # lite | full | ultra | off
talk = "normal"       # normal | lite | caveman
auto_persona = true   # the brief picks the personas and standards the task needs
```

| Setting | Default | Effect in the brief |
|---|---|---|
| `character` | `full` | `lite`: propose the smaller option and let the user pick. `full`: smallest change that fully works, never cutting safety. `ultra`: challenge the request first. `off`: no character rules. All but `off` end replies with **Skipped:** and **Risk:**. |
| `talk` | `normal` | `lite`: terse sentences. `caveman`: short, blunt replies. Code, commands, paths, errors, warnings, questions for the user, and Skipped/Risk are never shortened. |
| `auto_persona` | `true` | Lists the personas the active task needs and tells the agent to run them before close. |

One person can use a different style without changing the team's file:

```bash
export AITK_TALK=caveman AITK_CHARACTER=ultra
```

## When a persona runs

| Signal | Persona | Standards |
|---|---|---|
| No active task | `aitk-team` (finds the phase, routes the request) | sdlc |
| Task tagged `bug` | `aitk-fix` (failing test first) | testing |
| Task tagged `security`, or a strict task | `aitk-security` | security, review |
| Migrations, `.sql`, `schema.prisma` changed | `aitk-dba` | database |
| `.tsx`, `.jsx`, `.vue`, `.svelte`, `.css`, `.html`, Blade changed | `aitk-ux` | ux |
| Dockerfile, compose, CI workflows, Terraform, Helm, k8s changed | `aitk-devops` | operations |
| OpenAPI/Swagger or `routes/` changed | `aitk-architect` | api |
| Code changed with no test change (standard or strict profile) | `aitk-qa` | testing |

The brief shows each one as a command, for example `aitk prompt aitk-dba fix`. The agent prints the prompt and follows it, in a subagent if its tool can start one. The signals are simple file and tag rules, so you can predict them; turn the list off with `auto_persona = false`.

Not automatic, on purpose: the independent review of strict work (`/aitk-review`) must run in a different tool than the one that built the change, and only a person accepts UAT.

Limits: character and talk are instructions to the model, not gates; how closely an agent follows them depends on the model. Each picked persona adds a step (time and tokens) to the task that triggered it.

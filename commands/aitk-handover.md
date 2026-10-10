---
description: Hand the current task over to another session, tool, or person
argument-hint: "[who takes over, e.g. codex]"
---
Hand over the active task without finishing it.

1. Run `aitk check` so the handoff records the current state.
2. Run `aitk close --status in_progress --summary "<what is done, what is not, why>" --knowledge "<what the next person should know, or none>" --next "<first concrete step>" --next "<second step>"`.
3. If a tool was named ($ARGUMENTS), also run `aitk switch <tool> --print` and show the command it prints.
Keep the next steps concrete enough that a different model can continue without asking.

---
description: Hand the current task over to another session, tool, or person
argument-hint: "[who takes over, e.g. codex]"
---
Hand over the active task without finishing it.

Tool to hand over to (may be empty): $ARGUMENTS

1. Run `aitk check` so the record shows the current state.
2. If a tool was named: run `aitk switch <tool> --note "<what is done, what is not, the first concrete next step>" --print` and show the command it prints (it writes the handoff).
3. Otherwise run `aitk close --status in_progress --summary "<what is done, what is not, why>" --knowledge "<what the next person should know, or none>" --next "<first concrete step>" --next "<second step>"`.
Keep the next steps concrete enough that a different model can continue without asking.

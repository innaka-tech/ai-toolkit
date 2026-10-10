---
description: Project status - what was done, what is in progress, what is ready, what waits for a person
argument-hint: "[since, e.g. 7d]"
---
Summarise the project's state for the user.
Period given (may be empty): $ARGUMENTS

1. Run `aitk report --since <period>` with that period (7d when none was given), and `aitk task next`.
2. Report in short sections: done (with which tool), in progress, blocked (and why), waiting for the user (review or `aitk uat accept`), ready next.
3. End with the single most useful next action.

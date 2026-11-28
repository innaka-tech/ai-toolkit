# Knowledge inbox

<!-- New entries land here; `aitk knowledge compact` files them by topic. -->

- aitk run must re-derive outcomes from a pre-attempt snapshot plus its own check/audit, because agents can write aitk's own files <!-- aitk:k date=2026-10-09 task=T-9ycc tags=autopilot -->
- Built-in standards live in standards/*.md (embedded) and are overridden per project by docs/ai/standards/<topic>.md, mirroring how commands/ and docs/ai/commands/ work <!-- aitk:k date=2026-11-26 task=T-7m0j -->
- cobra StringVar silently keeps only the last value of a repeated flag; list-like flags must be StringArray <!-- aitk:k date=2026-11-26 task=T-cy5a tags=bug -->
- A task's changes are measured from the newest merge base of local and origin default branch; gates.DefaultBase (CI) prefers origin/ on purpose <!-- aitk:k date=2026-11-27 task=T-5w2x tags=bug -->
- Everything that must apply automatically to every agent goes into aitk brief: every tool starts there (AGENTS.md, SessionStart hook, MCP instructions) <!-- aitk:k date=2026-11-27 task=T-gt80 -->
- aitk run's guard must fail closed and re-derive from a per-task snapshot; every new probe class came from trusting something the agent can write (files, links, git config/attributes, index, sequencer state, unprotected task fields). Treat aitk run as raising cost, not as a sandbox. <!-- aitk:k date=2026-11-28 task=T-3918 tags=security -->
- one_task is a workflow aid, not the quality gate: trust the close and let acceptance/close re-apply the DoD; every rule that tried to re-judge the worktree from the current tree found new edge cases. Handoff order within one second must come from the write number in the file name, applied as a second pass after a total order (a mixed comparator is not transitive). <!-- aitk:k date=2026-11-28 task=T-tzq8 -->
- Slop rules stay narrow and advisory: a repo-wide scan of aitk itself must stay at zero hits, or the rule is too broad <!-- aitk:k date=2026-11-27 task=T-yy6r -->
- Everything aitk generates for AI tools must be Managed, or installing it makes the next task look standard and the check stale. <!-- aitk:k date=2026-11-28 task=T-cc2p -->
- Turn new safety behaviour on by default through the absent key, not by writing the key: older aitk rejects unknown keys (E_SCHEMA), but ignores a key that is not there. <!-- aitk:k date=2026-11-28 task=T-3bp7 -->

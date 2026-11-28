# Knowledge inbox

<!-- New entries land here; `aitk knowledge compact` files them by topic. -->

- aitk run must re-derive outcomes from a pre-attempt snapshot plus its own check/audit, because agents can write aitk's own files <!-- aitk:k date=2026-10-09 task=T-9ycc tags=autopilot -->
- Built-in standards live in standards/*.md (embedded) and are overridden per project by docs/ai/standards/<topic>.md, mirroring how commands/ and docs/ai/commands/ work <!-- aitk:k date=2026-11-26 task=T-7m0j -->
- cobra StringVar silently keeps only the last value of a repeated flag; list-like flags must be StringArray <!-- aitk:k date=2026-11-26 task=T-cy5a tags=bug -->
- A task's changes are measured from the newest merge base of local and origin default branch; gates.DefaultBase (CI) prefers origin/ on purpose <!-- aitk:k date=2026-11-27 task=T-5w2x tags=bug -->
- Everything that must apply automatically to every agent goes into aitk brief: every tool starts there (AGENTS.md, SessionStart hook, MCP instructions) <!-- aitk:k date=2026-11-27 task=T-gt80 -->
- Slop rules stay narrow and advisory: a repo-wide scan of aitk itself must stay at zero hits, or the rule is too broad <!-- aitk:k date=2026-11-27 task=T-yy6r -->
- Git tracks files behind an in-repo directory symlink under the link target and refuses pathspecs through the link: map aitk paths with Project.GitRel/GitPath before passing them to git, and git's paths back with AitkRel before comparing with aitk's names. <!-- aitk:k date=2026-11-28 task=T-bq2a -->

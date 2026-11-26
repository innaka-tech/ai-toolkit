# Documentation

Write the document the next person needs at 2 a.m.: short, current, and next to the code it describes.

| Document | Purpose | Rule |
|---|---|---|
| README.md | What it is, how to run, test, and deploy it | A new developer runs the project from it in under 30 minutes |
| docs/ai/project-context.md | Purpose, stack, constraints for agents | Under 60 lines; first three lines say what and for whom |
| docs/ai/conventions.md | The project's own coding rules | Filled in, not the template (`aitk doctor` warns) |
| ADR (`aitk adr new`) | Why a significant decision was made | One decision per record: context, options, decision, consequences |
| API spec | The contract (see `aitk standards api`) | Generated docs come from the spec, not the other way round |
| Runbook | How to operate and recover | Copy-pasteable commands; tested during drills |
| CHANGELOG.md | What changed for users | Keep a Changelog; updated in the same PR |

## Style
- Lead with the conclusion; then steps; then background.
- Plain language, active voice, present tense. Define an acronym once.
- Commands in fenced code blocks that run as written; no placeholder that silently breaks.
- Diagrams as text where possible (Mermaid) so they are reviewed in diffs.
- Delete documentation that is wrong; outdated docs are worse than none.

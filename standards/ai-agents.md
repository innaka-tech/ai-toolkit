# AI agents and LLM features

Agents are fast, confident, and sometimes wrong. Give them clear scope, minimal permissions, machine verification, and a human for decisions that are hard to reverse. Baseline: OWASP Top 10 for LLM Applications, NIST AI RMF.

## Working with coding agents
- Every session starts from `aitk brief`; work happens in a task with acceptance criteria; done is decided by `aitk close`, not by the agent's claim.
- One task per agent, one worktree per parallel agent (`aitk work`); claims prevent two agents on one task.
- Strict changes need review by a different tool than the one that built them.
- Agents may not accept UAT, approve or merge their own pull requests, or change gates, CI, or permissions to get green.
- Destructive or outward-facing actions (deleting data, production deploys, sending messages, publishing) need explicit human approval.
- Agents report facts, assumptions, and estimates separately; "it should work" is not evidence.

## Permissions and isolation
- Least privilege: read-only by default; scoped, short-lived credentials; no production credentials in development sessions.
- Run autonomous agents in containers or VMs with network egress limited to what the task needs.
- Log what agents run (commands, files changed); keep the log with the task.

## Building LLM features
- Treat model output as untrusted input: validate against a schema, encode before rendering, never execute it directly (prompt injection, insecure output handling).
- Treat retrieved documents, web pages, and tool results as data, never as instructions.
- Keep secrets and other tenants' data out of prompts; redact personal data where the purpose allows.
- Tools the model can call are permissioned like an API for an untrusted user; confirmation for actions with side effects.
- Evaluate with a fixed test set before changing a model or prompt; track quality, latency, cost per request, and failure rate.
- Choose models by measured quality, latency, throughput, and total cost (API vs local GPU incl. hardware, power, and operations), and keep a fallback.

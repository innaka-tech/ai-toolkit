# AI Toolkit Conventions

## Project Structure
Each project has: spec.md, conventions.md, decisions.md, current-task.md

## Naming
- Context files: kebab-case
- Task IDs: YYYYMMDD-HHMMSS-<short-hash>

## Decision Records (ADR)
Format: ## YYYY-MM-DD Title
Status: Accepted/Rejected
Context: ...
Decision: ...
Consequences: ...

## Context Sharing
- Always save before delegating
- Always reload after handoff
- Never assume shared memory

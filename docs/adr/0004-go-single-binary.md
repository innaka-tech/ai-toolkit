---
status: accepted
date: 2026-10-09
---

# ADR-0004: Rewrite the core in Go as one binary

## Context and Problem Statement

v1 is ~3.7k lines of bash that edits Markdown and JSON with awk/sed/heredocs. It has no tests beyond a smoke test, is fragile across macOS/Linux, and cannot run natively on Windows. An MCP server would need a second runtime.

Considered options: keep bash; TypeScript/Node; Python; Rust; Go.

## Decision Outcome

Write v2 in Go: a single static binary that contains the CLI, the MCP server (official `modelcontextprotocol/go-sdk`), and the tool adapters. Release with GoReleaser. Freeze bash at v1.0.0.

### Consequences

* Good: native macOS/Linux/Windows; no runtime dependency; fast startup (target: `brief` < 200 ms).
* Good: one artifact to sign, checksum, and package (Homebrew, Scoop, npm wrapper).
* Good: strong standard library for JSON, file locking, and testing (golden files).
* Bad: full rewrite; mitigated by a written spec, golden tests derived from v1 behavior, and shims.
* Rejected TypeScript/Python: require a runtime on every machine. Rejected Rust: slower iteration for this scope with no offsetting benefit.

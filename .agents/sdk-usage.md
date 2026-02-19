# SDK Usage Guide

Last Updated: 2026-02-19
Related Files: `USAGE.md`, `agent.md`, `src/commands_session.go`, `src/commands_agent.go`

## Overview

How to use Lisa as infrastructure from an LLM orchestrator or script.

## Integration Pattern

1. **Spawn** one session per task (`session spawn --json`), store returned session name (custom `--session` values must start with `lisa-`)
2. **Poll** with `session monitor --json` (blocking loop) or `session status --json` (one-shot)
3. If state is `stuck`, send next instruction with `session send --text "..." --enter`; if state is `degraded`, retry polling and inspect `signals.*Error`
4. Fetch artifacts with `session capture --lines N`
   - Raw capture now suppresses known Codex/MCP startup noise by default; use `session capture --raw --keep-noise` to keep full raw output
5. Kill and clean up with `session kill --session NAME`

Nested Codex note: `codex exec --full-auto` runs sandboxed and can block tmux socket creation for child Lisa sessions. For deep nested orchestration (L1->L2->L3), prefer interactive sessions (`--mode interactive` + `session send`) unless you intentionally use unsandboxed Codex exec via `--agent-args '--dangerously-bypass-approvals-and-sandbox'`.

Manual nested smoke command: run `./smoke-nested` from repo root to validate L1->L2->L3 interactive nesting end-to-end with deterministic markers.

`session exists` now also accepts `--project-root` for explicit socket/project routing.

## Command Contract Source

All CLI usage details live in `USAGE.md`:

- command syntax and flags
- state definitions
- exit-code contract
- JSON vs text output modes
- environment variable controls

## Related Context

- @AGENTS.md
- @src/AGENTS.md

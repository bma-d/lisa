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

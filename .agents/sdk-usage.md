# SDK Usage Guide

Last Updated: 2026-02-20
Related Files: `USAGE.md`, `agent.md`, `src/commands_session.go`, `src/commands_agent.go`

## Overview

How to use Lisa as infrastructure from an LLM orchestrator or script.

## Integration Pattern

1. **Spawn** one session per task (`session spawn --json`), store returned session name (custom `--session` values must start with `lisa-`)
2. **Poll** with `session monitor --json` (blocking loop) or `session status --json` (one-shot)
   - For stricter interactive stop semantics, use `session monitor --stop-on-waiting true --waiting-requires-turn-complete true` to stop only after transcript turn completion is detected
3. If state is `stuck`, send next instruction with `session send --text "..." --enter`; if state is `degraded`, retry polling and inspect `signals.*Error`
4. Fetch artifacts with `session capture --lines N`
   - Raw capture now suppresses known Codex/MCP startup noise by default (including MCP OAuth refresh/auth-failure startup noise); use `session capture --raw --keep-noise` to keep full raw output
   - Claude transcript capture now requires session metadata to include prompt + createdAt; promptless/custom-command sessions automatically fall back to raw pane capture
5. Kill and clean up with `session kill --session NAME`

Nested Codex note: `codex exec --full-auto` runs sandboxed and can block tmux socket creation for child Lisa sessions. For deep nested orchestration (L1->L2->L3), prefer interactive sessions (`--mode interactive` + `session send`) or bypass mode.
Lisa now auto-enables Codex bypass (`--dangerously-bypass-approvals-and-sandbox`, no `--full-auto`) when exec prompts suggest nesting (`./lisa`, `lisa session spawn`, `nested lisa`).
You can still pass `--agent-args '--dangerously-bypass-approvals-and-sandbox'` explicitly; Lisa omits `--full-auto` automatically because Codex rejects combining both flags.
For deeply nested prompt chains, prefer heredoc prompt injection (`PROMPT=$(cat <<'EOF' ... EOF)` then `--prompt "$PROMPT"`) instead of highly escaped inline single-quoted chains.

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

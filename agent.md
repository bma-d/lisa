# Lisa Agent SDK Guide

`Lisa` is a standalone Go CLI for orchestrating Claude/Codex sessions in tmux.

Use it when an orchestrator needs to:
- Spawn concurrent AI worker sessions.
- Poll runtime state without attaching tmux.
- Send follow-up input into active sessions.
- Capture transcript/pane output for downstream automation.

## Quick Start

```bash
go build -o lisa .
./lisa doctor
./lisa version
```

CLI command syntax/flags/examples: `USAGE.md` (single source of truth).

## Code Layout

- `main.go`: process entrypoint.
- `src/run.go`: top-level command routing.
- `src/help.go`: per-command help text.
- `src/commands_agent.go`: `doctor`, `agent build-cmd`.
- `src/commands_session*.go`: session command handlers.
- `src/agent_command.go`: startup command construction.
- `src/tmux.go`: tmux and process-tree interactions.
- `src/status.go`, `src/status_helpers.go`: state classification.
- `src/session_files.go`: naming + artifact pathing + persistence.
- `src/session_wrapper.go`: run-id wrapper, heartbeat, done sidecar.
- `src/session_observability.go`: event logging and lock/retention logic.
- `src/claude_session.go`, `src/codex_session.go`: transcript/session-id helpers.

## Core Contract

- Spawn via `session spawn --json`; persist `session` id.
- Poll with `session monitor --json` or `session status --json`.
- Send follow-up with `session send`.
- Capture output with `session capture`.
- Cleanup with `session kill` / `session kill-all`.

Exact command forms and flags: `USAGE.md`.

## Integration Pattern

1. Spawn per task (`session spawn --json`) and keep `session` id.
2. Poll with `session monitor` or `session status`.
3. On `stuck`, send next instruction. On `degraded`, keep polling and inspect `signals.*Error`.
4. Capture transcript/pane output with `session capture`.
5. Kill sessions when complete.

## Exit Behavior

- See `USAGE.md` for CLI exit-code contract.

## Runtime Assumptions

- tmux server available to current user.
- At least one of `claude` or `codex` on `PATH`.
- Repository/project root readable and writable by spawned agent process.

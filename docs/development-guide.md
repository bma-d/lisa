# Lisa - Development Guide

## Prerequisites

- Go 1.21+
- tmux on `PATH`
- Claude and/or Codex on `PATH` (at least one agent for `doctor: ready`)
- macOS or Linux

## Build

```bash
go build -o lisa .
./lisa version
./lisa doctor
```

## Test Matrix

```bash
# default suite
go test ./...

# race detection
go test -race ./...

# coverage snapshot
go test -cover ./src

# optional real-agent e2e
LISA_E2E_CLAUDE=1 go test ./src -run TestE2EClaudeRunsEntireSuiteWithAgentsContext -count=1 -v
LISA_E2E_CODEX=1 go test ./src -run TestE2ECodexRunsEntireSuiteWithAgentsContext -count=1 -v
```

Hermetic lifecycle e2e tests (`e2e_interactive_fake_test.go`, `e2e_exec_fake_test.go`) run in normal suite when tmux is available.

## Core Command Flow

Canonical command usage (flags/examples/states/exit codes): `USAGE.md`.

## `src/` File Groups

- Routing/help: `run.go`, `help.go`
- Agent commands: `commands_agent.go`, `agent_command.go`
- Session commands: `commands_session*.go`
- Runtime state: `status.go`, `status_helpers.go`, `types.go`
- tmux/process: `tmux.go`
- Session artifacts/locks/events: `session_files.go`, `session_observability.go`, `session_wrapper.go`
- Transcript paths: `claude_session.go`, `codex_session.go`
- Shared helpers: `utils.go`, `build_info.go`

## Behavior Controls (Env Vars)

- Timeouts/intervals:
- `LISA_CMD_TIMEOUT_SECONDS`
- `LISA_PROCESS_SCAN_INTERVAL_SECONDS`
- `LISA_PROCESS_LIST_CACHE_MS`
- Staleness:
- `LISA_OUTPUT_STALE_SECONDS`
- `LISA_HEARTBEAT_STALE_SECONDS`
- Locking:
- `LISA_STATE_LOCK_TIMEOUT_MS`
- `LISA_EVENT_LOCK_TIMEOUT_MS`
- Event retention:
- `LISA_EVENTS_MAX_BYTES`
- `LISA_EVENTS_MAX_LINES`
- `LISA_EVENT_RETENTION_DAYS`
- Cleanup scope:
- `LISA_CLEANUP_ALL_HASHES`
- Agent process matching overrides:
- `LISA_AGENT_PROCESS_MATCH`
- `LISA_AGENT_PROCESS_MATCH_CLAUDE`
- `LISA_AGENT_PROCESS_MATCH_CODEX`
- E2E toggles:
- `LISA_E2E_CLAUDE`
- `LISA_E2E_CODEX`

## Conventions

- stdlib only; no third-party runtime deps.
- Manual flag parsing loops for command handlers.
- JSON output for machine-integrated commands (`doctor`, `agent build-cmd`, `session spawn|send|status|monitor|capture|explain`).
- Function-variable seams for test mocking external interactions.
- Atomic writes + private perms for session metadata/state artifacts.

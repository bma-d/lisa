# Lisa - Architecture

## High-Level Model

Lisa is a single-process CLI (`main.go` -> `src.Run`) with command routing into domain handlers. Runtime work is delegated to tmux sessions. Session state is inferred by a process-first classifier and persisted in hash-scoped files under `/tmp`.

```text
caller
  -> lisa CLI router
    -> session/agent/doctor handlers
      -> tmux layer + file artifacts + status classifier
```

## Command Routing

- `src/run.go`: top-level command switch.
- `src/commands_agent.go`: `doctor`, `agent build-cmd`.
- `src/commands_session.go`: router + `name|spawn|send`.
- `src/commands_session_state.go`: `status|monitor|capture`.
- `src/commands_session_manage.go`: `list|exists|kill|kill-all`.
- `src/commands_session_explain.go`: diagnostics (`status + event tail`).
- `src/help.go`: per-command help text.

## Session Lifecycle

```text
spawned -> in_progress -> completed
                     -> crashed
                     -> stuck
                     -> degraded
                     -> just_started (grace)
```

Notes:
- `waiting_input` remains in payload compatibility but is non-emitting in default classifier behavior.
- `monitor` exits `0` on `completed` (and `waiting_input` if ever emitted), else `2` for terminal failure/timeout paths.

## State Classification (Process-First)

`computeSessionStatus()` in `src/status.go` evaluates signals in priority order:

1. tmux pane terminal state (`crashed:N` / `exited:N`).
2. done sidecar (`.done.txt`) matching run-id and exit code.
3. active agent PID detection from pane PID process tree.
4. heartbeat freshness (`.heartbeat.txt` mtime).
5. pane command class (non-shell commands imply activity).
6. infra error fallbacks (`done_file_read_error`, `agent_scan_error`, `state_lock_timeout`, tmux read errors).
7. grace period (`just_started` for early polls) then `stuck` fallback.

## Session Artifacts

All artifact names are project-hash scoped (`projectHash(canonicalProjectRoot)`):

- Meta: `/tmp/.lisa-{hash}-session-{id}-meta.json`
- State: `/tmp/.lisa-{hash}-session-{id}-state.json`
- Output: `/tmp/lisa-{hash}-output-{id}.txt`
- Heartbeat: `/tmp/.lisa-{hash}-session-{id}-heartbeat.txt`
- Done: `/tmp/.lisa-{hash}-session-{id}-done.txt`
- Events: `/tmp/.lisa-{hash}-session-{id}-events.jsonl`
- Locks/count sidecars: `.lock`, `.lines`
- Long command scripts: `/tmp/lisa-cmd-{hash}-{id}-{nanos}.sh`

Cleanup defaults to hash-scoped removal; cross-hash cleanup is opt-in (`--cleanup-all-hashes` or `LISA_CLEANUP_ALL_HASHES`).

## Tmux Layer

`src/tmux.go` centralizes tmux interaction:

- Session create with runtime env injection (`LISA_SESSION*`, `LISA_AGENT`, `LISA_MODE`, `LISA_PROJECT_HASH`, done/heartbeat paths).
- Command send strategy:
- Inline `send-keys` for short commands.
- Script fallback for long commands.
- Buffer-based paste for multi-line `--text`.
- Process detection via `ps` scan + BFS child traversal from pane PID.
- Short-lived process table cache (`LISA_PROCESS_LIST_CACHE_MS`).

## Wrapper + Observability

- `src/session_wrapper.go` wraps startup command with:
- run-id markers
- heartbeat loop
- EXIT trap done marker
- signal trap handling for interrupts
- `src/session_observability.go` manages:
- state file locks (`LISA_STATE_LOCK_TIMEOUT_MS`)
- event file shared/exclusive locks (`LISA_EVENT_LOCK_TIMEOUT_MS`)
- bounded event trimming (`LISA_EVENTS_MAX_BYTES`, `LISA_EVENTS_MAX_LINES`)
- stale event retention cleanup (`LISA_EVENT_RETENTION_DAYS`)

## Transcript Handling

- Claude transcript discovery + parsing: `src/claude_session.go`.
- Codex transcript turn-complete probing: `src/codex_session.go`.
- `session capture` behavior:
- default: Claude transcript when metadata resolves to Claude and transcript read succeeds.
- fallback: raw tmux capture.
- force raw: `--raw`.

## Testability Strategy

External interactions are bound through function variables (`var fooFn = foo`) across tmux/status/files/observability paths, enabling deterministic unit/regression tests without live tmux for most cases.

## Reliability Posture

- Atomic writes for structured files.
- Private artifact permissions (`0600`).
- Per-command timeout for shell invocations (`LISA_CMD_TIMEOUT_SECONDS`).
- Extensive regression suite + race test + hermetic e2e lifecycle tests.

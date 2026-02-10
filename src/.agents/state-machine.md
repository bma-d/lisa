# State Machine & Status Classification

Last Updated: 2026-02-10
Related Files: `src/status.go`, `src/types.go`

## Overview

`computeSessionStatus()` is the core function. It combines multiple signals to classify a session into one of: `in_progress`, `waiting_input`, `completed`, `crashed`, `stuck`, `degraded`, `just_started`.

## Signal Sources

1. **Pane status snapshot** (`readPaneSnapshot`): prefers a single tmux format query (`pane_dead`, `pane_dead_status`, `pane_current_command`, `pane_pid`) with fallback to per-field reads
2. **Agent process** (`detectAgentProcess`): BFS walk from pane PID through process tree, matches "claude"/"codex" in command string, returns PID + CPU%; cached between polls via `LISA_PROCESS_SCAN_INTERVAL_SECONDS` (default 8s)
3. **Output freshness**: MD5 hash of captured pane output compared to last known hash; stale after `LISA_OUTPUT_STALE_SECONDS` (default 240s). Updates are monotonic by nanosecond capture timestamp so older concurrent polls cannot overwrite newer freshness state.
4. **Prompt detection** (`looksLikePromptWaiting`): agent-specific regex patterns on last output line
   - Claude: trailing `>` or `›`, or "press enter to send"
   - Codex: `❯` with timestamp pattern, or "tokens used"
5. **Session completion** (`parseSessionCompletionForRun`): searches for `__LISA_SESSION_DONE__:{runID}:N` markers and validates against metadata `runId`
6. **Exec completion** (`parseExecCompletion`): searches for `__LISA_EXEC_DONE__:N` marker
7. **Heartbeat freshness**: reads `/tmp/.lisa-*-heartbeat.txt` mtime (`LISA_HEARTBEAT_FILE`), stale after `LISA_HEARTBEAT_STALE_SECONDS` (default 8s)
8. **Todo progress** (`parseTodos`): counts `[x]`/`[ ]` checkboxes in output
9. **State lock observability**: lock wait timing in `signals.stateLockWaitMs`, timeout fallback to `state_lock_timeout` classification
10. **Structured signals**: status payload includes `classificationReason` + `signals` block for observability and debugging
11. **Process scan errors**: `signals.agentScanError` captures `ps`/scan failures; classification falls back to `degraded` (`agent_scan_error`) when no stronger activity signals exist
12. **Capture fallback**: when pane capture fails, pane terminal status (`exited`/`crashed`) still takes precedence so completed/crashed sessions are not misclassified as degraded

## Classification Priority

```
pane crashed/exited → immediate terminal state
session done marker (matching runId) → completed/crashed based on exit code
exec mode + exec done marker → completed/crashed based on exit code
interactive waiting (low CPU + stale output) OR prompt regex → waiting_input
agent PID alive OR output fresh OR heartbeat fresh OR non-shell pane command → in_progress
process scan failure with no stronger activity signals → degraded
tmux read/snapshot/pid parse failures → degraded (non-fatal payload)
poll count ≤ 3 → just_started (grace period)
else → stuck
```

Additional fallback:
- state lock timeout (`LISA_STATE_LOCK_TIMEOUT_MS`, default 2500ms) -> `degraded` with reason `state_lock_timeout` (non-fatal status payload)

Infra observability signals:
- `signals.metaReadError` when metadata read/parse fails
- `signals.stateReadError` when state file read/parse fails
- `signals.eventsWriteError` when event append fails
- `signals.agentScanError` when process scan fails
- `signals.tmuxReadError` when tmux capture/snapshot/pid parsing fails

## Wait Estimation

`estimateWait()` uses keyword matching on active task text and todo progress percentage to return estimated seconds remaining. Range: 30-120s.

## State Persistence

`sessionState` struct saved to `/tmp/` between polls: tracks output freshness, poll counters, and last classification.  
`/tmp/.lisa-*-events.jsonl` receives snapshot/transition events per status computation and is bounded by `LISA_EVENTS_MAX_BYTES`/`LISA_EVENTS_MAX_LINES`. Event writes happen outside the state-file lock to keep lock hold-times short.

## Related Context

- @src/AGENTS.md

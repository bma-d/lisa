# Tmux Interaction Layer

Last Updated: 2026-02-10
Related Files: `src/tmux.go`

## Overview

All tmux interactions are wrapped in Go functions. No direct tmux commands elsewhere in codebase.
External command wrappers are now bounded by `LISA_CMD_TIMEOUT_SECONDS` (default 20s) to prevent hung `tmux`/`ps` calls from stalling status/monitor loops.

## Session Creation

`tmuxNewSession()` creates detached session with:
- Custom dimensions (`-x`, `-y`)
- Working directory (`-c projectRoot`)
- Environment variables: `LISA_SESSION`, `LISA_SESSION_NAME`, `LISA_AGENT`, `LISA_MODE`, `LISA_PROJECT_HASH`, `LISA_HEARTBEAT_FILE`

Spawn path now hard-fails before session creation if heartbeat artifact path cannot be prepared.

## Command Sending Strategies

| Strategy | When | Implementation |
|----------|------|----------------|
| `send-keys` | Command ≤ 500 chars | `tmuxSendKeys()` — direct key injection |
| Script fallback | Command > 500 chars | Write to `/tmp/lisa-cmd-*.sh`, send `bash <script>` |
| Buffer paste | `--text` input | `tmuxSendText()` — `load-buffer` + `paste-buffer` (safe for multi-line) |

## Key Functions

- `tmuxSendCommandWithFallback()`: auto-selects inline vs script strategy
- `buildFallbackScriptBody()`: wraps command in bash script, uses `set +e` when exec markers present
- `tmuxSendText()`: uses named tmux buffer with nano-timestamp for uniqueness, auto-cleanup via defer
- `tmuxCapturePane()`: captures N lines from pane via `capture-pane -p -S -N`
- `tmuxDisplay()`: queries tmux format strings (`pane_dead`, `pane_pid`, etc.)
- `tmuxPaneStatus()`: combines `pane_dead` + `pane_dead_status` into alive/exited/crashed
- `tmuxListSessions()`: normalizes tmux "no server running"/"no sessions" errors to an empty list so list/kill-all behave correctly when no tmux sessions exist

## Process Detection

`detectAgentProcess()`: given pane PID, does BFS through process tree (`ps -axo pid=,ppid=,%cpu=,command=`), finds child process matching agent name. Returns best match by CPU usage; no-match returns `(0, 0)` and `ps` failures return an error surfaced via `signals.agentScanError`.

Process-table reads are shared through a short-lived cache (`LISA_PROCESS_LIST_CACHE_MS`, default 500ms) to reduce repeated full `ps` scans across concurrent status polls.
Cache stores successful scans only; failed scans are not cached so status polling retries `ps` immediately on the next probe.

Process matching supports defaults (`claude`/`anthropic` for Claude sessions, `codex`/`openai` for Codex sessions) plus optional custom substrings via:
- `LISA_AGENT_PROCESS_MATCH` (applies to both agents)
- `LISA_AGENT_PROCESS_MATCH_CLAUDE`
- `LISA_AGENT_PROCESS_MATCH_CODEX`

## Mocking

All key functions have `var fooFn = foo` pattern for test substitution:
`tmuxShowEnvironmentFn`, `tmuxHasSessionFn`, `tmuxKillSessionFn`, `tmuxListSessionsFn`, `tmuxCapturePaneFn`, `tmuxDisplayFn`, `tmuxPaneStatusFn`, `detectAgentProcessFn`

## Related Context

- @src/AGENTS.md

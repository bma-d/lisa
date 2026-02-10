# Tmux Interaction Layer

Last Updated: 2026-02-09
Related Files: `src/tmux.go`

## Overview

All tmux interactions are wrapped in Go functions. No direct tmux commands elsewhere in codebase.

## Session Creation

`tmuxNewSession()` creates detached session with:
- Custom dimensions (`-x`, `-y`)
- Working directory (`-c projectRoot`)
- Environment variables: `LISA_SESSION`, `LISA_AGENT`, `LISA_MODE`, `LISA_PROJECT_HASH`

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

## Process Detection

`detectAgentProcess()`: given pane PID, does BFS through process tree (`ps -axo pid=,ppid=,%cpu=,command=`), finds child process matching agent name. Returns best match by CPU usage. Excludes grep processes.

## Mocking

All key functions have `var fooFn = foo` pattern for test substitution:
`tmuxShowEnvironmentFn`, `tmuxHasSessionFn`, `tmuxKillSessionFn`, `tmuxListSessionsFn`, `tmuxCapturePaneFn`, `tmuxDisplayFn`, `tmuxPaneStatusFn`, `detectAgentProcessFn`

## Related Context

- @src/AGENTS.md

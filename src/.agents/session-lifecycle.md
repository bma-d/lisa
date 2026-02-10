# Session Lifecycle & File Management

Last Updated: 2026-02-10
Related Files: `src/session_files.go`, `src/commands_session.go`

## Overview

Session naming, artifact storage, and lifecycle management for Lisa sessions.

## Session Naming

Format: `lisa-{projectSlug}-{timestamp}-{agent}-{mode}[-{tag}]`
- `projectSlug`: sanitized, lowercased base dir name (max 10 chars, alphanumeric only)
- `timestamp`: `YYMMDD-HHMMSS-{nanoseconds}` for uniqueness
- `agent`: `claude` or `codex`
- `mode`: `interactive` or `exec`
- `tag`: optional, sanitized (max 16 chars)

## Project Isolation

`projectHash()`: MD5 (first 8 hex chars) of canonical project root (resolved symlinks, absolute path). All artifact paths include this hash.

## Artifact Files

All stored in `/tmp/`:

| Type | Path Pattern | Content |
|------|-------------|---------|
| Meta | `.lisa-{hash}-session-{id}-meta.json` | `sessionMeta`: agent, mode, projectRoot, startCmd, prompt, createdAt, runId |
| State | `.lisa-{hash}-session-{id}-state.json` | `sessionState`: pollCount, hasEverBeenActive, output freshness fields, last classification |
| Output | `lisa-{hash}-output-{id}.txt` | Captured pane output (up to 260 lines) |
| Heartbeat | `.lisa-{hash}-session-{id}-heartbeat.txt` | File mtime refreshed by wrapper heartbeat loop |
| Events | `.lisa-{hash}-session-{id}-events.jsonl` | Snapshot/transition events for observability (`status`, `reason`, `signals`) |
| Scripts | `lisa-cmd-{id}-{nano}.sh` | Temp scripts for long commands |

## Lifecycle Operations

1. **Spawn** (`cmdSessionSpawn`): reset stale artifacts -> validate heartbeat path -> create tmux session -> wrap startup command (`__LISA_SESSION_START__:{runId}:{ts}` / `__LISA_SESSION_DONE__:{runId}:{exit}` + heartbeat loop + signal traps) -> send command -> save meta -> clear state. Metadata persistence is now fail-fast: if meta write fails, Lisa kills the new tmux session and cleans artifacts before returning non-zero.
2. **Monitor** (`cmdSessionMonitor`): poll loop calling `computeSessionStatus()` at interval, stops on terminal state
3. **Kill** (`cmdSessionKill`): cleanup artifacts -> kill tmux session (order matters: artifacts first)
4. **Kill-all** (`cmdSessionKillAll`): list sessions -> kill each with cleanup

## Observability Retention

Event logs are bounded: `appendSessionEvent()` trims oversized `/tmp/.lisa-*-events.jsonl` files using `LISA_EVENTS_MAX_BYTES` and `LISA_EVENTS_MAX_LINES` before/after appends.

## Project Matching

`sessionMatchesProjectRoot()`: checks `LISA_PROJECT_HASH` env var in tmux session. Falls back to meta file existence for legacy sessions created before the env var was added.

## Related Context

- @src/AGENTS.md

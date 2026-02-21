# Session Lifecycle & File Management

Last Updated: 2026-02-21
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
| Meta | `.lisa-{hash}-session-{id}-meta.json` | `sessionMeta`: session, parentSession (when nested), agent, mode, projectRoot, startCmd, prompt, createdAt, runId |
| State | `.lisa-{hash}-session-{id}-state.json` | `sessionState`: pollCount, hasEverBeenActive, output freshness fields, last classification |
| Output | `lisa-{hash}-output-{id}.txt` | Captured pane output (up to 260 lines) |
| Heartbeat | `.lisa-{hash}-session-{id}-heartbeat.txt` | File mtime refreshed by wrapper heartbeat loop |
| Done | `.lisa-{hash}-session-{id}-done.txt` | Wrapper trap writes `{runId}:{exitCode}` completion sidecar |
| Events | `.lisa-{hash}-session-{id}-events.jsonl` | Snapshot/transition events for observability (`status`, `reason`, `signals`) |
| Scripts | `lisa-cmd-{hash}-{id}-{nano}.sh` | Temp scripts for long commands (project-hash scoped) |

## Lifecycle Operations

1. **Spawn** (`cmdSessionSpawn`): reset stale artifacts -> validate heartbeat path -> create tmux session -> wrap startup command (`__LISA_SESSION_START__:{runId}:{ts}` / `__LISA_SESSION_DONE__:{runId}:{exit}` + heartbeat loop + signal traps) -> send command -> save meta -> clear state. Metadata persistence is fail-fast: if meta write fails, Lisa kills the new tmux session and cleans artifacts before returning non-zero. If tmux session creation itself fails after heartbeat prep, Lisa now cleans artifacts before returning. Spawn failure paths also emit lifecycle failure reasons (`spawn_*_error`) for observability.
   - Wrapper exports `LISA_RUN_ID` to child processes so project-scoped hooks (for example Claude finish hooks) can emit run-id-matching done sidecars via `LISA_DONE_FILE`.
   - Spawn metadata now records `parentSession` when session is created from inside another Lisa session (`LISA_SESSION_NAME`).
   - `--dry-run` validates + resolves spawn command/socket/env and exits without tmux/artifact mutation.
2. **Monitor** (`cmdSessionMonitor`): poll loop calling `computeSessionStatus()` at interval, stops on terminal state
   - Optional `--until-marker` stops with `exitReason=marker_found` when pane output contains a deterministic marker token.
   - `--expect terminal|marker` adds fail-fast expectation gates for success-path ambiguity.
3. **Kill** (`cmdSessionKill`): resolve descendants from metadata parent links -> kill descendants first -> kill target session -> cleanup runtime artifacts (preserve event log) -> append lifecycle events
4. **Kill-all** (`cmdSessionKillAll`): list sessions -> kill each -> cleanup runtime artifacts (preserve event log) -> append lifecycle event. Non-`--project-only` kill-all now cleans artifacts across hashes by default for the listed session IDs.
5. **Tree** (`cmdSessionTree`): builds metadata graph (`parentSession` links) and returns nested hierarchy (text/JSON) or flat rows (`--flat`).
6. **Smoke** (`cmdSessionSmoke`): deterministic nested orchestration (`L1->...->LN`, max 4) with marker assertions and optional JSON summary.

Cleanup scope is hash-scoped by default (current project hash only). Cross-hash cleanup for spawn/kill paths requires explicit `--cleanup-all-hashes`, except non-`--project-only` kill-all which now enables cross-hash cleanup automatically.

## Observability Retention

Event logs are bounded: `appendSessionEvent()` trims `/tmp/.lisa-*-events.jsonl` files using both `LISA_EVENTS_MAX_BYTES` and `LISA_EVENTS_MAX_LINES` on every append. Appends + trims are serialized with an event-file lock (`.events.jsonl.lock`) and `LISA_EVENT_LOCK_TIMEOUT_MS`.
`readSessionEventTail()` now takes a shared lock on the same lock file to avoid reading partial lines during concurrent append/trim operations.
Trim now compacts from a bounded tail window (`~2x` max-bytes), preventing large historical logs from causing unbounded trim cost in the append path.
Stale event artifacts are pruned by age via `LISA_EVENT_RETENTION_DAYS` (default 14) during spawn/kill/kill-all maintenance paths.

## Project Matching

`sessionMatchesProjectRoot()`: checks `LISA_PROJECT_HASH` env var in tmux session. Falls back to meta file existence for legacy sessions created before the env var was added.

## Related Context

- @src/AGENTS.md

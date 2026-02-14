# Architecture

Last Updated: 2026-02-14
Related Files: `src/status.go`, `src/tmux.go`, `src/session_files.go`, `src/commands_session.go`

## Overview

Lisa is a standalone Go CLI (zero dependencies) that orchestrates Claude/Codex AI agent sessions inside tmux. It's infrastructure for LLM orchestrators that manage concurrent AI workers across projects.

## Session Lifecycle

```
spawn → in_progress → completed
                   ↘ crashed
                   ↘ stuck (no active process/heartbeat after grace period)
                   ↘ degraded (infra contention/read failures)
                   ↘ just_started (first 3 polls, idle)
```

`waiting_input` remains in the public enum for compatibility but is currently non-emitting in default classification.

## State Classification (Multi-Signal)

`computeSessionStatus()` in `status.go` is process-first and combines:
1. **Pane status**: alive / exited:N / crashed:N (via tmux `pane_dead` + `pane_dead_status`)
2. **Done sidecar**: `/tmp/.lisa-*-done.txt` (`{runID}:{exitCode}`) written by wrapper trap
3. **Agent process detection**: BFS walk of process tree from pane PID, matching "claude"/"codex"
4. **Heartbeat freshness**: wrapper-updated heartbeat file mtime
5. **Pane command class**: shell vs non-shell fallback activity signal

Tmux output capture is used only for terminal/full capture artifacts, not for state inference.

## Session Isolation

- Sessions named: `lisa-{projectSlug}-{timestamp}-{agent}-{mode}[-{tag}]`
- Artifacts in `/tmp/` keyed by `projectHash` (MD5 of canonical project root)
- Three artifact files per session: meta (.json), state (.json), output (.txt)
- Command scripts for long commands: `/tmp/lisa-cmd-{session}-{nano}.sh`

## Tmux Integration

- Sessions created with env vars: `LISA_SESSION`, `LISA_AGENT`, `LISA_MODE`, `LISA_PROJECT_HASH`
- Commands sent via `send-keys` (short) or temp script fallback (>500 chars)
- Text sent via `load-buffer` / `paste-buffer` for safe multi-line delivery
- Capture via `capture-pane -p -S -{lines}`

## Related Context

- @src/AGENTS.md

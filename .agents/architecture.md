# Architecture

Last Updated: 2026-02-09
Related Files: `src/status.go`, `src/tmux.go`, `src/session_files.go`, `src/commands_session.go`

## Overview

Lisa is a standalone Go CLI (zero dependencies) that orchestrates Claude/Codex AI agent sessions inside tmux. It's infrastructure for LLM orchestrators that manage concurrent AI workers across projects.

## Session Lifecycle

```
spawn → in_progress → waiting_input → completed
                   ↘ crashed
                   ↘ stuck (output stale, no agent process)
                   ↘ just_started (first 3 polls, idle)
```

## State Classification (Multi-Signal)

`computeSessionStatus()` in `status.go` combines:
1. **Pane status**: alive / exited:N / crashed:N (via tmux `pane_dead` + `pane_dead_status`)
2. **Agent process detection**: BFS walk of process tree from pane PID, matching "claude"/"codex"
3. **Output freshness**: MD5 hash of captured output, age vs `LISA_OUTPUT_STALE_SECONDS` (default 240s)
4. **Prompt detection**: agent-specific regex on last captured line (e.g. trailing `>` for claude, `❯` timestamp for codex)
5. **Exec completion marker**: `__LISA_EXEC_DONE__:N` printed after non-interactive commands
6. **Todo parsing**: `[x]`/`[ ]` checkbox counting from captured output

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

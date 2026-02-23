# Context
This documents the `tmux inactive-looking panes` behavior observed while `lisa-loop` drives `lisa` sessions.

## Incident Summary
When running feedback workflows, tmux panes sometimes show only `__LISA_SESSION_START__` (or very little output), and observers interpret the pane as inactive/stuck even while the run is still progressing.

## Scope
- Repo: `lisa`
- Affected path: session monitoring + status classification
- Not a `lisa-loop` orchestration correctness issue by itself.

# Reproduction
1. Start a session command that emits little/no stdout for long periods.
2. Observe pane via tmux attach or monitor tools.
3. Status may look idle/just-started/stuck despite the command still running.

# Evidence
## Session wrapper emits sparse markers by design
`src/session_wrapper.go:8` wraps command execution and emits:
- start marker (`sessionStartPrefix`) before command
- done marker (`sessionDonePrefix`) on exit

If command stdout is quiet, pane capture can look empty apart from markers.

## Status relies heavily on agent PID detection
`src/status.go:350` marks active via `agentPID > 0` (`classification_reason=agent_pid_alive`).

If no agent PID is detected, status can fall back to:
- `heartbeat_fresh` (best case), or
- `grace_period_just_started` / `stuck_no_signals` (`src/status.go:383-413`).

## Agent detection is narrowly keyed to claude/codex
`src/tmux.go:599` uses `agentPrimaryExecutable`:
- `codex` for codex agent
- `claude` otherwise

`src/tmux.go:405-408` detection needs strict/wrapper/needle matches. If real command chain does not expose expected tokens, `agentPID` remains `0`.

# Root Cause
`lisa` monitoring currently infers liveness from process-name heuristics (`claude`/`codex` + optional needles). For wrapper-heavy or nonstandard launch commands, PID matching misses, so status logic may classify as idle/stuck even while work is ongoing.

# Impact
- False operator perception: "pane inactive" while pipeline is still running.
- Confusing debugging in long-running quiet phases.
- Premature manual interruption risk.

# Recommended Fixes
## 1) Treat heartbeat as strong in-progress signal when pane command is live
In `src/status.go`, prefer `active/in_progress` if heartbeat is fresh and pane is still running, even when `agentPID == 0`.

## 2) Expand process matching strategy
In `src/tmux.go`:
- allow broader primary executable aliases
- strengthen wrapper command parsing for nested launchers
- support per-session/runtime match hints (not only global env needles)

## 3) Improve monitor UX
When classified via heartbeat fallback (without PID), display explicit reason: "active by heartbeat; agent pid unresolved" to avoid "inactive" interpretation.

## 4) Add regression tests
Add tests for:
- quiet command with fresh heartbeat + no agentPID -> active
- wrapper command where binary name is indirect but still running -> active fallback
- stale heartbeat + no PID -> stuck (expected)

# Triage Notes
- This is a `lisa` status/monitoring issue.
- `lisa-loop` can still have separate orchestration issues, but this specific inactive-pane signal mismatch belongs in `lisa`.

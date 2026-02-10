# SDK Usage Guide

Last Updated: 2026-02-10
Related Files: `agent.md`, `src/commands_session.go`, `src/commands_agent.go`

## Overview

How to use Lisa as infrastructure from an LLM orchestrator or script.

## Integration Pattern

1. **Spawn** one session per task (`session spawn --json`), store returned session name (custom `--session` values must start with `lisa-`)
2. **Poll** with `session monitor --json` (blocking loop) or `session status --json` (one-shot)
3. If state is `waiting_input` or `stuck`, send next instruction with `session send --text "..." --enter`; if state is `degraded`, retry polling and inspect `signals.*Error`
4. Fetch artifacts with `session capture --lines N`
5. Kill and clean up with `session kill --session NAME`

## Key Commands

```bash
# Spawn interactive Claude
lisa session spawn --agent claude --mode interactive --prompt "Review code" --json

# Spawn exec-mode Codex
lisa session spawn --agent codex --mode exec --prompt "Run tests" --json

# Monitor until terminal state
lisa session monitor --session NAME --json --poll-interval 20

# Send follow-up
lisa session send --session NAME --text "Continue" --enter

# Capture output
lisa session capture --session NAME --lines 300

# Build command string without spawning
lisa agent build-cmd --agent claude --mode exec --prompt "Fix lint"
```

## Exit Codes

- `session monitor`: 0 on completed/waiting_input, 2 on crashed/stuck/not_found/timeout
- `session status`: 0 always (unless arg parse error)
- `session exists`: 0 if exists, 1 if not

## Session States

| State | Meaning |
|-------|---------|
| `in_progress` | Agent appears active (process running or output fresh) |
| `waiting_input` | Session idle, agent waiting for user input |
| `completed` | Exec done or pane exited cleanly |
| `crashed` | Pane exited with non-zero or agent crashed |
| `stuck` | Output stale, no agent process, no prompt detected |
| `degraded` | Infrastructure contention/error state (e.g., lock timeout); retry polling |
| `just_started` | Idle but within first 3 polls (grace period) |

## Related Context

- @AGENTS.md
- @src/AGENTS.md

# State Machine & Status Classification

Last Updated: 2026-02-09
Related Files: `src/status.go`, `src/types.go`

## Overview

`computeSessionStatus()` is the core function. It combines multiple signals to classify a session into one of: `in_progress`, `waiting_input`, `completed`, `crashed`, `stuck`, `just_started`.

## Signal Sources

1. **Pane status** (`tmuxPaneStatus`): checks `pane_dead` + `pane_dead_status` -> alive / exited:N / crashed:N
2. **Agent process** (`detectAgentProcess`): BFS walk from pane PID through process tree, matches "claude"/"codex" in command string, returns PID + CPU%
3. **Output freshness**: MD5 hash of captured pane output compared to last known hash; stale after `LISA_OUTPUT_STALE_SECONDS` (default 240s)
4. **Prompt detection** (`looksLikePromptWaiting`): agent-specific regex patterns on last output line
   - Claude: trailing `>` or `›`, or "press enter to send"
   - Codex: `❯` with timestamp pattern, or "tokens used"
5. **Exec completion** (`parseExecCompletion`): searches for `__LISA_EXEC_DONE__:N` marker
6. **Todo progress** (`parseTodos`): counts `[x]`/`[ ]` checkboxes in output

## Classification Priority

```
pane crashed/exited → immediate terminal state
exec mode + done marker → completed/crashed based on exit code
interactive waiting (low CPU + stale output) OR prompt regex → waiting_input
agent PID alive OR output fresh OR non-shell pane command → in_progress
poll count ≤ 3 → just_started (grace period)
else → stuck
```

## Wait Estimation

`estimateWait()` uses keyword matching on active task text and todo progress percentage to return estimated seconds remaining. Range: 30-120s.

## State Persistence

`sessionState` struct saved to `/tmp/` between polls: tracks `PollCount`, `HasEverBeenActive`, `LastOutputHash`, `LastOutputAt`.

## Related Context

- @src/AGENTS.md

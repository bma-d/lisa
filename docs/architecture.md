# Lisa - Architecture Document

## Executive Summary

Lisa is a Go CLI that orchestrates AI agent sessions (Claude Code and Codex) inside tmux. It follows a straightforward command-routing architecture with domain-split implementation files. The system manages session lifecycle through tmux primitives, persists state via JSON files in `/tmp/`, and classifies session health through a multi-signal state machine.

## Architecture Pattern

**CLI with Subcommand Routing + State Machine**

```
User / Orchestrator
        │
        ▼
    main.go → Run()
        │
        ├── doctor          → Health check (tmux, claude, codex on PATH)
        ├── agent build-cmd  → Generate agent startup command string
        └── session
              ├── name      → Generate deterministic session name
              ├── spawn     → Create tmux session + send startup command
              ├── send      → Send text/keys to running session
              ├── status    → One-shot status classification
              ├── monitor   → Polling loop around status
              ├── capture   → Capture tmux pane output
              ├── list      → List Lisa sessions (optionally project-scoped)
              ├── exists    → Check if session exists
              ├── kill      → Kill session + cleanup artifacts
              └── kill-all  → Kill all Lisa sessions
```

## Core Concepts

### Session Lifecycle

```
spawn → in_progress → waiting_input → (send) → in_progress → completed
                   ↘ stuck                                  ↗
                    ↘ crashed ──────────────────────────────╯
```

1. **Spawn**: Creates a tmux session with environment variables (`LISA_SESSION`, `LISA_AGENT`, `LISA_MODE`, `LISA_PROJECT_HASH`), sends the agent startup command
2. **Monitor**: Polls `computeSessionStatus()` at configurable intervals, exits on terminal states
3. **Status Classification**: Multi-signal state machine combining pane status, process tree, output freshness, and prompt-pattern heuristics
4. **Cleanup**: Kills tmux session, removes meta/state/output files and command scripts from `/tmp/`

### Session State Machine

`computeSessionStatus()` in `status.go` classifies sessions using these signals:

| Signal | Source | Purpose |
|---|---|---|
| Pane status | `tmux display #{pane_dead}` | Detect crashed/exited processes |
| Agent process | Process tree walk from pane PID | Confirm agent is running |
| Agent CPU | `ps -axo %cpu` | Distinguish active vs idle agent |
| Output hash | MD5 of captured output | Detect output changes |
| Output age | Time since last output change | Stale detection (default 240s) |
| Prompt patterns | Regex on last line of output | Detect Claude/Codex input prompts |
| Exec marker | `__LISA_EXEC_DONE__:N` | Detect non-interactive completion |

Classification priority:
1. Pane dead with exit code → `completed` or `crashed`
2. Exec done marker found → `completed` or `crashed` (by exit code)
3. Interactive waiting (low CPU + stale output) or prompt pattern → `waiting_input`
4. Agent process running or output fresh → `in_progress`
5. Early polls (1-3) with no agent → `just_started`
6. Otherwise → `stuck`

### File System Layout

All session artifacts live in `/tmp/` with project-hash scoping:

```
/tmp/
├── .lisa-{projectHash}-session-{artifactID}-meta.json   # Session metadata (agent, mode, command, etc.)
├── .lisa-{projectHash}-session-{artifactID}-state.json  # Poll state (output hash, timestamps)
├── lisa-{projectHash}-output-{artifactID}.txt           # Captured output on terminal states
└── lisa-cmd-{artifactID}-{nanos}.sh                     # Long command scripts (>500 chars)
```

- **Project hash**: First 8 chars of MD5 of canonical project root path
- **Artifact ID**: Sanitized session name (alphanumeric + `-_.` only, max 64 chars)
- **File permissions**: 0600 (owner-only read/write)
- **Atomic writes**: Write to `.tmp` file then `os.Rename()`

### Session Naming Convention

```
lisa-{projectSlug}-{YYMMDD}-{HHMMSS}-{nanoseconds}-{agent}-{mode}[-{tag}]
```

- `projectSlug`: Lowercase alphanumeric of directory basename (max 10 chars)
- Timestamp ensures uniqueness across concurrent spawns
- Optional tag for user-provided labels (max 16 chars)

### Agent Command Generation

`buildAgentCommand()` produces the startup command:

| Agent | Mode | Command |
|---|---|---|
| claude | interactive | `claude [agentArgs] [prompt]` |
| claude | exec | `claude -p 'prompt' [agentArgs]` |
| codex | interactive | `codex [agentArgs] [prompt]` |
| codex | exec | `codex exec 'prompt' --full-auto [agentArgs]` |

Exec commands are wrapped with `wrapExecCommand()` to append `__LISA_EXEC_DONE__:$?` for completion detection.

Long commands (>500 chars) are written to temp scripts and executed via `bash '/tmp/lisa-cmd-...'` to avoid tmux `send-keys` length limits.

### Testability Architecture

tmux functions are assigned to package-level variables for test mocking:

```go
var tmuxHasSessionFn = tmuxHasSession
var tmuxKillSessionFn = tmuxKillSession
var tmuxCapturePaneFn = tmuxCapturePane
// etc.
```

Tests override these in `t.Cleanup()` blocks to simulate tmux behavior without a running tmux server.

## Testing Strategy

| Layer | File | Trigger | Coverage |
|---|---|---|---|
| Unit/Regression | `regressions_test.go` | `go test ./...` | Core logic, edge cases, error paths |
| E2E Integration | `e2e_claude_test.go` | `LISA_E2E_CLAUDE=1 go test ./...` | Full spawn→monitor→capture cycle |

Key test areas:
- Fallback script generation preserves exec markers
- Session artifact cleanup doesn't expand wildcards
- File permissions are owner-only (0600)
- Project hash is stable across equivalent paths (`.` vs absolute)
- Kill/kill-all propagate tmux errors
- Status computation handles all tmux read failure modes
- Doctor requires tmux + at least one agent

## Security Considerations

- Session artifacts stored with `0600` permissions
- Atomic file writes prevent corruption
- Session names are sanitized to prevent path traversal
- Shell quoting via single-quote wrapping with proper escaping
- Wildcard session names don't expand during cleanup
- No secrets stored in session metadata (prompts may contain sensitive content in `/tmp/`)

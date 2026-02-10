# src/ Context

All application source code. Single `package app` with thin `main.go` entrypoint delegating to `Run()`.

## Context Files

| File | Description | Read When |
|------|-------------|-----------|
| `.agents/state-machine.md` | Status classification, multi-signal detection, prompt patterns | Debugging or modifying status logic |
| `.agents/tmux-layer.md` | tmux command wrappers, send strategies, process detection | Modifying tmux interactions |
| `.agents/session-lifecycle.md` | Session naming, file paths, artifacts, cleanup | Working with session persistence |
| `.agents/testing.md` | Test patterns, mocking strategy, E2E tests | Writing or running tests |

## Key Decisions

- `status.go` is the core: multi-signal state machine combining pane status, process tree, output freshness, prompt regex
- `tmux.go` wraps all tmux calls; uses `send-keys` for short commands, temp script fallback for long ones
- `session_files.go` manages `/tmp/` artifacts with atomic writes and MD5-based project isolation
- `agent_command.go` builds agent CLI invocations; `wrapExecCommand()` injects completion markers

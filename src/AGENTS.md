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

- Status classification is centralized in `status.go`.
- All tmux IO stays in `tmux.go`.
- Session artifacts are hash-scoped under `/tmp/`.
- Agent command assembly is centralized in `agent_command.go`.

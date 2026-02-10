# Testing

Last Updated: 2026-02-09
Related Files: `src/regressions_test.go`, `src/e2e_claude_test.go`

## Overview

Two test files covering unit/regression tests and E2E integration tests.

## Unit/Regression Tests

File: `regressions_test.go`

Run with: `go test ./...`

Uses function variable mocking pattern — tests replace `tmuxFooFn` variables with test doubles to avoid real tmux calls. Example:
```go
tmuxHasSessionFn = func(session string) bool { return true }
tmuxCapturePaneFn = func(session string, lines int) (string, error) { return mockOutput, nil }
```

Covers: state machine classification, artifact paths, sanitization, edge cases.

## E2E Integration Tests

File: `e2e_claude_test.go`

Run with: `LISA_E2E_CLAUDE=1 go test ./...`

Requires real tmux server + Claude/Codex on PATH. Spawns actual sessions, monitors, captures, and cleans up. Gated behind env var to avoid CI failures.

## Writing New Tests

1. For status/state logic: mock tmux function variables in `regressions_test.go`
2. For new commands: add flag parsing tests, mock underlying functions
3. For integration: add to `e2e_claude_test.go` with `LISA_E2E_CLAUDE` guard

## Related Context

- @src/AGENTS.md

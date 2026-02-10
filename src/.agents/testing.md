# Testing

Last Updated: 2026-02-10
Related Files: `src/regressions_test.go`, `src/session_wrapper_test.go`, `src/e2e_claude_test.go`, `src/e2e_codex_test.go`

## Overview

Four test files covering regression/unit, wrapper/observability tests, and Claude/Codex E2E integration tests.

## Unit/Regression Tests

File: `regressions_test.go`

Run with: `go test ./...`

Uses function variable mocking pattern — tests replace `tmuxFooFn` variables with test doubles to avoid real tmux calls. Example:
```go
tmuxHasSessionFn = func(session string) bool { return true }
tmuxCapturePaneFn = func(session string, lines int) (string, error) { return mockOutput, nil }
```

Covers: state machine classification, artifact paths, sanitization, edge cases.

## Wrapper/Observability Tests

File: `session_wrapper_test.go`

Covers run-id marker matching, heartbeat mtime freshness boundaries, transition/snapshot event logging, session explain payloads, malformed event line tolerance, event log trimming, process-scan caching, signal trap behavior, and concurrent status polling lock safety.

## E2E Integration Tests

Files: `e2e_claude_test.go`, `e2e_codex_test.go`

Run with:
- `LISA_E2E_CLAUDE=1 go test ./...`
- `LISA_E2E_CODEX=1 go test ./...`

Requires real tmux server + matching agent on PATH. Spawns actual sessions, monitors, captures, and cleans up. Gated behind env vars to avoid CI failures.

## Writing New Tests

1. For status/state logic: mock tmux function variables in `regressions_test.go`
2. For new commands: add flag parsing tests, mock underlying functions
3. For integration: add to `e2e_claude_test.go` or `e2e_codex_test.go` with env guard

## Related Context

- @src/AGENTS.md

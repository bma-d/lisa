# Coding Conventions

Last Updated: 2026-02-09
Related Files: `src/utils.go`, `src/types.go`, `src/tmux.go`

## Overview

Project conventions and patterns used across the Lisa codebase.

## Language & Dependencies

- Go 1.21, stdlib only (zero external dependencies)
- Single `src/` package (`package app`), thin `main.go` entrypoint

## Patterns

- **Function variable mocking**: tmux operations stored as `var tmuxFooFn = tmuxFoo` for test substitution
- **Atomic file writes**: temp file + `os.Rename` via `writeFileAtomic()`
- **Manual flag parsing**: hand-rolled `for i := 0; i < len(args)` loops (no flag library)
- **JSON output**: all commands support `--json` for machine consumption
- **CSV-style text output**: comma-separated fields for human/script consumption
- **Shell quoting**: single-quote wrapping with `'"'"'` escapes

## Build & Test

```bash
go build -o lisa .           # build binary
go test ./...                # unit + regression tests
LISA_E2E_CLAUDE=1 go test ./...  # include E2E integration tests
```

## Agent Guidelines

- Use the repo's package manager/runtime; no swaps without approval
- Fix root causes, not band-aids
- If unsure, read more code first; if still blocked, ask with short options
- If instructions conflict, call it out and pick the safer path

## Related Context

- @AGENTS.md

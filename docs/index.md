# Lisa - Documentation Index

## Project Overview

- **Type:** Monolith (single CLI binary)
- **Language:** Go 1.21
- **Dependencies:** stdlib only (zero external)
- **Architecture:** CLI with subcommand routing + session state machine
- **Purpose:** Orchestrate Claude/Codex AI agent sessions in tmux

## Quick Reference

- **Entry Point:** `main.go` → `src/run.go:Run()`
- **Build:** `go build -o lisa .`
- **Test:** `go test ./...`
- **Health Check:** `./lisa doctor`
- **Source Lines:** ~2,700 (11 source files + 2 test files in `src/`)

## Generated Documentation

- [Project Overview](./project-overview.md) — Purpose, tech stack, design decisions
- [Architecture](./architecture.md) — State machine, session lifecycle, file layout, security
- [Source Tree Analysis](./source-tree-analysis.md) — Annotated directory tree, file dependencies, code metrics
- [Development Guide](./development-guide.md) — Build, test, run, conventions

## Existing Documentation

- [AGENTS.md](../AGENTS.md) — Contributor and agent guidelines
- [agent.md](../agent.md) — Lisa SDK usage guide (command reference, integration patterns)
- [Changelog (latest)](./changelog/260210.md) — Recent changes

## Getting Started

1. Build: `go build -o lisa .`
2. Verify: `./lisa doctor`
3. Spawn a session: `./lisa session spawn --agent claude --mode interactive --prompt "Hello" --json`
4. Monitor: `./lisa session monitor --session <NAME> --json`
5. Capture output: `./lisa session capture --session <NAME> --json`
6. Cleanup: `./lisa session kill --session <NAME>`

## AI-Assisted Development

When working on this codebase with AI agents:

- **Architecture reference:** Start with [architecture.md](./architecture.md) for the state machine and session lifecycle
- **Adding commands:** Follow the pattern in `src/commands_session.go` — flag parsing loop, validation, tmux interaction, JSON output
- **Testing:** Mock tmux via function variables (see `src/tmux.go` var declarations, `src/regressions_test.go` for examples)
- **File paths:** All session artifacts go to `/tmp/` with project-hash scoping (see `src/session_files.go`)

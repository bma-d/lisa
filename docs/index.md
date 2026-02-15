# Lisa - Documentation Index

## Project Snapshot (2026-02-15)

- **Type:** Monolith (single CLI binary)
- **Language:** Go 1.21
- **Dependencies:** stdlib only (zero external)
- **Runtime:** tmux + Claude and/or Codex on PATH
- **Architecture:** CLI subcommand router + process-first session state machine
- **Source footprint (`src/`):** 33 Go files (19 non-test + 14 test), 12,450 LOC
- **Coverage:** 79.9% statements (`go test -cover ./src`)

## Quick Reference

- **Entrypoint:** `main.go` -> `src/run.go:Run()`
- **Build:** `go build -o lisa .`
- **Test:** `go test ./...`
- **Race test:** `go test -race ./...`
- **Health check:** `./lisa doctor`

## Documentation Set

- [Usage Guide](../USAGE.md) - canonical CLI usage and command reference
- [Project Overview](./project-overview.md) - purpose, components, decisions
- [Architecture](./architecture.md) - runtime flow, state classification, artifacts
- [Source Tree Analysis](./source-tree-analysis.md) - annotated tree and code metrics
- [Development Guide](./development-guide.md) - build/test/run workflow
- [Project Scan Report](./project-scan-report.json) - machine-readable repo snapshot metadata

## Canonical Runtime Docs

- [README](../README.md) - install and quick start
- [Usage Guide](../USAGE.md) - single source of truth for CLI usage
- [Agent SDK Guide](../agent.md) - orchestrator integration contract
- [Root Context Index](../AGENTS.md) - JIT context index and project conventions

## Changelog

- [260215](./changelog/260215.md)
- [260214](./changelog/260214.md)
- [260213](./changelog/260213.md)
- [260212](./changelog/260212.md)
- [260210](./changelog/260210.md)
- [260209](./changelog/260209.md)

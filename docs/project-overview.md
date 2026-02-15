# Lisa - Project Overview

## Purpose

Lisa is a standalone Go CLI that orchestrates Claude/Codex agent sessions in tmux. It is infrastructure for higher-level orchestrators that need to spawn, monitor, steer, and collect output from concurrent AI worker sessions.

## What Lisa Does

- Creates isolated tmux sessions per task.
- Builds agent startup commands for Claude and Codex (interactive or exec mode).
- Classifies session runtime state from process/pane/heartbeat/done-file signals.
- Sends follow-up input (`text` or raw tmux keys) to running sessions.
- Captures transcript or raw pane output.
- Emits lifecycle/status observability events for diagnostics.
- Cleans up session artifacts safely (hash-scoped by default).

## Technology Stack

| Category | Value |
|---|---|
| Language | Go 1.21 |
| Dependencies | stdlib only |
| Runtime dependency | tmux |
| Agent targets | Claude Code, OpenAI Codex |
| Packaging | GoReleaser (tar.gz + Homebrew + deb/rpm/apk) |

## Repository Layout (Current)

```text
lisa/
├── main.go
├── go.mod
├── README.md
├── agent.md
├── AGENTS.md
├── .goreleaser.yaml
├── .github/workflows/{ci.yml,release.yml}
├── docs/
│   ├── architecture.md
│   ├── development-guide.md
│   ├── index.md
│   ├── project-overview.md
│   ├── project-scan-report.json
│   └── source-tree-analysis.md
├── src/
│   ├── run.go
│   ├── help.go
│   ├── commands_*.go
│   ├── status*.go
│   ├── tmux.go
│   ├── session_*.go
│   ├── claude_session.go
│   ├── codex_session.go
│   ├── utils.go
│   ├── types.go
│   └── *_test.go
└── _bmad/
```

## Current Build/Test Health

- `go test ./...` passes.
- `go test -race ./...` passes.
- `go test -cover ./src` reports 79.9% statement coverage.

## Key Runtime Decisions

- Process-first status classification (not output-text-first).
- Session completion signal uses wrapper done-file sidecar and pane exit state.
- Session artifacts are project-hash-scoped under `/tmp`.
- Event logs are bounded, lock-protected JSONL with retention cleanup.
- Default transcript capture path is Claude transcript when metadata indicates Claude; raw pane remains available via `--raw`.

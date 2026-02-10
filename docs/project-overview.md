# Lisa - Project Overview

## Purpose

Lisa is a standalone Go CLI for orchestrating Claude and Codex AI agent sessions inside tmux. It enables spawning parallel AI workers, tracking their progress, sending follow-up input, and managing session lifecycles — all without attaching to tmux directly.

## Executive Summary

Lisa provides a unified interface to launch, monitor, and interact with AI coding agents (Claude Code and OpenAI Codex) running in tmux sessions. It is designed as infrastructure for LLM orchestrators that need to manage multiple concurrent agent workers across projects.

Key capabilities:
- Spawn interactive or execution-mode agent sessions in tmux
- Poll session status with intelligent state classification (in_progress, waiting_input, completed, crashed, stuck)
- Send text or key sequences to running sessions
- Capture session output for downstream consumption
- Project-scoped session management with hash-based isolation
- Health checks via `doctor` command

## Technology Stack

| Category | Technology | Version | Notes |
|---|---|---|---|
| Language | Go | 1.21 | Minimum version |
| Dependencies | stdlib only | — | Zero external dependencies |
| Runtime | tmux | — | Required for session management |
| Agents | Claude Code, Codex | — | At least one must be on PATH |
| Build | `go build` | — | Single binary output |

## Architecture Classification

- **Repository type:** Monolith
- **Architecture pattern:** CLI with domain-split subcommand routing
- **Project type:** CLI tool
- **Parts:** 1 (single)

## Key Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| No external deps | stdlib only | Minimal binary, fast builds, no supply chain risk |
| tmux as runtime | Process isolation | Persistent sessions, detached execution, cross-platform terminal multiplexing |
| JSON output mode | `--json` flag | Machine-readable output for orchestrator integration |
| Exec completion marker | `__LISA_EXEC_DONE__:` | Reliable detection of non-interactive command completion |
| Project hash scoping | MD5-based | Isolate sessions per project without path collisions |
| Atomic file writes | tmp+rename | Prevent corrupted state/meta files on crash |

## Repository Structure

```
lisa/
├── main.go              # Process entrypoint
├── go.mod               # Go module definition (Go 1.21, zero deps)
├── AGENTS.md            # Repo contributor guidelines
├── agent.md             # Lisa SDK/usage guide
├── src/                 # All application source code
│   ├── run.go           # Command routing and usage text
│   ├── types.go         # Shared types and constants
│   ├── agent_command.go # Agent startup command generation
│   ├── commands_agent.go    # doctor + agent subcommands
│   ├── commands_session.go  # session subcommands (spawn/send/status/monitor/...)
│   ├── status.go        # Session state inference and classification
│   ├── tmux.go          # tmux interaction layer
│   ├── session_files.go # Session naming, metadata, state persistence
│   ├── utils.go         # Shared helpers (shell, json, env, file)
│   ├── e2e_claude_test.go   # E2E test with real Claude
│   └── regressions_test.go  # Unit and regression tests
└── docs/                # Generated documentation
    └── changelog/       # Changelog entries
```

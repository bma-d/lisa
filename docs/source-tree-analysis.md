# Lisa - Source Tree Analysis

## Current Tree (Condensed)

```text
lisa/
├── main.go
├── go.mod
├── README.md
├── agent.md
├── AGENTS.md
├── .goreleaser.yaml
├── .github/workflows/
│   ├── ci.yml
│   └── release.yml
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
│   ├── commands_agent.go
│   ├── commands_session.go
│   ├── commands_session_state.go
│   ├── commands_session_explain.go
│   ├── commands_session_manage.go
│   ├── agent_command.go
│   ├── tmux.go
│   ├── status.go
│   ├── status_helpers.go
│   ├── session_files.go
│   ├── session_observability.go
│   ├── session_wrapper.go
│   ├── claude_session.go
│   ├── codex_session.go
│   ├── utils.go
│   ├── types.go
│   ├── build_info.go
│   └── *_test.go
└── _bmad/
```

## `src/` Metrics (Live)

- Go files: 33
- Non-test files: 19
- Test files: 14
- Total LOC in `src/*.go`: 12,450
- Non-test function count: 369
- Test count (`func Test...`): 184

## Largest Code Files

- `src/regressions_test.go` - 2,646 lines
- `src/session_wrapper_test.go` - 1,217 lines
- `src/claude_session_test.go` - 579 lines
- `src/hardening_test.go` - 567 lines
- `src/tmux.go` - 534 lines
- `src/session_observability.go` - 474 lines
- `src/commands_session_state.go` - 449 lines
- `src/commands_session.go` - 433 lines
- `src/claude_session.go` - 430 lines
- `src/status.go` - 421 lines

## Functional Dependency Map

```text
main.go
  -> src.Run
     -> cmdDoctor / cmdAgent
        -> buildAgentCommand
     -> cmdSession (router)
        -> spawn/send/name handlers
           -> tmux layer + wrapper + session file persistence
        -> status/monitor/explain handlers
           -> computeSessionStatus
              -> tmux snapshot + process scan + done file + heartbeat + state locks
              -> optional terminal capture artifact write
           -> event tail read/write
        -> capture handler
           -> Claude transcript path (metadata-driven) or raw tmux capture
        -> list/exists/kill/kill-all
           -> tmux listing/kill + scoped artifact cleanup + lifecycle events
```

## Testing Topology

- Core regression: `src/regressions_test.go`
- Wrapper/lock/event hardening: `src/session_wrapper_test.go`, `src/stability_findings_test.go`, `src/hardening_test.go`
- Command branch coverage: `src/command_coverage_test.go`, `src/coverage_cli_additional_test.go`, `src/coverage_status_observability_test.go`
- Transcript readers: `src/claude_session_test.go`, `src/codex_session_test.go`
- Help routing: `src/help_test.go`
- Hermetic lifecycle e2e: `src/e2e_interactive_fake_test.go`, `src/e2e_exec_fake_test.go`
- Real-agent e2e (env-gated): `src/e2e_claude_test.go`, `src/e2e_codex_test.go`

## Operational Notes

- Runtime source of truth is code + `.agents` context files.
- `docs/` is manually maintained; refresh this file when `src/` file set or runtime classifier behavior changes.

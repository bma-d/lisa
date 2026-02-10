# Lisa - Source Tree Analysis

## Directory Structure

```
lisa/                           # Project root
├── main.go                     # ENTRY POINT — delegates to src.Run()
├── go.mod                      # Go 1.21, zero external dependencies
├── AGENTS.md                   # Contributor/agent guidelines
├── agent.md                    # Lisa SDK usage guide for AI agents
├── src/                        # All application logic (package: app)
│   ├── run.go                  # Top-level command router (doctor|session|agent|help)
│   ├── types.go                # Constants + shared structs (sessionMeta, sessionState, sessionStatus, monitorResult, processInfo)
│   ├── agent_command.go        # buildAgentCommand() — generates claude/codex startup commands
│   ├── commands_agent.go       # cmdDoctor() + cmdAgent() + cmdAgentBuildCmd()
│   ├── commands_session.go     # All session subcommands: name, spawn, send, status, monitor, capture, list, exists, kill, kill-all
│   ├── status.go               # computeSessionStatus() — state machine for session classification
│   ├── tmux.go                 # tmux wrapper layer (new-session, send-keys, capture-pane, etc.)
│   ├── session_files.go        # Session naming, file path generation, metadata/state persistence
│   ├── utils.go                # Shared utilities (runCmd, shellQuote, writeJSON, filterInputBox, etc.)
│   ├── e2e_claude_test.go      # E2E integration test (requires LISA_E2E_CLAUDE=1)
│   └── regressions_test.go     # Unit tests for safety, regression, and edge cases
└── docs/                       # Documentation output
    └── changelog/
        └── 260209.md
```

## Critical Files

### Entry Point

- **`main.go`** — Minimal; calls `app.Run(os.Args[1:])` and exits with the returned code.

### Command Routing

- **`src/run.go`** — Top-level switch: `doctor`, `session`, `agent`, `help`. Prints usage on unknown commands.

### Core Domain Logic

- **`src/commands_session.go`** (801 lines) — The largest file. Implements all 10 session subcommands with flag parsing. Central to Lisa's functionality.
- **`src/status.go`** (333 lines) — Session state machine. Classifies sessions as `in_progress`, `waiting_input`, `completed`, `crashed`, `stuck`, or `just_started`. Includes prompt-waiting heuristics for both Claude and Codex.
- **`src/tmux.go`** (284 lines) — All tmux interactions. Includes testability hooks via function variables (`tmuxHasSessionFn`, etc.). Handles session creation, key sending, pane capture, process tree walking.

### Supporting Infrastructure

- **`src/session_files.go`** (225 lines) — Session naming convention (`lisa-{slug}-{timestamp}-{agent}-{mode}`), file path generation for meta/state/output files in `/tmp/`, atomic writes, cleanup.
- **`src/agent_command.go`** (100 lines) — Generates Claude (`claude -p` / `claude`) or Codex (`codex exec --full-auto` / `codex`) commands. Wraps exec commands with completion markers.
- **`src/types.go`** (66 lines) — All shared structs and constants. Defaults: poll interval 30s, max polls 120, stale threshold 240s, tmux 220x60.
- **`src/utils.go`** (159 lines) — Shell execution, JSON output, MD5 hashing, file operations, input box filtering (strips Claude's TUI input boxes from captured output).

### Testing

- **`src/e2e_claude_test.go`** — Full integration: builds Lisa, spawns a Claude exec session, monitors to completion, validates output markers. Gated behind `LISA_E2E_CLAUDE=1`.
- **`src/regressions_test.go`** — 10 unit tests covering: fallback script generation, tail truncation, project hash canonicalization, session artifact safety (no wildcard expansion, file permissions), kill error propagation, status computation error paths.

## File Dependency Graph

```
main.go
  └── src/run.go (Run)
        ├── src/commands_agent.go (cmdDoctor, cmdAgent)
        │     └── src/agent_command.go (buildAgentCommand)
        └── src/commands_session.go (cmdSession)
              ├── src/agent_command.go (buildAgentCommand, wrapExecCommand)
              ├── src/session_files.go (generateSessionName, saveSessionMeta, loadSessionMeta, cleanupSessionArtifacts, ...)
              ├── src/status.go (computeSessionStatus)
              │     ├── src/tmux.go (capture, display, paneStatus, detectAgentProcess)
              │     └── src/session_files.go (loadSessionState, saveSessionState)
              ├── src/tmux.go (tmuxNewSession, tmuxSendCommandWithFallback, tmuxSendText, ...)
              └── src/utils.go (shellQuote, writeJSON, trimLines, ...)
```

## Code Metrics

| File | Lines | Functions | Test Coverage |
|---|---|---|---|
| commands_session.go | 801 | 13 | Via regressions_test.go |
| status.go | 333 | 10 | Via regressions_test.go |
| tmux.go | 284 | 16 | Via regressions_test.go (mocked) |
| session_files.go | 225 | 16 | Via regressions_test.go |
| utils.go | 159 | 14 | Indirect |
| e2e_claude_test.go | 156 | 4 | E2E test file |
| regressions_test.go | 384 | 10 | Unit test file |
| commands_agent.go | 151 | 4 | Via regressions_test.go |
| agent_command.go | 100 | 5 | Via regressions_test.go |
| types.go | 66 | 0 | Type definitions |
| run.go | 53 | 2 | — |
| main.go | 12 | 1 | — |
| **Total** | **~2,724** | **~95** | — |

# Lisa - Development Guide

## Prerequisites

- **Go** 1.21 or later
- **tmux** installed and available on PATH
- **Claude Code** (`claude`) and/or **Codex** (`codex`) on PATH (at least one required)

## Getting Started

### Build

```bash
cd /Users/joon/projects/tools/lisa
go build -o lisa .
```

### Health Check

```bash
./lisa doctor
```

Verifies that `tmux`, `claude`, and `codex` are available. Requires tmux + at least one agent to report "ready".

### Run

```bash
# Show usage
./lisa help

# Spawn an interactive Claude session
./lisa session spawn --agent claude --mode interactive --prompt "Review this codebase"

# Spawn a non-interactive Codex session
./lisa session spawn --agent codex --mode exec --prompt "Run tests and fix lint errors" --json
```

## Project Structure

```
lisa/
├── main.go           # Entry point (delegates to src.Run)
├── go.mod            # Go 1.21, zero dependencies
└── src/              # Package "app" — all implementation
    ├── run.go                  # Command router
    ├── types.go                # Shared types/constants
    ├── agent_command.go        # Agent command builder
    ├── commands_agent.go       # doctor, agent subcommands
    ├── commands_session.go     # session subcommands
    ├── status.go               # Status state machine
    ├── tmux.go                 # tmux wrapper layer
    ├── session_files.go        # File paths, metadata, state
    ├── utils.go                # Shared utilities
    ├── e2e_claude_test.go      # E2E test
    └── regressions_test.go     # Unit tests
```

## Testing

### Unit Tests

```bash
go test ./...
```

Runs all unit and regression tests. These mock tmux interactions via function variables and don't require a running tmux server.

### E2E Tests

```bash
LISA_E2E_CLAUDE=1 go test ./... -timeout 600s
```

Requires:
- tmux server running
- `claude` on PATH with valid authentication
- Network access for Claude API calls

The E2E test builds Lisa, spawns a Claude exec session, monitors to completion, and validates output markers.

### Test Conventions

- Function variable pattern for tmux mocking: `tmuxHasSessionFn = func(...) { ... }`
- Tests restore originals via `t.Cleanup()`
- No external test dependencies

## Key Commands Reference

| Command | Description |
|---|---|
| `lisa doctor` | Check prerequisites |
| `lisa session spawn` | Create a new agent session |
| `lisa session status` | Get one-shot session status |
| `lisa session monitor` | Poll until terminal state |
| `lisa session send` | Send text or keys to session |
| `lisa session capture` | Capture pane output |
| `lisa session list` | List Lisa sessions |
| `lisa session exists` | Check if session exists |
| `lisa session kill` | Kill and cleanup a session |
| `lisa session kill-all` | Kill all Lisa sessions |
| `lisa agent build-cmd` | Generate agent command string |
| `lisa session name` | Generate session name |

## Configuration

Lisa uses no config files. Behavior is controlled via:

- **CLI flags**: `--agent`, `--mode`, `--prompt`, `--json`, etc.
- **Environment variables**:
  - `LISA_OUTPUT_STALE_SECONDS`: Override stale output threshold (default: 240s)
  - `LISA_E2E_CLAUDE`: Set to `1` to enable E2E tests

## Code Conventions

- **Package name**: `app` (in `src/` directory)
- **Error handling**: Return exit codes (0=success, 1=error, 2=terminal failure in monitor)
- **Output**: Plain text by default, JSON with `--json` flag
- **Flag parsing**: Manual `for`-loop with `switch` (no external flag library)
- **Testability**: Package-level function variables for external dependency mocking
- **File writes**: Atomic (write to tmp, then rename) with 0600 permissions

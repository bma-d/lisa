# Lisa Agent SDK Guide

`Lisa` is a standalone Go CLI for orchestrating Claude or Codex sessions in tmux.

Use this when an agent needs to:
- Spawn parallel Claude/Codex workers.
- Track each worker's progress without attaching to tmux.
- Send follow-up input when a worker pauses or waits.
- Run non-interactive execution mode (`claude -p` / `codex exec`) from tmux.

## Quick start

Build once:

```bash
cd /Users/joon/projects/tools/lisa
go build -o lisa .
```

Optional health check:

```bash
./lisa doctor
```

## Code layout

The CLI now uses a thin root entrypoint and domain-split implementation under `src/`:

- `main.go`: minimal process entrypoint; delegates to `src.Run(...)`.
- `src/run.go`: top-level command routing and usage text.
- `src/commands_agent.go`: `doctor` and `agent build-cmd` command handling.
- `src/commands_session.go`: `session` subcommands (`spawn/send/status/monitor/...`).
- `src/status.go`: session state inference, active/waiting/completed/crashed classification.
- `src/agent_command.go`: agent startup command generation and exec wrapping.
- `src/tmux.go`: tmux interactions and process discovery.
- `src/session_files.go`: session naming, metadata/state/output file paths and persistence.
- `src/utils.go`: shared shell/json/env/string helpers.
- `src/types.go`: shared structs and constants.

## Core contract

### 1) Spawn a worker session

Interactive Claude:

```bash
./lisa session spawn --agent claude --mode interactive --prompt "Review current repo status" --json
```

Interactive Codex:

```bash
./lisa session spawn --agent codex --mode interactive --prompt "Inspect failing tests and propose fixes" --json
```

Execution mode Claude (non-interactive):

```bash
./lisa session spawn --agent claude --mode exec --prompt "Summarize uncommitted changes as bullet points" --json
```

Execution mode Codex (non-interactive):

```bash
./lisa session spawn --agent codex --mode exec --prompt "Run tests and fix lint errors" --json
```

Notes:
- `--mode exec` maps to `claude -p ...` or `codex exec ... --full-auto`.
- Provide `--command` to fully override generated startup command.
- Long commands are auto-written to `/tmp/lisa-cmd-<session>-*.sh` to avoid tmux send-keys length issues.

### 2) Track progress

One-shot status:

```bash
./lisa session status --session <SESSION_NAME> --json
```

Continuous monitor loop:

```bash
./lisa session monitor --session <SESSION_NAME> --json --poll-interval 20 --max-polls 120
```

Important states:
- `in_progress`: agent appears active.
- `waiting_input`: session is waiting for extra input.
- `completed`: run is complete.
- `stuck`: output stale and no active work detected.
- `crashed`: pane/process exited with failure-like state.

### 3) Send follow-up input to a running session

Send text only:

```bash
./lisa session send --session <SESSION_NAME> --text "Continue and apply all safe fixes" --enter
```

Send raw tmux keys:

```bash
./lisa session send --session <SESSION_NAME> --keys "C-c" --enter
```

### 4) Capture output

```bash
./lisa session capture --session <SESSION_NAME> --lines 300
```

When status/monitor is run with `--full` (monitor uses full internally), Lisa can persist captured output to:
- `/tmp/lisa-<projectHash>-output-<session>.txt`

## Session management

List sessions:

```bash
./lisa session list
```

Project-only list:

```bash
./lisa session list --project-only
```

Check if exists:

```bash
./lisa session exists --session <SESSION_NAME>
```

Kill one:

```bash
./lisa session kill --session <SESSION_NAME>
```

Kill all Lisa sessions:

```bash
./lisa session kill-all
```

## Command builder helper

If your orchestrator wants to build command strings first:

```bash
./lisa agent build-cmd --agent codex --mode exec --prompt "Run unit tests and report failures"
```

## Integration pattern for any LLM orchestrator

1. Spawn one session per task (`session spawn --json`), store returned session ID.
2. Poll with `session monitor` or `session status`.
3. If state is `waiting_input` or `stuck`, send next instruction with `session send --enter`.
4. Fetch artifacts with `session capture`.
5. Kill and clean up sessions when done.

## Exit behavior summary

- `session monitor` exit code:
  - `0` on `completed` or `waiting_input`.
  - `2` on `crashed`, `stuck`, `not_found`, or timeout.
- `session status` always returns current status payload unless argument parsing fails.

## Assumptions

- `tmux`, `claude`, and/or `codex` are installed and on `PATH`.
- tmux server is available for the current user.
- Workspace path is readable/writable for the launched agent process.

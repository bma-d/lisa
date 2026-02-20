# Lisa Usage Guide

Standalone CLI for orchestrating Claude/Codex sessions in tmux.

## Prerequisites

- macOS or Linux
- `tmux` on `PATH`
- at least one agent CLI on `PATH`: `claude` or `codex`

Quick check:

```bash
lisa doctor
```

## Build / Install

Build from source:

```bash
go build -o lisa .
./lisa version
```

Install options are also listed in `README.md` (Homebrew, `go install`, release archives/packages).

## Command Map

```text
lisa doctor
lisa version
lisa session name
lisa session spawn
lisa session send
lisa session status
lisa session explain
lisa session monitor
lisa session capture
lisa session list
lisa session exists
lisa session kill
lisa session kill-all
lisa agent build-cmd
```

Per-command help:

```bash
lisa <command> --help
lisa session <subcommand> --help
```

## Quick Start

```bash
# 1) Verify runtime
lisa doctor

# 2) Spawn
lisa session spawn --agent claude --mode interactive --prompt "Review this repo" --json

# 3) Poll
lisa session monitor --session <SESSION_NAME> --json --poll-interval 20

# 4) Send follow-up
lisa session send --session <SESSION_NAME> --text "Continue" --enter

# 5) Capture output
lisa session capture --session <SESSION_NAME> --lines 300

# 6) Cleanup
lisa session kill --session <SESSION_NAME>
```

## Session States

Lisa classifies sessions process-first (pane/process/heartbeat/done-file signals).

- `just_started`: initial grace window
- `in_progress`: agent appears active
- `completed`: clean completion
- `crashed`: non-zero exit / crashed pane
- `stuck`: inactive beyond grace rules
- `degraded`: infra contention/read-error path
- `not_found`: tmux session missing
- `waiting_input`: interactive session appears idle and waiting for user input

## Command Reference

### `doctor`

Check prerequisites (`tmux`, `claude`, `codex`).

```bash
lisa doctor
lisa doctor --json
```

Exit code: `0` when `tmux` exists and at least one of `claude|codex` exists, else `1`.

### `version`

Print build metadata:

```bash
lisa version
```

### `session name`

Generate a unique session name for project+agent+mode (timestamp-based).

```bash
lisa session name \
  --agent claude \
  --mode interactive \
  --project-root /abs/path \
  --tag audit
```

Flags:

- `--agent`: `claude|codex` (default `claude`)
- `--mode`: `interactive|exec` (default `interactive`)
- `--project-root`: defaults to current directory
- `--tag`: optional suffix (sanitized)

### `session spawn`

Create tmux session + start agent command.

```bash
lisa session spawn \
  --agent claude \
  --mode exec \
  --prompt "Summarize uncommitted changes" \
  --json
```

Flags:

- `--agent`: `claude|codex`
- `--mode`: `interactive|exec`
- `--session`: explicit name (must start with `lisa-`)
- `--prompt`: startup prompt
- `--command`: full command override (skips agent command builder)
- `--agent-args`: extra args appended to agent CLI
- `--project-root`: project isolation root (default cwd)
- `--width`: tmux width (default `220`)
- `--height`: tmux height (default `60`)
- `--cleanup-all-hashes`: clean artifacts across all project hashes
- `--no-dangerously-skip-permissions`: disable default Claude permission-skip flag injection
- `--json`: machine-readable output

Notes:

- For Claude, Lisa injects `--dangerously-skip-permissions` by default unless disabled.
- `exec` mode requires a prompt unless `--command` is provided.
- If no `--session`, Lisa auto-generates one.
- Codex `exec` defaults include `--full-auto` and `--skip-git-repo-check`.
- Nested Codex runs: `codex exec --full-auto` uses a sandbox that can block tmux socket creation (`Operation not permitted`). For 2-3 level nested Lisa flows, prefer `--mode interactive` plus `session send`.
- If you pass `--agent-args '--dangerously-bypass-approvals-and-sandbox'`, Lisa omits `--full-auto` automatically (Codex rejects combining both flags).

### `session send`

Send input to running session.

```bash
lisa session send --session <NAME> --text "Continue with safe fixes" --enter
lisa session send --session <NAME> --keys "C-c" --enter
```

Flags:

- `--session` (required)
- `--project-root` (default cwd)
- `--text` (mutually exclusive with `--keys`)
- `--keys` (mutually exclusive with `--text`; whitespace-split into tmux key tokens)
- `--enter`
- `--json`

### `session status`

One-shot session status snapshot.

```bash
lisa session status --session <NAME>
lisa session status --session <NAME> --full
lisa session status --session <NAME> --json
```

Flags:

- `--session` (required)
- `--agent`: `auto|claude|codex` (default `auto`)
- `--mode`: `auto|interactive|exec` (default `auto`)
- `--project-root` (default cwd)
- `--full`: include classification/signal columns in CSV mode
- `--json`

### `session explain`

Diagnostics: status + recent lifecycle events.

```bash
lisa session explain --session <NAME>
lisa session explain --session <NAME> --events 30 --json
```

Flags:

- `--session` (required)
- `--agent`: `auto|claude|codex`
- `--mode`: `auto|interactive|exec`
- `--project-root`
- `--events N` (default `10`)
- `--json`

### `session monitor`

Poll status until terminal/stop condition.

```bash
lisa session monitor --session <NAME> --json --poll-interval 20 --max-polls 120
```

Flags:

- `--session` (required)
- `--agent`: `auto|claude|codex`
- `--mode`: `auto|interactive|exec`
- `--project-root`
- `--poll-interval N` seconds (default `30`)
- `--max-polls N` (default `120`)
- `--stop-on-waiting true|false` (default `true`)
- `--waiting-requires-turn-complete true|false` (default `false`)
- `--json`
- `--verbose`

When `--waiting-requires-turn-complete true` is set, `monitor` only stops on
`waiting_input` after transcript tail inspection confirms an assistant turn is
complete (Claude/Codex interactive sessions with prompt metadata).

Exit code behavior:

- `0`: final `completed` (or `waiting_input` / `waiting_input_turn_complete` when emitted and stop enabled)
- `2`: `crashed`, `stuck`, `not_found`, timeout, degraded timeout path
- `1`: argument/infra errors

### `session capture`

Capture output from session.

```bash
lisa session capture --session <NAME>
lisa session capture --session <NAME> --raw --lines 500
lisa session capture --session <NAME> --json
```

Flags:

- `--session` (required)
- `--raw`: force tmux pane capture
- `--keep-noise`: keep Codex/MCP startup noise in pane capture
- `--lines N`: pane lines for raw capture (default `200`)
- `--project-root`
- `--json`

Behavior:

- default: for Claude sessions, tries transcript capture first
- fallback: raw tmux pane capture if transcript path fails/unavailable
- raw capture path filters known Codex/MCP startup noise by default
- `--keep-noise`: disables that filtering

### `session list`

List tmux sessions with `lisa-` prefix.

```bash
lisa session list
lisa session list --project-only
```

Flags:

- `--project-only`
- `--project-root`

### `session exists`

Check existence of one session.

```bash
lisa session exists --session <NAME>
```

Flags:

- `--session` (required)
- `--project-root` (default cwd)

Output: `true` or `false`.

Exit codes:

- `0`: exists
- `1`: missing (or argument errors)

### `session kill`

Kill one session + cleanup artifacts.

```bash
lisa session kill --session <NAME>
```

Flags:

- `--session` (required)
- `--project-root`
- `--cleanup-all-hashes`

### `session kill-all`

Kill multiple sessions + cleanup artifacts.

```bash
lisa session kill-all
lisa session kill-all --project-only
```

Flags:

- `--project-only`
- `--project-root`
- `--cleanup-all-hashes`

### `agent build-cmd`

Build agent startup command without spawning tmux session.

```bash
lisa agent build-cmd --agent codex --mode exec --prompt "Run tests"
lisa agent build-cmd --agent claude --mode interactive --prompt "Review diff" --json
```

Flags:

- `--agent`: `claude|codex`
- `--mode`: `interactive|exec`
- `--prompt`
- `--agent-args`
- `--no-dangerously-skip-permissions`
- `--json`

## Output Modes

JSON support:

- `doctor`
- `agent build-cmd`
- `session spawn`
- `session send`
- `session status`
- `session explain`
- `session monitor`
- `session capture`

Text/CSV-only commands:

- `session name`
- `session list`
- `session exists`
- `session kill`
- `session kill-all`
- `version`

## Runtime Environment Variables

All optional; defaults shown from source.

```text
LISA_CMD_TIMEOUT_SECONDS=20
LISA_OUTPUT_STALE_SECONDS=240
LISA_HEARTBEAT_STALE_SECONDS=8
LISA_PROCESS_SCAN_INTERVAL_SECONDS=8
LISA_PROCESS_LIST_CACHE_MS=500
LISA_STATE_LOCK_TIMEOUT_MS=2500
LISA_EVENT_LOCK_TIMEOUT_MS=2500
LISA_EVENTS_MAX_BYTES=1000000
LISA_EVENTS_MAX_LINES=2000
LISA_EVENT_RETENTION_DAYS=14
LISA_CLEANUP_ALL_HASHES=false
LISA_AGENT_PROCESS_MATCH=...
LISA_AGENT_PROCESS_MATCH_CLAUDE=...
LISA_AGENT_PROCESS_MATCH_CODEX=...
LISA_PROJECT_ROOT=(set internally per command)
LISA_TMUX_SOCKET=(set internally; defaults to /tmp/lisa-tmux-<slug>-<hash>.sock)
```

tmux env keys propagated into spawned panes:

```text
LISA_SESSION
LISA_SESSION_NAME
LISA_AGENT
LISA_MODE
LISA_PROJECT_HASH
LISA_HEARTBEAT_FILE
LISA_DONE_FILE
```

Lisa clears `TMUX` when executing tmux commands, and routes tmux through a
project-derived socket path in `/tmp`, so nested Lisa calls are detached from
the current tmux client context.

## Orchestrator Pattern

Recommended automation loop:

1. `session spawn --json` per task
2. poll with `session monitor --json` (or `status --json`)
3. on `stuck`: `session send --text ... --enter`
4. on `degraded`: keep polling; inspect `signals.*Error`
5. collect results with `session capture`
6. cleanup with `session kill`

## Nested Smoke Script

Repo-local command for deterministic 3-level nested interactive tmux validation:

```bash
./smoke-nested
```

Optional flags:

- `--project-root PATH`
- `--max-polls N` (default `180`)
- `--keep-sessions` (skip auto-kill for debugging)

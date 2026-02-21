---
name: lisa
description: |
  Use the `lisa` CLI to orchestrate concurrent Claude/Codex AI agent sessions
  inside tmux. Use when: (1) spawning parallel AI workers on tasks, (2) monitoring
  agent session completion, (3) sending follow-up prompts to running agents,
  (4) capturing agent output, (5) managing session lifecycle (list/kill/cleanup),
  (6) nested orchestration (Lisa sessions launching Lisa-managed workers).
  Requires: tmux, claude or codex on PATH. Install: brew install bma-d/tap/lisa.
  Examples assume repo-local `./lisa`; only switch to global `lisa` intentionally.
author: Claude Code
version: 2.6.1
date: 2026-02-20
tags: [lisa, tmux, orchestration, claude, codex, agents]
---

# Lisa — tmux AI Agent Orchestrator

## What It Does

Lisa spawns Claude/Codex agents in tmux sessions, monitors their progress via a process-first multi-signal state machine, and provides structured output. Zero external Go dependencies.

## LLM Invocation Rules (Critical)

1. In this repo, run `./lisa` (not `lisa`) to avoid PATH/version drift.
2. Use real subcommands: `./lisa session spawn ...` (do not pass `"session spawn"` as one quoted token).
3. Always pass `--project-root` in multi-step or nested flows so all commands hit the same tmux socket/hash.
4. Use `./lisa cleanup --include-tmux-default` only when explicitly requested (can target default tmux sockets).

## Prerequisites

```bash
brew install bma-d/tap/lisa   # or: go install github.com/bma-d/lisa@latest
LISA_BIN=./lisa                # keep repo-local binary pinned for deterministic behavior
$LISA_BIN doctor               # verify: tmux + at least one of claude/codex on PATH
```

## LLM Fast Path (Use First)

```bash
ROOT=/path/to/project
LISA_BIN=./lisa
# 1) Spawn with explicit routing
SESSION=$($LISA_BIN session spawn \
  --agent codex --mode interactive \
  --project-root "$ROOT" \
  --prompt "Do X, then wait." \
  --json | jq -r .session)

# 2) Pick one stop strategy
$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --json
$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --stop-on-waiting true --json
$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --until-marker "TASK_DONE" --json

# 3) Capture, then cleanup
$LISA_BIN session capture --session "$SESSION" --project-root "$ROOT" --raw --lines 300
$LISA_BIN session kill --session "$SESSION" --project-root "$ROOT"
```
Observed behavior (validated 2026-02-20):
- `--until-marker` returns `exitReason:"marker_found"` with `finalState:"in_progress"` / `finalStatus:"active"` (success, not terminal completion).
- `--waiting-requires-turn-complete true` can timeout (`max_polls_exceeded`) for custom-command sessions without transcript turn boundaries.
- `session exists` prints `false` and exits `1` when missing.

## Core Pattern: Spawn -> Monitor -> Capture -> Cleanup

```bash
# 1. Spawn an agent session
$LISA_BIN session spawn \
  --agent claude --mode interactive \
  --prompt "Refactor auth module" \
  --project-root /path/to/project \
  --json

# 2. Monitor until terminal state (blocking)
$LISA_BIN session monitor \
  --session "$SESSION_NAME" \
  --project-root /path/to/project \
  --poll-interval 30 --max-polls 120 \
  --json

# 3. Capture transcript (default: clean user/assistant messages from Claude JSONL)
$LISA_BIN session capture --session "$SESSION_NAME" --json

# 3b. Capture raw tmux pane output (noise stripped by default)
$LISA_BIN session capture --session "$SESSION_NAME" --raw --lines 200
# Keep startup noise/chrome when needed
$LISA_BIN session capture --session "$SESSION_NAME" --raw --keep-noise --lines 200

# 4. Cleanup detached/stale tmux sockets after finishing Lisa work
$LISA_BIN cleanup --dry-run
$LISA_BIN cleanup
```

## Commands Reference

### session spawn

Create and start an agent session.

| Flag | Default | Description |
|------|---------|-------------|
| `--agent` | `claude` | Agent: `claude` or `codex` |
| `--mode` | `interactive` | Mode: `interactive` or `exec` (aliases: `execution`, `non-interactive`) |
| `--prompt` | `""` | Initial prompt |
| `--project-root` | cwd | Project directory |
| `--session` | auto | Override session name (must start with `lisa-`) |
| `--command` | `""` | Custom command (overrides agent CLI) |
| `--agent-args` | `""` | Extra args passed to agent CLI |
| `--width` | `220` | Tmux pane width |
| `--height` | `60` | Tmux pane height |
| `--no-dangerously-skip-permissions` | false | Don't add `--dangerously-skip-permissions` to claude (skip is on by default) |
| `--cleanup-all-hashes` | false | Clean artifacts across all project hash variants |
| `--dry-run` | false | Print resolved spawn plan without creating tmux session/artifacts |
| `--json` | false | JSON output |

JSON output: `{"session","agent","mode","runId","projectRoot","command"}`

Notes:
- `exec` mode requires `--prompt` unless `--command` is provided.
- If no `--session`, Lisa auto-generates one.
- For Codex `exec`, Lisa defaults include `--full-auto` and `--skip-git-repo-check`.
- Nested Codex prompts (`./lisa`, `lisa session spawn`, `nested lisa`) auto-enable `--dangerously-bypass-approvals-and-sandbox` and omit `--full-auto`.
- If you pass `--agent-args '--dangerously-bypass-approvals-and-sandbox'`, Lisa omits `--full-auto` automatically (Codex rejects combining both flags).
- For non-nested Codex `exec`, `--full-auto` sandbox can block child Lisa tmux socket creation (`Operation not permitted`); use interactive mode + `session send` or explicit bypass args.
- For deeply nested prompt chains, prefer heredoc prompt injection (`PROMPT=$(cat <<'EOF' ... EOF)` then `--prompt "$PROMPT"`) to avoid shell-quote collisions.
- `--dry-run` returns planned `command`, wrapped `startupCommand`, `socketPath`, and injected `env` keys for deterministic orchestration planning.

### session send

Send text or keys to a running session.

| Flag | Default | Description |
|------|---------|-------------|
| `--session` | (required) | Session name |
| `--project-root` | cwd | Project directory |
| `--text` | `""` | Text to send (mutually exclusive with `--keys`) |
| `--keys` | `""` | Tmux keys to send (mutually exclusive with `--text`; whitespace-split into tmux key tokens) |
| `--enter` | false | Press Enter after sending |
| `--json` | false | JSON output |

JSON output: `{"session","ok","enter"}`

### session status

One-shot status poll.

| Flag | Default | Description |
|------|---------|-------------|
| `--session` | (required) | Session name |
| `--project-root` | cwd | Project directory |
| `--agent` | `auto` | Agent hint: `auto`, `claude`, `codex` |
| `--mode` | `auto` | Mode hint: `auto`, `interactive`, `exec` |
| `--full` | false | Include classification/signal columns in CSV |
| `--fail-not-found` | false | Exit 1 when resolved status is `not_found` |
| `--json` | false | JSON output |

CSV output: `status,todosDone,todosTotal,activeTask,waitEstimate,sessionState`
With `--full`: `status_full_v1,status,todosDone,todosTotal,activeTask,waitEstimate,sessionState,classificationReason,paneStatus,agentPid,agentCpu,outputAgeSeconds,heartbeatAge,promptWaiting,heartbeatFresh,stateLockTimedOut,stateLockWaitMs,agentScanError,tmuxReadError,stateReadError,metaReadError,doneFileReadError`

Exit: 0 by default for all resolved statuses (including `not_found`); with `--fail-not-found`, returns 1 on `not_found`.

### session monitor

Block until terminal state.

| Flag | Default | Description |
|------|---------|-------------|
| `--session` | (required) | Session name |
| `--project-root` | cwd | Project directory |
| `--agent` | `auto` | Agent hint |
| `--mode` | `auto` | Mode hint |
| `--poll-interval` | `30` | Seconds between polls |
| `--max-polls` | `120` | Maximum polls (default = 1 hour) |
| `--stop-on-waiting` | `true` | Stop on `waiting_input` (takes bool value: `true`/`false`) |
| `--waiting-requires-turn-complete` | `false` | With `--stop-on-waiting true`, stop only after transcript turn-complete |
| `--until-marker` | `""` | Stop successfully when raw pane output contains marker text |
| `--expect` | `any` | Success expectation: `any`, `terminal`, `marker` |
| `--verbose` | false | Print poll details to stderr |
| `--json` | false | JSON output |

JSON output: `{"finalState","session","todosDone","todosTotal","outputFile","exitReason","polls","finalStatus"}`

Exit reasons: `completed`, `crashed`, `not_found`, `stuck`, `waiting_input`, `waiting_input_turn_complete`, `marker_found`, `max_polls_exceeded`, `degraded_max_polls_exceeded`

Timeout nuance: JSON commonly returns `finalState:"timeout"` with `exitReason:"max_polls_exceeded"` and `finalStatus:"active"`.

Marker nuance: when `exitReason:"marker_found"`, monitor succeeds but state commonly remains `finalState:"in_progress"`/`finalStatus:"active"` (marker reached before terminal completion).

Turn-complete nuance: `--waiting-requires-turn-complete true` is strict; with non-transcript/custom-command flows it may never emit `waiting_input_turn_complete` and can timeout.

Expectation nuance: `--expect terminal` fails fast on marker/waiting success paths (`expected_terminal_got_*`, exit `2`). `--expect marker` fails fast when marker is not first success condition (`expected_marker_got_*`, exit `2`).

Exit codes: 0 = completed/waiting_input/waiting_input_turn_complete/marker_found, 2 = crashed/stuck/not_found/timeout/expectation-mismatch.

### session capture

Capture transcript (default for Claude) or raw pane output.

| Flag | Default | Description |
|------|---------|-------------|
| `--session` | (required) | Session name |
| `--lines` | `200` | Pane lines for raw capture |
| `--raw` | false | Force raw tmux capture (skip transcript) |
| `--keep-noise` | false | Keep Codex/MCP startup noise in raw pane capture |
| `--strip-noise` | n/a | Compatibility alias to force default startup noise filtering |
| `--project-root` | cwd | Project directory |
| `--json` | false | JSON output |

Default behavior for Claude sessions: reads `~/.claude/projects/{encoded-path}/{sessionId}.jsonl` to return structured messages. Falls back to raw if transcript unavailable.
Promptless/custom-command Claude sessions without prompt+createdAt metadata intentionally fall back to raw capture (prevents stale transcript mismatches).
Raw capture strips startup noise by default. Use `--keep-noise` to preserve full pane text (`--strip-noise` kept as compatibility alias).

JSON (transcript): `{"session","claudeSession","messages":[{"role","text","timestamp"}]}`
JSON (raw): `{"session","capture"}`

### session explain

Detailed diagnostics with event timeline.

| Flag | Default | Description |
|------|---------|-------------|
| `--session` | (required) | Session name |
| `--project-root` | cwd | Project directory |
| `--agent` | `auto` | Agent hint |
| `--mode` | `auto` | Mode hint |
| `--events` | `10` | Number of recent events to show |
| `--json` | false | JSON output |

JSON output: `{"status":{...},"eventFile","events":[...],"droppedEventLines"}`.
`status` uses the same terminal normalization as `session status` (`completed`, `crashed`, `stuck`, `not_found`).

### session list / exists / kill / kill-all / name

| Command | Key Flags | Output |
|---------|-----------|--------|
| `session list` | `--all-sockets`, `--project-only`, `--project-root`, `--json` | names (text) or JSON payload |
| `session exists` | `--session`, `--project-root`, `--json` | `true`/`false` or JSON payload (exit 0=yes, 1=no) |
| `session kill` | `--session`, `--project-root`, `--cleanup-all-hashes`, `--json` | `ok` or JSON payload |
| `session kill-all` | `--project-only`, `--project-root`, `--cleanup-all-hashes`, `--json` | `killed N sessions` or JSON payload |
| `session name` | `--agent`, `--mode`, `--project-root`, `--tag`, `--json` | name string or JSON payload |

Kill preserves event files for post-mortem.
`session list` scope is socket-bound: each project root uses its own Lisa tmux socket; pass the intended `--project-root` explicitly.
`session list --all-sockets` expands discovery across metadata-known project roots and only returns sessions still active on those roots.

### session tree

Inspect metadata parent/child relationships for nested orchestration.

| Flag | Default | Description |
|------|---------|-------------|
| `--session` | `""` | Optional root session filter |
| `--project-root` | cwd | Project directory |
| `--all-hashes` | false | Include metadata from all project hashes |
| `--flat` | false | Emit machine-friendly parent/child rows |
| `--json` | false | JSON output |

JSON output: `{"session","projectRoot","allHashes","nodeCount","roots":[{"session","parentSession","agent","mode","projectRoot","createdAt","children":[...]}]}`

### session smoke

Deterministic nested smoke (L1->...->LN) with marker assertions.

Flags: `--project-root`, `--levels` (1-4, default `3`), `--poll-interval`, `--max-polls`, `--keep-sessions`, `--json`.
Uses nested `session spawn/monitor/capture`, asserts all level markers, and returns non-zero on spawn/monitor/marker failure.

### cleanup

Clean detached tmux servers and stale socket files left behind by Lisa runs.

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | false | Show what would be removed/killed without mutating |
| `--include-tmux-default` | false | Also sweep tmux default sockets (`/tmp/tmux-*`) |
| `--json` | false | JSON output |

JSON output: `{"dryRun","scanned","removed","wouldRemove","killedServers","wouldKillServers","keptActive"}` (+ optional `errors` array when failures occur).
When not using `--json`, Lisa prints a one-line summary. Exit 1 if any socket probe/kill/remove errors occurred.

### Other Commands

| Command | Purpose |
|---------|---------|
| `doctor [--json]` | Check prerequisites (tmux + at least one of claude/codex). Exit 0 = ok, 1 = missing |
| `cleanup [--dry-run] [--include-tmux-default] [--json]` | Remove stale sockets and kill detached no-client tmux servers |
| `agent build-cmd` | Preview agent CLI command (`--agent`, `--mode`, `--prompt`, `--agent-args`, `--no-dangerously-skip-permissions`, `--json`) |
| `session smoke` | Deterministic nested smoke test (`--levels`, `--max-polls`, `--poll-interval`, `--keep-sessions`, `--json`) |
| `skills sync` | Sync external Lisa skill into repo `skills/lisa` (`--from codex|claude|path`, `--repo-root`, `--json`) |
| `skills install` | Install repo `skills/lisa` to `codex|claude|project` (`--to`, `--project-path`, `--repo-root`, `--json`) |
| `version` | Print version (also `--version`, `-v`) |

## Modes

| Mode | Agent runs as | Use for |
|------|--------------|---------|
| `interactive` | REPL (default) | Multi-turn tasks, follow-up prompts |
| `exec` | One-shot (`claude -p` / `codex exec --full-auto`) | Single prompt, auto-exits on completion |

Aliases `execution` and `non-interactive` are accepted for `exec`.

## Session States

Classification is process-first: pane status -> done file -> agent PID -> interactive idle checks -> heartbeat -> pane command -> degraded/grace fallback.

| State | Meaning | Terminal? |
|-------|---------|-----------|
| `just_started` | Grace period (polls 1-3, idle) | No |
| `in_progress` | Agent PID alive, heartbeat fresh, or non-shell command active | No |
| `waiting_input` | Interactive session idle after grace period (agent low CPU or known interactive shell idle) | No* |
| `completed` | Pane exited 0 or done file with exit 0 | Yes |
| `crashed` | Pane exited non-zero or done file with non-zero exit | Yes |
| `stuck` | No active process/heartbeat after grace period | Yes |
| `degraded` | Infrastructure error (lock timeout, read failure, scan error) | No** |
| `not_found` | Session doesn't exist in tmux | Yes |

*`waiting_input` is actively emitted for idle interactive sessions. Monitor stops on it when `--stop-on-waiting true`.
**`degraded` is retried by monitor; becomes `degraded_max_polls_exceeded` if all polls are degraded.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success / completed / waiting_input / waiting_input_turn_complete (with --stop-on-waiting) |
| 1 | Error (bad flags, missing session, tmux failure) |
| 2 | Monitor: crashed, stuck, not_found, timeout (`max_polls_exceeded` / `degraded_max_polls_exceeded`) |

## Recipes

### Parallel workers

```bash
S1=$($LISA_BIN session spawn --agent claude --mode exec \
  --prompt "Write unit tests for auth.go" \
  --project-root . --json | jq -r .session)

S2=$($LISA_BIN session spawn --agent claude --mode exec \
  --prompt "Add input validation to handlers.go" \
  --project-root . --json | jq -r .session)

# Monitor both (in background)
$LISA_BIN session monitor --session "$S1" --project-root . --json &
$LISA_BIN session monitor --session "$S2" --project-root . --json &
wait
```

### Capture clean transcript (default for Claude)

```bash
# Default: reads Claude's JSONL conversation file, returns clean messages
$LISA_BIN session capture --session "$S" --json
# {"session":"...","claudeSession":"uuid","messages":[{"role":"user","text":"...","timestamp":"..."},...]}

# Plain text output (> prefixed user messages, bare assistant messages)
$LISA_BIN session capture --session "$S"

# Raw tmux pane output (startup noise stripped by default)
$LISA_BIN session capture --session "$S" --raw --lines 200 --json
# Keep startup noise/chrome
$LISA_BIN session capture --session "$S" --raw --keep-noise --lines 200 --json
```

Transcript discovery: matches session prompt + timestamp against `~/.claude/history.jsonl` to find the Claude session ID, then reads `~/.claude/projects/{encoded-path}/{sessionId}.jsonl`.
Transcript capture requires prompt+createdAt metadata; if missing, default capture falls back to raw pane capture.

### Send follow-up to interactive session

```bash
$LISA_BIN session send --session "$S" --text "Now add error handling" --enter
# Send special keys
$LISA_BIN session send --session "$S" --keys "Escape"
```

### Poll status

```bash
# One-shot CSV
$LISA_BIN session status --session "$S" --project-root .

# Full diagnostics
$LISA_BIN session explain --session "$S" --project-root . --events 20

# Verbose monitoring (progress to stderr)
$LISA_BIN session monitor --session "$S" --project-root . --verbose --json
```

### Cleanup

```bash
$LISA_BIN session kill --session "$S" --project-root .
$LISA_BIN session kill-all --project-only --project-root .
# Clean across all project hash variants
$LISA_BIN session kill-all --cleanup-all-hashes

# Recommended after finishing Lisa usage: sweep detached/stale sockets
$LISA_BIN cleanup --dry-run
$LISA_BIN cleanup
# Optional broader sweep (includes /tmp/tmux-* default sockets)
$LISA_BIN cleanup --include-tmux-default
```

Safety: prefer `cleanup --dry-run` first, especially in shared tmux environments.

### Nested orchestration (Lisa inside Lisa/Codex agents)

```bash
# Parent session prompt tells agent to use ./lisa internally.
PARENT=$($LISA_BIN session spawn --agent codex --mode interactive \
  --project-root . \
  --prompt "Use ./lisa only. Spawn 2 exec workers, monitor both, then summarize findings." \
  --json | jq -r .session)

# Stop when agent reaches waiting_input (idle, ready for follow-up).
$LISA_BIN session monitor --session "$PARENT" --project-root . --stop-on-waiting true --json

# Stricter stop condition: wait for transcript-confirmed assistant completion.
$LISA_BIN session monitor --session "$PARENT" --project-root . \
  --stop-on-waiting true --waiting-requires-turn-complete true --json

# Follow-up nudge that preserves nested project routing.
$LISA_BIN session send --session "$PARENT" \
  --text "If incomplete, run ./lisa session list --project-root . and continue." \
  --enter
```

Nested trigger wording (Codex exec auto-bypass): prompts containing `./lisa`, `lisa session spawn`, or `nested lisa` automatically add `--dangerously-bypass-approvals-and-sandbox` and omit `--full-auto`.

Wording that reliably triggers nested bypass:
- `Use ./lisa for all child orchestration.`
- `Run lisa session spawn inside the spawned agent.`
- `Build a nested lisa chain and report markers.`

Deterministic nested validation:

```bash
# Built-in 3-level smoke
./smoke-nested --project-root "$(pwd)" --max-polls 120

# For 4-level verification, chain L1->L2->L3->L4 with command wrappers:
# each level runs:
#   ./lisa session spawn --command "/bin/bash <next-level-script>"
#   ./lisa session monitor ...
#   ./lisa session capture ...
# and assert markers from all levels in L1 capture.
```

## State Detection (Process-First)

Classification priority chain in `computeSessionStatus`:

1. **Pane status** — tmux `pane_dead` + `pane_dead_status` (alive/exited:N/crashed:N)
2. **Done file** — `{runId}:{exitCode}` written by wrapper EXIT trap
3. **Agent process** — BFS walk of process tree from pane PID, matching claude/codex
4. **Interactive idle checks** — (a) known interactive + agent PID alive + low CPU => `waiting_input`; (b) known interactive shell with fresh heartbeat + no active non-shell descendants (passive `sleep` ignored) => `waiting_input`
5. **Heartbeat** — wrapper touches file every 2s; fresh = within threshold (default 8s)
6. **Pane command** — shell vs non-shell as fallback activity signal
7. **Grace period** — `just_started` for polls 1-3
8. **Fallbacks** — done-file read error / agent scan error -> `degraded`; otherwise `stuck`

Output capture is used only for terminal/artifact capture, not state inference.

## Session Artifacts

Stored in `/tmp/` keyed by project hash (first 8 hex chars of MD5 of canonical project root):

```
/tmp/.lisa-{hash}-session-{id}-meta.json        # metadata (agent, mode, runId, prompt, createdAt)
/tmp/.lisa-{hash}-session-{id}-state.json       # poll state (cached agent/mode, scan results)
/tmp/.lisa-{hash}-session-{id}-done.txt         # completion marker ({runId}:{exitCode})
/tmp/.lisa-{hash}-session-{id}-heartbeat.txt    # liveness signal (mtime-based)
/tmp/.lisa-{hash}-session-{id}-events.jsonl     # event log (auto-trimmed at 1MB / 2000 lines)
/tmp/lisa-{hash}-output-{id}.txt                # captured pane output (terminal states)
/tmp/lisa-cmd-{hash}-{id}-{nanos}.sh            # temp script for long commands (>500 chars)
```

Artifacts cleaned on `kill`/`kill-all` (events preserved). Stale event files pruned after 14 days.

## Environment Overrides

| Variable | Default | Purpose |
|----------|---------|---------|
| `LISA_CMD_TIMEOUT_SECONDS` | `20` | Subprocess (tmux, ps) command timeout |
| `LISA_STATE_LOCK_TIMEOUT_MS` | `2500` | State file lock timeout |
| `LISA_EVENT_LOCK_TIMEOUT_MS` | `2500` | Event file lock timeout |
| `LISA_PROCESS_LIST_CACHE_MS` | `500` | Raw `ps` output cache TTL |
| `LISA_PROCESS_SCAN_INTERVAL_SECONDS` | `8` | Min interval between agent process scans |
| `LISA_HEARTBEAT_STALE_SECONDS` | `8` | Heartbeat freshness threshold |
| `LISA_OUTPUT_STALE_SECONDS` | `240` | Output freshness threshold |
| `LISA_EVENTS_MAX_BYTES` | `1000000` | Max event file size before trim |
| `LISA_EVENTS_MAX_LINES` | `2000` | Max event lines before trim |
| `LISA_EVENT_RETENTION_DAYS` | `14` | Stale event file pruning threshold |
| `LISA_AGENT_PROCESS_MATCH` | - | Custom process match pattern (all agents) |
| `LISA_AGENT_PROCESS_MATCH_CLAUDE` | - | Custom process match pattern (claude only) |
| `LISA_AGENT_PROCESS_MATCH_CODEX` | - | Custom process match pattern (codex only) |
| `LISA_CLEANUP_ALL_HASHES` | `false` | Default cleanup across all project hashes |
| `LISA_PROJECT_ROOT` | internal | Canonical project root used for runtime routing |
| `LISA_TMUX_SOCKET` | internal (`/tmp/lisa-tmux-<slug>-<hash>.sock`) | tmux socket path used by Lisa runtime |

## Notes

- Sessions isolated per project via MD5 hash of project root
- Session names: `lisa-{projectSlug}-{timestamp}-{agent}-{mode}[-{tag}]`
- Custom `--session` names must start with `lisa-`
- Commands > 500 chars sent via temp script (not send-keys)
- Text sent via tmux `load-buffer`/`paste-buffer` for safe multi-line delivery
- `--json` available on spawn/send/status/monitor/capture/explain/doctor/cleanup/build-cmd
- Spawn wraps commands with heartbeat loop + EXIT trap (done file + session marker)
- Claude gets `--dangerously-skip-permissions` by default (disable with `--no-dangerously-skip-permissions`)
- Tmux env vars set per session: `LISA_SESSION`, `LISA_SESSION_NAME`, `LISA_AGENT`, `LISA_MODE`, `LISA_PROJECT_HASH`, `LISA_HEARTBEAT_FILE`, `LISA_DONE_FILE`
- Raw pane capture strips startup noise by default; opt out with `--keep-noise`
- `session exists` supports `--project-root` for cross-root checks
- Nested runs: always pass `--project-root` and prefer `./lisa` inside prompts to avoid PATH drift
- After finishing Lisa usage (especially nested runs), run `$LISA_BIN cleanup` (`--dry-run` first in shared environments)

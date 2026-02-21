# Lisa Commands

Use with `LISA_BIN=./lisa` in this repo.

## session spawn

Create/start an agent session.

| Flag | Default | Description |
|---|---|---|
| `--agent` | `claude` | `claude` or `codex` |
| `--mode` | `interactive` | `interactive` or `exec` (aliases: `execution`, `non-interactive`) |
| `--prompt` | `""` | Initial prompt |
| `--project-root` | cwd | Project directory |
| `--session` | auto | Override name (must start with `lisa-`) |
| `--command` | `""` | Custom command (overrides agent CLI) |
| `--agent-args` | `""` | Extra args passed to agent CLI |
| `--width` | `220` | tmux pane width |
| `--height` | `60` | tmux pane height |
| `--no-dangerously-skip-permissions` | false | Disable Claude default skip-permissions flag |
| `--cleanup-all-hashes` | false | Clean artifacts across all project hashes |
| `--dry-run` | false | Print plan only; do not create session/artifacts |
| `--json` | false | JSON output |

JSON: `{"session","agent","mode","runId","projectRoot","command"}`

Spawn notes:
- `exec` requires `--prompt` unless `--command` is provided.
- If `--session` absent, Lisa auto-generates one.
- Codex `exec` defaults: `--full-auto --skip-git-repo-check`.
- Nested Codex hints (`./lisa`, `lisa session spawn`, `nested lisa`) auto-enable `--dangerously-bypass-approvals-and-sandbox` and omit `--full-auto`.
- If `--agent-args '--dangerously-bypass-approvals-and-sandbox'` is passed, Lisa omits `--full-auto` automatically.
- Non-nested Codex `exec` with `--full-auto` can block child Lisa tmux sockets (`Operation not permitted`); prefer interactive + `session send` or explicit bypass args.
- For deeply nested prompts, prefer heredoc injection (`PROMPT=$(cat <<'EOF' ... EOF)` then `--prompt "$PROMPT"`).
- `--dry-run` returns resolved `command`, wrapped `startupCommand`, `socketPath`, and injected `env` keys.

## session send

| Flag | Default | Description |
|---|---|---|
| `--session` | required | Session name |
| `--project-root` | cwd | Project directory |
| `--text` | `""` | Text to send (exclusive with `--keys`) |
| `--keys` | `""` | tmux key tokens (exclusive with `--text`) |
| `--enter` | false | Press Enter after send |
| `--json` | false | JSON output |

JSON: `{"session","ok","enter"}`

## session status

One-shot status poll.

| Flag | Default | Description |
|---|---|---|
| `--session` | required | Session name |
| `--project-root` | cwd | Project directory |
| `--agent` | `auto` | `auto`, `claude`, `codex` |
| `--mode` | `auto` | `auto`, `interactive`, `exec` |
| `--full` | false | Include classification/signal columns in CSV |
| `--fail-not-found` | false | Exit 1 when resolved status is `not_found` |
| `--json` | false | JSON output |

CSV: `status,todosDone,todosTotal,activeTask,waitEstimate,sessionState`

CSV with `--full`:
`status_full_v1,status,todosDone,todosTotal,activeTask,waitEstimate,sessionState,classificationReason,paneStatus,agentPid,agentCpu,outputAgeSeconds,heartbeatAge,promptWaiting,heartbeatFresh,stateLockTimedOut,stateLockWaitMs,agentScanError,tmuxReadError,stateReadError,metaReadError,doneFileReadError`

Status exits:
- default: exit `0` for all resolved statuses (including `not_found`)
- with `--fail-not-found`: exit `1` on `not_found`

## session monitor

Block until success/terminal condition per flags.

| Flag | Default | Description |
|---|---|---|
| `--session` | required | Session name |
| `--project-root` | cwd | Project directory |
| `--agent` | `auto` | Agent hint |
| `--mode` | `auto` | Mode hint |
| `--poll-interval` | `30` | Seconds between polls |
| `--max-polls` | `120` | Max polls |
| `--stop-on-waiting` | `true` | Stop on `waiting_input` |
| `--waiting-requires-turn-complete` | `false` | Require transcript turn-complete before waiting stop |
| `--until-marker` | `""` | Stop on marker text in raw pane output |
| `--expect` | `any` | `any`, `terminal`, `marker` |
| `--verbose` | false | Progress details to stderr |
| `--json` | false | JSON output |

JSON: `{"finalState","session","todosDone","todosTotal","outputFile","exitReason","polls","finalStatus"}`

Exit reasons:
`completed`, `crashed`, `not_found`, `stuck`, `waiting_input`, `waiting_input_turn_complete`, `marker_found`, `max_polls_exceeded`, `degraded_max_polls_exceeded`

Monitor nuance:
- Timeout often returns `finalState:"timeout"`, `exitReason:"max_polls_exceeded"`, `finalStatus:"active"`.
- `marker_found` is success, often before terminal completion (`in_progress`/`active`).
- `--waiting-requires-turn-complete true` can timeout in custom-command/non-transcript flows.
- `--expect terminal` on marker/waiting success returns `expected_terminal_got_*` (exit `2`).
- `--expect marker` when marker is not first success returns `expected_marker_got_*` (exit `2`).

Monitor exits:
- exit `0`: `completed`, `waiting_input`, `waiting_input_turn_complete`, `marker_found`
- exit `2`: `crashed`, `stuck`, `not_found`, `max_polls_exceeded`, `degraded_max_polls_exceeded`, `expected_*`

## session capture

Capture transcript (default for Claude) or raw pane output.

| Flag | Default | Description |
|---|---|---|
| `--session` | required | Session name |
| `--lines` | `200` | Pane lines for raw capture |
| `--raw` | false | Force raw tmux capture |
| `--keep-noise` | false | Keep Codex/MCP startup noise |
| `--strip-noise` | n/a | Compatibility alias for default filtering |
| `--project-root` | cwd | Project directory |
| `--json` | false | JSON output |

Capture behavior:
- Claude default reads `~/.claude/projects/{encoded-path}/{sessionId}.jsonl` for structured messages.
- Falls back to raw pane capture if transcript unavailable.
- Promptless/custom-command Claude sessions lacking prompt+createdAt metadata intentionally fall back to raw.
- Raw capture strips startup noise by default; use `--keep-noise` to preserve full pane output.

JSON:
- transcript: `{"session","claudeSession","messages":[{"role","text","timestamp"}]}`
- raw: `{"session","capture"}`

## session explain

Detailed diagnostics with recent event timeline.

| Flag | Default | Description |
|---|---|---|
| `--session` | required | Session name |
| `--project-root` | cwd | Project directory |
| `--agent` | `auto` | Agent hint |
| `--mode` | `auto` | Mode hint |
| `--events` | `10` | Number of events to show |
| `--json` | false | JSON output |

JSON: `{"status":{...},"eventFile","events":[...],"droppedEventLines"}`

## session list / exists / kill / kill-all / name

| Command | Key Flags | Output |
|---|---|---|
| `session list` | `--all-sockets`, `--project-only`, `--project-root`, `--json` | names (text) or JSON |
| `session exists` | `--session`, `--project-root`, `--json` | `true`/`false` (exit 0/1) or JSON |
| `session kill` | `--session`, `--project-root`, `--cleanup-all-hashes`, `--json` | `ok` or JSON |
| `session kill-all` | `--project-only`, `--project-root`, `--cleanup-all-hashes`, `--json` | `killed N sessions` or JSON |
| `session name` | `--agent`, `--mode`, `--project-root`, `--tag`, `--json` | name string or JSON |

Scope/retention:
- `session kill`/`kill-all` preserve event files for post-mortem.
- `session list` is socket-bound; pass explicit `--project-root` for deterministic scope.
- `session list --all-sockets` scans metadata-known project roots and returns active sessions only.

## session tree

Inspect metadata parent/child links for nested orchestration.

| Flag | Default | Description |
|---|---|---|
| `--session` | `""` | Optional root-session filter |
| `--project-root` | cwd | Project directory |
| `--all-hashes` | false | Include metadata from all project hashes |
| `--flat` | false | Machine-friendly parent/child rows |
| `--json` | false | JSON output |

JSON: `{"session","projectRoot","allHashes","nodeCount","roots":[{"session","parentSession","agent","mode","projectRoot","createdAt","children":[...]}]}`

## session smoke

Deterministic nested smoke (`L1 -> ... -> LN`) with marker assertions.

Flags: `--project-root`, `--levels` (1-4, default `3`), `--poll-interval`, `--max-polls`, `--keep-sessions`, `--json`.

Behavior: uses nested `session spawn/monitor/capture`, asserts all level markers, non-zero exit on spawn/monitor/marker failure.

## cleanup

Clean detached tmux servers and stale socket files from Lisa runs.

| Flag | Default | Description |
|---|---|---|
| `--dry-run` | false | Show removals/kills without mutating |
| `--include-tmux-default` | false | Also sweep `/tmp/tmux-*` default sockets |
| `--json` | false | JSON output |

JSON: `{"dryRun","scanned","removed","wouldRemove","killedServers","wouldKillServers","keptActive"}` plus optional `errors`.

Non-JSON output: one-line summary. Exit `1` if any probe/kill/remove errors occurred.

## Other commands

| Command | Purpose |
|---|---|
| `doctor [--json]` | Check prerequisites (tmux + at least one of claude/codex). Exit 0=ok, 1=missing |
| `agent build-cmd` | Preview agent CLI command (`--agent`, `--mode`, `--prompt`, `--agent-args`, `--no-dangerously-skip-permissions`, `--json`) |
| `skills sync` | Sync external skill into repo `skills/lisa` |
| `skills install` | Install repo `skills/lisa` to `codex`, `claude`, or `project` |
| `version` | Print build version (`version`, `--version`, `-v`) |

## Modes

| Mode | Agent runs as | Use for |
|---|---|---|
| `interactive` | REPL (default) | Multi-turn tasks, follow-up prompts |
| `exec` | One-shot (`claude -p` / `codex exec --full-auto`) | Single prompt, auto-exits |

Aliases `execution` and `non-interactive` map to `exec`.

## JSON Surface

`--json` exists on: `doctor`, `cleanup`, `agent build-cmd`, `skills sync`, `skills install`, `session name|spawn|send|status|explain|monitor|capture|tree|smoke|list|exists|kill|kill-all`.

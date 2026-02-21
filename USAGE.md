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

If you run from repo source, rebuild `./lisa` after code changes before validating CLI behavior.

Install options are also listed in `README.md` (Homebrew, `go install`, release archives/packages).

## Command Map

```text
lisa doctor
lisa cleanup
lisa version
lisa capabilities
lisa session name
lisa session spawn
lisa session detect-nested
lisa session send
lisa session snapshot
lisa session status
lisa session explain
lisa session monitor
lisa session capture
lisa session tree
lisa session smoke
lisa session preflight
lisa session list
lisa session exists
lisa session kill
lisa session kill-all
lisa agent build-cmd
lisa skills sync
lisa skills install
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

### `cleanup`

Sweep tmux socket residue (stale sockets + detached Lisa tmux servers).

```bash
lisa cleanup
lisa cleanup --dry-run
lisa cleanup --json
lisa cleanup --include-tmux-default
```

Flags:

- `--dry-run`: print what would be removed/killed
- `--include-tmux-default`: also sweep `/tmp/tmux-*` default sockets
- `--json`: JSON summary

Behavior:

- Probes each candidate socket for reachability + client count.
- Unreachable sockets are treated as stale and removed.
- Reachable sockets with zero clients are treated as detached servers; Lisa runs `kill-server` then removes stale socket files when possible.
- Reachable sockets with active clients are kept.
- `--dry-run` reports `wouldKillServers` / `wouldRemove` without mutation.
- Any probe/kill/remove failures print per-socket errors to stderr and exit `1`.

### `capabilities`

Describe current CLI command/flag support for orchestration clients.

```bash
lisa capabilities
lisa capabilities --json
```

Flags:

- `--json`: JSON output including build metadata and command+flag matrix

### `skills sync`

Sync an external Lisa skill directory into this repo's `skills/lisa`.

```bash
lisa skills sync --from codex --repo-root /path/to/lisa-repo
lisa skills sync --from claude --repo-root /path/to/lisa-repo
lisa skills sync --from path --path /tmp/lisa-skill --repo-root /path/to/lisa-repo
```

Flags:

- `--from`: `codex|claude|path` (default `codex`)
- `--path`: required when `--from path`
- `--repo-root`: repo root containing `skills/` (default cwd)
- `--json`: JSON summary output

### `skills install`

Install repo `skills/lisa` into Codex, Claude, or a project path.

```bash
lisa skills install --to codex --repo-root /path/to/lisa-repo
lisa skills install --to claude --repo-root /path/to/lisa-repo
lisa skills install --to project --project-path /tmp/target-project --repo-root /path/to/lisa-repo
lisa skills install --repo-root /path/to/lisa-repo   # auto: install to available ~/.codex and ~/.claude
```

Flags:

- `--to`: `codex|claude|project` (default `auto`)
- `--project-path`: required when `--to project` (installs to `<project>/skills/lisa`)
- `--path`: explicit destination path override
- `--repo-root`: repo root containing `skills/` (default cwd)
- `--json`: JSON summary output

Auto target behavior (when `--to` and `--path` are omitted):

- installs to all available targets among `~/.codex` and `~/.claude`
- errors if neither exists (use `--to` or `--path` to override)

Source behavior:

- local/dev builds (`version=dev`) read from repo `skills/lisa`
- tagged release builds fetch `skills/lisa` from GitHub tag matching the binary version (fallback: `main`)

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
- `--json`

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
- `--nested-policy`: `auto|force|off` (default `auto`)
- `--nesting-intent`: `auto|nested|neutral` (default `auto`)
- `--session`: explicit name (must start with `lisa-`)
- `--prompt`: startup prompt
- `--command`: full command override (skips agent command builder)
- `--agent-args`: extra args appended to agent CLI
- `--model`: Codex model name (supported with `--agent codex`; e.g. `GPT-5.3-Codex-Spark`)
- `--project-root`: project isolation root (default cwd)
- `--width`: tmux width (default `220`)
- `--height`: tmux height (default `60`)
- `--cleanup-all-hashes`: clean artifacts across all project hashes
- `--dry-run`: print resolved spawn plan (command/socket/env) without creating tmux session or artifacts
- `--detect-nested`: include nested bypass detection diagnostics in JSON output
- `--no-dangerously-skip-permissions`: disable default Claude permission-skip flag injection
- `--json`: machine-readable output

Notes:

- For Claude, Lisa injects `--dangerously-skip-permissions` by default unless disabled.
- `exec` mode requires a prompt unless `--command` is provided.
- If no `--session`, Lisa auto-generates one.
- Codex `exec` defaults include `--full-auto` and `--skip-git-repo-check`.
- Nested Codex prompts: when prompt text suggests Lisa nesting (`./lisa`, `lisa session spawn`, `nested lisa`), Lisa auto-adds `--dangerously-bypass-approvals-and-sandbox` and omits `--full-auto`.
- Quote/doc guard: non-executable references like `The string './lisa' appears in docs only.` do not auto-trigger bypass.
- `--nested-policy force` enables Codex nested bypass without relying on prompt wording (and omits `--full-auto`).
- `--nested-policy off` disables prompt-based nested bypass heuristics.
- `--nesting-intent nested|neutral` explicitly overrides prompt heuristics.
- For non-nested Codex `exec`, `--full-auto` sandbox can still block tmux socket creation for child Lisa sessions (`Operation not permitted`); use `--mode interactive` + `session send` or pass explicit bypass args.
- If you pass `--agent-args '--dangerously-bypass-approvals-and-sandbox'`, Lisa omits `--full-auto` automatically (Codex rejects combining both flags).
- Use `--model GPT-5.3-Codex-Spark` (or another Codex model name) to inject `--model` without manual `--agent-args` quoting.
- For deeply nested prompt chains, prefer heredoc prompt injection (`PROMPT=$(cat <<'EOF' ... EOF)` then `--prompt "$PROMPT"`) to avoid shell quoting collisions in inline nested commands.
- Spawned panes receive `LISA_*` routing env (see Runtime Environment Variables) so nested Lisa commands preserve project/socket isolation.
- `--dry-run` validates inputs and returns planned spawn payload (`session`, `command`, wrapped `startupCommand`, `socketPath`, injected env vars) without creating a session.
- `--detect-nested --json` adds `nestedDetection` with decision fields (`autoBypass`, `reason`, `matchedHint`, arg/full-auto signals, effective command flags).

### `session detect-nested`

Probe nested Codex bypass decisions without spawning tmux sessions.

```bash
lisa session detect-nested --prompt "Use ./lisa for child orchestration." --json
```

Flags:

- `--agent`: `claude|codex` (default `codex`)
- `--mode`: `interactive|exec` (default `exec`)
- `--nested-policy`: `auto|force|off` (default `auto`)
- `--nesting-intent`: `auto|nested|neutral` (default `auto`)
- `--prompt`
- `--agent-args`
- `--model`
- `--project-root`
- `--json`

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
- `--json-min`: minimal JSON ack (`session`, `ok`)

### `session snapshot`

One-shot poll helper: status + raw capture + `nextOffset` in one call.

```bash
lisa session snapshot --session <NAME> --json-min
```

Flags:

- `--session` (required)
- `--agent`: `auto|claude|codex`
- `--mode`: `auto|interactive|exec`
- `--project-root`
- `--lines N` (default `200`)
- `--delta-from VALUE`
- `--markers CSV` (marker-only extraction mode)
- `--keep-noise`
- `--strip-noise`
- `--fail-not-found`
- `--json`
- `--json-min`

### `session status`

One-shot session status snapshot.

```bash
lisa session status --session <NAME>
lisa session status --session <NAME> --full
lisa session status --session <NAME> --json
lisa session status --session <NAME> --json --fail-not-found
```

Flags:

- `--session` (required)
- `--agent`: `auto|claude|codex` (default `auto`)
- `--mode`: `auto|interactive|exec` (default `auto`)
- `--project-root` (default cwd)
- `--full`: include classification/signal columns in CSV mode
- `--fail-not-found`: exit `1` when resolved state is `not_found`
- `--json`
- `--json-min`: minimal JSON (`session`, `status`, `sessionState`, `todosDone`, `todosTotal`, `waitEstimate`)

Output note:

- `sessionState` is the lifecycle state.
- `status` is normalized to match terminal lifecycle states (`completed`, `crashed`, `stuck`, `not_found`) so JSON/CSV no longer report `status=idle` for terminal outcomes.

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
- `--recent N` (alias for `--events`)
- `--json`
- `--json-min` (minimal JSON: `session`, `status`, `sessionState`, `reason`, `recent`)

Output note:

- Embedded `status` payload uses the same terminal normalization as `session status` (`completed`, `crashed`, `stuck`, `not_found`).

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
- `--until-marker TEXT`: stop successfully when raw pane output contains marker text
- `--expect any|terminal|marker` (default `any`)
- `--json`
- `--json-min` (minimal JSON: `session`, `finalState`, `exitReason`, `polls`)
- `--stream-json` (line-delimited JSON poll events before final result)
- `--verbose`

Output note:

- `finalState` is the terminal/stop-state from monitor.
- `finalStatus` is normalized for terminal monitor states (`completed`, `crashed`, `stuck`, `not_found`) so it aligns with `finalState` in JSON/CSV output.
- Timeout exits use `finalState=timeout` and `finalStatus=timeout`.
- `--stream-json` emits one JSON object per poll (`type=poll`), then emits the standard final monitor JSON payload.
- Final monitor JSON includes `nextOffset` when pane capture is available (ready for follow-up delta capture polling).

When `--waiting-requires-turn-complete true` is set, `monitor` only stops on
`waiting_input` after transcript tail inspection confirms an assistant turn is
complete (Claude/Codex interactive sessions with prompt metadata).
When this path is taken, `exitReason=waiting_input_turn_complete` (exit `0`) and lifecycle reason is `monitor_waiting_input_turn_complete`.
When `--until-marker` is set and marker text appears in pane output, monitor exits `0` with `exitReason=marker_found`, often while `finalState=in_progress`.
`--expect terminal` fails fast on `marker_found`/`waiting_input` success cases (`exitReason=expected_terminal_got_*`, exit `2`).
`--expect marker` fails fast if a terminal/waiting reason occurs before marker match (`exitReason=expected_marker_got_*`, exit `2`).

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
- `--delta-from VALUE`: delta start (`offset` integer, `@unix` timestamp, or RFC3339 timestamp; requires `--raw`)
- `--markers CSV`: marker-only extraction mode (comma-separated markers)
- `--keep-noise`: keep Codex/MCP startup noise in pane capture
- `--strip-noise`: compatibility alias to force default noise filtering
- `--lines N`: pane lines for raw capture (default `200`)
- `--project-root`
- `--agent` (optional model-probe agent; currently `codex`)
- `--model` (optional codex model probe)
- `--json`
- `--json-min` (compact JSON payloads for polling workflows)

Behavior:

- default: for Claude sessions, tries transcript capture first
- fallback: raw tmux pane capture if transcript path fails/unavailable
- raw capture path filters known Codex/MCP startup noise by default
- `--keep-noise`: disables that filtering
- `--strip-noise`: compatibility alias for default filtering (legacy scripts)
- `--delta-from` supports low-token polling:
  - offset mode (`--delta-from 1200`): returns capture bytes after offset
  - timestamp mode (`--delta-from @1700000000` or RFC3339): returns full capture only if output changed after timestamp
  - JSON capture includes `deltaMode` and `nextOffset` for subsequent polls
- `--json-min` keeps compact capture payloads (and includes `nextOffset` for delta polling).

### `session preflight`

Validate environment and key command contracts in one call.

```bash
lisa session preflight
lisa session preflight --json
```

Flags:

- `--project-root`
- `--json`

Behavior:

- Runs doctor-equivalent environment checks (`tmux`, `claude`, `codex`).
- Validates critical parser/contract assumptions (mode aliases, monitor marker guard, capture delta parsing, nested codex hint routing).
- Optional model probe: `--agent codex --model <NAME>` runs a real Codex model-availability check.
- Returns exit `0` when both environment and contract checks pass; else exit `1`.

### `session list`

List tmux sessions with `lisa-` prefix.

```bash
lisa session list
lisa session list --project-only
lisa session list --all-sockets
```

Flags:

- `--all-sockets`: discover active sessions across project sockets by replaying metadata roots
- `--project-only`
- `--stale`: include metadata historical/stale counts (+ stale list in full JSON/text)
- `--project-root`
- `--json`
- `--json-min`: minimal JSON (`sessions`, `count`)
- `--json-min` with `--stale` includes `historicalCount` + `staleCount`.

Behavior note:

- Default `session list` is current-socket scoped.
- `--all-sockets` expands discovery across metadata-known project roots and includes sessions currently active on those roots.

### `session tree`

Show parent/child hierarchy from session metadata.

```bash
lisa session tree --json
lisa session tree --session <ROOT_SESSION> --json
lisa session tree --flat
```

Flags:

- `--session` (optional root filter)
- `--project-root`
- `--all-hashes` (scan metadata across all project hashes)
- `--active-only` (include only sessions currently active in tmux)
- `--flat` (machine-friendly parent/child rows)
- `--json`
- `--json-min`: minimal JSON (`nodeCount` plus session graph rows/roots)

Behavior note:

- `session tree` is metadata-first and can show historical sessions.
- Use `--active-only` (or pair with `session list`) for active-only topology.

### `session smoke`

Deterministic nested Lisa smoke test (`L1 -> ... -> LN`) with marker assertions.

```bash
lisa session smoke --levels 3
lisa session smoke --levels 4 --json
```

Flags:

- `--project-root`
- `--levels N` (1-4, default `3`)
- `--prompt-style STYLE` (`none|dot-slash|spawn|nested|neutral`, default `none`)
- `--matrix-file PATH`: prompt regression matrix (`mode|prompt`, mode = `bypass|full-auto|any`)
- `--poll-interval N` (default `1`)
- `--max-polls N` (default `180`)
- `--keep-sessions`
- `--json`

Behavior:

- Creates nested interactive sessions using `session spawn/monitor/capture`.
- Asserts deterministic markers from every level in `L1` capture.
- Returns non-zero on any missing marker, spawn/monitor failure, or timeout.
- Optional `--prompt-style` runs a nested-wording probe (`session spawn --dry-run --detect-nested --json`) before smoke execution and records probe result in JSON summary.
- Optional `--matrix-file` runs a multi-prompt regression sweep before smoke execution and fails on expectation mismatch.

### `session exists`

Check existence of one session.

```bash
lisa session exists --session <NAME>
```

Flags:

- `--session` (required)
- `--project-root` (default cwd)
- `--json`

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
- `--json`

Behavior note:

- If metadata links descendants (`parentSession`), `session kill` kills descendants first, then the target session.
- Artifact cleanup is attempted even if target session is already missing or tmux kill returns an error.
- `--cleanup-all-hashes` extends artifact cleanup across all project-hash variants.
- Stale event-artifact retention pruning runs after kill cleanup.

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
- `--json`

### `agent build-cmd`

Build agent startup command without spawning tmux session.

```bash
lisa agent build-cmd --agent codex --mode exec --prompt "Run tests"
lisa agent build-cmd --agent codex --mode exec --model GPT-5.3-Codex-Spark --prompt "Run tests"
lisa agent build-cmd --agent claude --mode interactive --prompt "Review diff" --json
```

Flags:

- `--agent`: `claude|codex`
- `--mode`: `interactive|exec`
- `--nested-policy`: `auto|force|off` (default `auto`)
- `--nesting-intent`: `auto|nested|neutral` (default `auto`)
- `--project-root` (context only; included in JSON payload)
- `--prompt`
- `--agent-args`
- `--model`: Codex model name (supported with `--agent codex`)
- `--no-dangerously-skip-permissions`
- `--json`

## Output Modes

JSON support:

- `doctor`
- `cleanup`
- `agent build-cmd`
- `session spawn`
- `session send`
- `session snapshot`
- `session status`
- `session explain`
- `session monitor`
- `session capture`
- `session tree`
- `session smoke`
- `session preflight`
- `session detect-nested`
- `session name`
- `session list`
- `session exists`
- `session kill`
- `session kill-all`

JSON failure contract:

- With `--json`, command/runtime failures emit `{"ok":false,"errorCode":"...","error":"..."}`.
- Stateful JSON failures (for example `session kill`, `session exists`, `session monitor`, `session smoke`) also include command-specific payload fields plus `errorCode`.
- JSON responses include `stderrPolicy` so LLM/tool callers can treat stderr as diagnostic stream.

Text/CSV-only commands:

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

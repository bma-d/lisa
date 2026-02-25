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
lisa oauth add
lisa oauth list
lisa oauth remove
lisa session name
lisa session spawn
lisa session detect-nested
lisa session send
lisa session turn
lisa session snapshot
lisa session status
lisa session explain
lisa session monitor
lisa session capture
lisa session contract-check
lisa session schema
lisa session checkpoint
lisa session dedupe
lisa session next
lisa session aggregate
lisa session prompt-lint
lisa session diff-pack
lisa session loop
lisa session context-cache
lisa session anomaly
lisa session budget-observe
lisa session budget-enforce
lisa session budget-plan
lisa session replay
lisa session handoff
lisa session packet
lisa session context-pack
lisa session route
lisa session autopilot
lisa session objective
lisa session memory
lisa session lane
lisa session state-sandbox
lisa session guard
lisa session tree
lisa session smoke
lisa session preflight
lisa session list
lisa session exists
lisa session kill
lisa session kill-all
lisa agent build-cmd
lisa skills sync
lisa skills doctor
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

### `oauth add`

Add a Claude OAuth token to Lisa's local token pool (`~/.lisa/oauth_tokens.json`, mode `0600`).

```bash
lisa oauth add --token "<oauth-token>"
printf '%s\n' "$CLAUDE_CODE_OAUTH_TOKEN" | lisa oauth add --stdin
lisa oauth add --stdin --json
```

Flags:

- `--token`: token value
- `--stdin`: read token from stdin
- `--json`: JSON output

Behavior:

- Deduplicates by token value.
- Stored tokens are used for Claude session spawns in round-robin order.
- When a spawned Claude session reports OAuth refresh failures (invalid/expired), Lisa removes that token automatically.

### `oauth list`

List token ids in the local pool.

```bash
lisa oauth list
lisa oauth list --json
```

Flags:

- `--json`: JSON output

### `oauth remove`

Remove token by id.

```bash
lisa oauth remove --id oauth-abc123def456
lisa oauth remove --id oauth-abc123def456 --json
```

Flags:

- `--id`: token id
- `--json`: JSON output

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
- `--deep`: include recursive content-hash drift checks
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

### `skills doctor`

Check installed Codex/Claude Lisa skill drift against repo version + capability contract.

```bash
lisa skills doctor --repo-root /path/to/lisa-repo
lisa skills doctor --json
```

Flags:

- `--repo-root`: repo root containing `skills/` (default cwd)
- `--deep`: include recursive content-hash drift checks
- `--explain-drift`: include remediation guidance for drift findings
- `--fix`: auto-install repo skill to outdated/missing targets
- `--contract-check`: include command/flag contract drift checks
- `--sync-plan`: include machine-readable install/sync plan
- `--json`: JSON summary output

Behavior notes:

- `--deep` computes recursive content hashes for repo and installed skill trees; matching versions can still report `content drift`.
- `--explain-drift` adds per-target remediation hints (`remediation`) in JSON and text output.
- `--fix` installs repo skill to drifted targets, then re-runs checks.
- `--contract-check` adds command/flag surface drift checks (including session contract checks).
- `--sync-plan` adds prioritized install/sync commands in `syncPlan`.

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
- `--model`: Codex model name (supported with `--agent codex`; e.g. `gpt-5.3-codex`)
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
- Use `--model gpt-5.3-codex-spark` (or another Codex model name) to inject `--model` without manual `--agent-args` quoting.
- Model IDs are case-sensitive in practice; mixed-case aliases may fail preflight probes.
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
- `--rewrite`: include trigger-safe prompt rewrite suggestions
- `--why`: include hint-span reasoning payload
- `--json`

Behavior notes:

- `--why` adds a `why` payload with nested decision fields plus matched hint spans/context classification.
- In text mode, `--why` prints `why: ...` after the primary detection line.

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

Behavior notes:

- When objective metadata is active for a session, Lisa prepends an `Objective reminder: ...` block to `--text` payloads before sending input.
- For Codex interactive sessions, `--text ... --enter` uses a staged submit path (paste, short settle, then Enter) to improve multi-turn follow-up reliability.

### `session turn`

One-shot orchestration turn: `send -> monitor -> packet`.

```bash
lisa session turn --session <NAME> --text "Continue from latest blocker summary" --enter --json
lisa session turn --session <NAME> --keys "C-c" --expect terminal --json-min
```

Flags:

- `--session` (required)
- `--project-root` (default cwd)
- `--text` or `--keys` (exactly one required)
- `--enter`
- monitor pass-through: `--agent`, `--mode`, `--expect`, `--poll-interval`, `--max-polls`, `--timeout-seconds`, `--stop-on-waiting`, `--waiting-requires-turn-complete`, `--until-marker`, `--until-state`, `--until-jsonpath`, `--auto-recover`, `--recover-max`, `--recover-budget`
- packet pass-through: `--lines`, `--events`, `--token-budget`, `--summary-style`, `--cursor-file`, `--fields`
- `--json`
- `--json-min`

Behavior notes:

- Returns step-scoped failure payload (`failedStep`, `errorCode`) and propagates non-zero exit from the failing step.
- `--json-min` returns compact turn outcome (`session`, `finalState`, `exitReason`, `status`, `sessionState`, `nextAction` when available).

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
- `--adaptive-poll`: auto-tune poll interval by heartbeat/state health
- `--max-polls N` (default `120`)
- `--timeout-seconds N`: optional wall-clock timeout budget in seconds
- `--stop-on-waiting true|false` (default `true`)
- `--waiting-requires-turn-complete true|false` (default `false`)
- `--until-marker TEXT`: stop successfully when raw pane output contains marker text
- `--until-state STATE`: stop when session reaches target state (`waiting_input|completed|crashed|...`)
- `--until-jsonpath EXPR`: stop when status JSON path expression resolves true/matches
- `--expect any|terminal|marker` (default `any`)
- `--json`
- `--json-min` (minimal JSON: `session`, `finalState`, `exitReason`, `polls`)
- `--stream-json` (line-delimited JSON poll events before final result)
- `--emit-handoff` (line-delimited compact handoff packets per poll; requires `--stream-json`)
- `--handoff-cursor-file PATH`: persist/reuse handoff delta offset (`--emit-handoff` only)
- `--event-budget N`: token budget hint for streamed handoff deltas (`--emit-handoff` only)
- `--webhook TARGET`: emit poll/final monitor events to file path or HTTPS endpoint
- `--auto-recover`: retry once on max-polls/degraded timeout via safe Enter nudge
- `--recover-max N`: maximum auto-recover attempts (default `1`)
- `--recover-budget N`: optional total poll budget across recover attempts
- `--verbose`

Output note:

- `finalState` is the terminal/stop-state from monitor.
- `finalStatus` is normalized for terminal monitor states (`completed`, `crashed`, `stuck`, `not_found`) so it aligns with `finalState` in JSON/CSV output.
- Timeout exits use `finalState=timeout` and `finalStatus=timeout`.
- `--stream-json` emits one JSON object per poll (`type=poll`), then emits the standard final monitor JSON payload.
- `--emit-handoff` adds `type=handoff` packets per poll for low-token multi-agent relay loops.
- `--handoff-cursor-file` and/or `--event-budget` switch handoff stream output to incremental deltas (`deltaFrom`, `nextDeltaOffset`, `recent`), otherwise handoff packets are summary-only.
- `--event-budget` maps to handoff delta event window size (approx `budget/32`, clamped to `1..24`, default delta window `8` when unset).
- Final monitor JSON includes `nextOffset` when pane capture is available (ready for follow-up delta capture polling).
- `--emit-handoff` requires `--stream-json`; `--handoff-cursor-file` and `--event-budget` both require `--emit-handoff`.

When `--waiting-requires-turn-complete true` is set, `monitor` only stops on
`waiting_input` after transcript tail inspection confirms an assistant turn is
complete (Claude/Codex interactive sessions with prompt metadata).
When this path is taken, `exitReason=waiting_input_turn_complete` (exit `0`) and lifecycle reason is `monitor_waiting_input_turn_complete`.
When `--until-marker` is set and marker text appears in pane output, monitor exits `0` with `exitReason=marker_found`, often while `finalState=in_progress`.
In interactive multi-turn flows, default `--stop-on-waiting true` can exit early with `waiting_input` before a later marker appears; for deterministic follow-up marker gating, use `--stop-on-waiting false` (or `--waiting-requires-turn-complete true` when transcript metadata is available).
`--expect terminal` fails fast on `marker_found`/`waiting_input` success cases (`exitReason=expected_terminal_got_*`, exit `2`).
`--expect marker` fails fast if a terminal/waiting reason occurs before marker match (`exitReason=expected_marker_got_*`, exit `2`).
On timeout/degraded exits, JSON can still include useful intermediate payloads; treat non-zero as contract signal, not empty output.

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
- `--cursor-file PATH`: persist/reuse raw capture offsets (`--raw` only)
- `--markers CSV`: marker-only extraction mode (comma-separated markers)
- `--markers-json`: include structured marker hits (`markerHits`) with offsets/line numbers (requires `--markers`)
- `--summary`: return bounded summary instead of full capture body
- `--summary-style MODE`: `terse|ops|debug` (default `terse`; requires `--summary`)
- `--token-budget N`: summary budget in approximate tokens (default `320`)
- `--semantic-delta`: return meaning-level delta (`semanticDelta`) against semantic cursor state (`--raw` only)
- `--keep-noise`: keep Codex/MCP startup noise in pane capture
- `--strip-noise`: compatibility alias to force default noise filtering
- `--strip-banner`: remove status/banner chrome from raw capture output
- `--lines N`: pane lines for raw capture (default `200`)
- `--project-root`
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
- `--cursor-file` auto-loads prior offset when `--delta-from` is omitted and writes back `nextOffset`.
- `--json-min` keeps compact capture payloads (and includes `nextOffset` for delta polling).
- `--summary` cannot be combined with `--markers`.
- `--summary-style` defaults to `terse`; non-default styles require `--summary`.
- `--markers-json` requires `--markers` and implies JSON output.
- `--semantic-delta` requires `--raw` and cannot be combined with `--markers`.
- `--semantic-delta` with `--cursor-file` reuses/persists semantic baseline; without `--cursor-file`, baseline is empty each call.
- With `--semantic-delta`, text output prints semantic delta lines (and `--summary` summarizes semantic delta text).

### `session schema`

Emit JSON schema contracts for session payloads.

```bash
lisa session schema
lisa session schema --command "session guard" --json
```

Flags:

- `--command`: optional schema selector (accepts `session <command>` or `<command>`)
- `--json`

Behavior:

- Without `--command`, text output lists available schema commands; JSON returns full command->schema catalog.
- With `--command`, returns only the selected schema payload.
- Unknown schema selectors fail with non-zero exit.

### `session checkpoint`

Save/resume orchestration state bundles.

```bash
lisa session checkpoint save --session <NAME> --file /tmp/lisa-checkpoint.json
lisa session checkpoint resume --file /tmp/lisa-checkpoint.json --json
```

Flags:

- `--action`: `save|resume` (default `save`)
- `--session`: required for `save`; optional resume guard (must match checkpoint session when provided)
- `--file` (required)
- `--project-root`
- `--events N` (default `8`)
- `--lines N` (default `120`)
- `--strategy`: `terse|balanced|full` (default `balanced`)
- `--token-budget N` (default `700`)
- `--json`

Behavior:

- `save` captures status/session state, recent events, context pack, and capture tail into the checkpoint file.
- `save` fails when the session cannot be resolved.
- `resume` loads and returns checkpoint metadata/payload; mismatched `--session` fails non-zero.

### `session dedupe`

Prevent duplicate work using task-hash claims.

```bash
lisa session dedupe --task-hash <HASH>
lisa session dedupe --task-hash <HASH> --session <NAME> --json
lisa session dedupe --task-hash <HASH> --release
```

Flags:

- `--task-hash` (required)
- `--session`: claim hash for this session/root
- `--release`: release existing hash claim
- `--project-root`
- `--json`

Behavior:

- Stores claims in `/tmp/.lisa-<project_hash>-dedupe.json`; stale claims are auto-pruned.
- Query mode (no `--session`, no `--release`) exits non-zero only when an active duplicate exists.
- Claim mode (`--session`) exits non-zero if hash is already claimed by an active different session.
- Release mode is idempotent and exits zero.

### `session handoff`

Build compact handoff payload for multi-agent orchestration.

```bash
lisa session handoff --session <NAME> --json-min
```

Flags:

- `--session` (required)
- `--project-root`
- `--agent`: `auto|claude|codex`
- `--mode`: `auto|interactive|exec`
- `--events N` (default `8`)
- `--delta-from N`: incremental event offset (non-negative integer)
- `--cursor-file PATH`: persist/reuse incremental event offset
- `--compress MODE`: `none|zstd` (default `none`)
- `--schema MODE`: `v1|v2|v3|v4` (default `v1`)
- `--json`
- `--json-min`

Delta handoff behavior:

- `--delta-from` returns events after that offset and includes `nextDeltaOffset` for next incremental pull.
- `--schema v4` emits typed `nextAction.commandAst` plus deterministic action identifiers.
- `--json-min` still includes the compact `recent` delta list when `--delta-from` is used.
- If active lane contract includes `handoff_v2_required`, handoff requires `--schema v2|v3|v4` and returns `errorCode=handoff_schema_v2_required` otherwise.

### `session packet`

Build one-shot status + capture summary + handoff packet.

```bash
lisa session packet --session <NAME> --json
lisa session packet --session <NAME> --cursor-file /tmp/lisa.packet.cursor --json-min
lisa session packet --session <NAME> --fields session,nextAction,nextOffset --json
```

Flags:

- `--session` (required)
- `--project-root`
- `--agent`: `auto|claude|codex`
- `--mode`: `auto|interactive|exec`
- `--lines N` (default `120`)
- `--events N` (default `8`)
- `--token-budget N` (default `320`)
- `--summary-style`: `terse|ops|debug` (default `ops`)
- `--cursor-file PATH`: persist/reuse handoff delta offset
- `--delta-json`: include field-level packet delta payloads (`--cursor-file` required)
- `--fields CSV`: project JSON payload to selected fields (requires `--json`)
- `--json`
- `--json-min`

Behavior notes:

- `--json-min` emits compact packet fields plus `recent`.
- `--cursor-file` switches handoff output to incremental delta fields: `deltaFrom`, `nextDeltaOffset`, `deltaCount`.
- `--delta-json` emits `delta.added|removed|changed` for selected packet fields and persists cursor snapshots.
- `--fields` supports dotted JSON path projection for low-token caller payloads.
- Missing session still emits packet JSON with `errorCode=session_not_found` and exits non-zero.

### `session context-pack`

Build token-budgeted context packet (`state + recent events + capture tail`).

```bash
lisa session context-pack --for <NAME> --token-budget 700 --json-min
```

Flags:

- `--for` / `--session` (required unless `--from-handoff` payload includes `session`)
- `--project-root`
- `--agent`: `auto|claude|codex`
- `--mode`: `auto|interactive|exec`
- `--events N` (default `8`)
- `--lines N` (default `120`)
- `--token-budget N` (default `700`)
- `--strategy`: `terse|balanced|full` (default `balanced`; adjusts default events/lines/token-budget)
- `--from-handoff PATH`: build from handoff JSON payload (`-` for stdin)
- `--redact CSV`: redaction rules `none|all|paths|emails|secrets|numbers|tokens`
- `--json`
- `--json-min`

Behavior notes:

- `--from-handoff` builds from handoff JSON payload fields instead of live tmux state polling.
- `--for` and `--from-handoff` must reference the same session when both are set.
- `--from-handoff` accepts `nextAction` as either string (v1) or object payload (`name`/`command` in v2/v3/v4).
- `--redact` applies in-pack redaction before JSON emission; `none` cannot be combined with other rules.
- Active redaction rules are returned in `redactRules` (`--json` and `--json-min`).

### `session route`

Recommend session mode/policy/model for orchestration goal.

```bash
lisa session route --goal nested --json
```

Flags:

- `--goal`: `nested|analysis|exec` (default `analysis`)
- `--agent`: `claude|codex` (default `codex`)
- `--lane NAME`: optional lane defaults/contracts source
- `--prompt`: optional override
- `--model`: optional codex model override
- `--profile NAME`: route profile `codex-spark|claude`
- `--budget N`: optional token budget hint for runbook/capture
- `--queue`: include prioritized session dispatch queue
- `--sessions CSV`: optional explicit sessions for queue mode
- `--queue-limit N`: optional cap on returned queue items
- `--concurrency N`: dispatch concurrency cap for queue planning
- `--topology CSV`: optional roles `planner,workers,reviewer`
- `--cost-estimate`: include token/time estimate payload
- `--from-state PATH`: route using handoff/status JSON payload (`-` for stdin)
- `--strict`: fail fast on invalid `--from-state` schema/fields
- `--project-root`
- `--emit-runbook`: include executable spawn/monitor/capture/handoff/cleanup plan JSON
- `--json`

Behavior notes:

- `--topology` adds a topology graph payload (`roles`, `nodes`, `edges`) to the JSON response.
- `--cost-estimate` adds `costEstimate` (`totalTokens`, `totalSeconds`, per-step estimates).
- Cost estimate scales by goal/mode and topology roles; `--budget` also influences capture-step token estimate.
- `--from-state` accepts handoff/status JSON (`session`, `sessionState`, `reason`, `nextAction`); parsed input is echoed as `fromState` in JSON output.
- Typed `nextAction` objects from `--from-state` are preserved in generated prompts/runbooks (no map-stringification).
- If `--prompt` is omitted, `--from-state` builds a continuation prompt from state fields; explicit `--prompt` overrides it.

### `session autopilot`

Run end-to-end orchestration (`spawn -> monitor -> capture -> handoff`, optional cleanup).

```bash
lisa session autopilot --goal analysis --json
```

Flags:

- `--goal`: `nested|analysis|exec` (default `analysis`)
- `--agent`: `claude|codex` (default `codex`)
- `--lane NAME`: optional lane defaults/contracts source
- `--mode`: optional override `interactive|exec`
- `--nested-policy`: optional override `auto|force|off`
- `--nesting-intent`: optional override `auto|nested|neutral`
- `--session NAME`: optional explicit session name (`lisa-*`)
- `--prompt`: optional override
- `--model`: optional codex model override
- `--project-root`
- `--poll-interval N`: monitor poll interval seconds (default `30`)
- `--max-polls N`: monitor max polls (default `120`)
- `--capture-lines N`: raw capture lines for capture step (default `220`)
- `--summary`: capture summary instead of full raw output
- `--summary-style`: `terse|ops|debug` (default `ops`; requires `--summary`)
- `--token-budget N`: summary token budget when `--summary` is set (default `320`)
- `--kill-after true|false`: kill session after handoff (default `false`)
- `--resume-from PATH`: resume from previous autopilot JSON summary (`-` for stdin)
- `--json`

Behavior notes:

- `--resume-from` loads a prior autopilot summary, resumes from the first failed/incomplete step, and preserves completed-step payloads.
- Resume inherits prior `mode`, `goal` (when still default), `session`, and `killAfter` unless explicit flags override.
- If the prior summary is already complete, autopilot returns a resume no-op.

### `session guard`

Shared tmux safety guardrails before cleanup/kill-all.

```bash
lisa session guard --shared-tmux --json
lisa session guard --shared-tmux --enforce --json
lisa session guard --shared-tmux --command "lisa cleanup --include-tmux-default" --json
```

Flags:

- `--shared-tmux` (required)
- `--enforce`: escalate medium/high risk findings to hard failure (exit non-zero)
- `--advice-only`: diagnostics only; always exit zero
- `--machine-policy`: `strict|warn|off` (default `strict`) for risk exit policy
- `--command`: optional command-risk check
- `--policy-file PATH`: optional JSON policy contract for guard evaluation
- `--project-root`
- `--json`

Behavior note:

- `--machine-policy strict` returns non-zero when guard is unsafe (unless `--advice-only` is set).
- `--machine-policy warn|off` keeps advisory output and exits zero.

### `session preflight`

Validate environment and key command contracts in one call.

```bash
lisa session preflight
lisa session preflight --json
```

Flags:

- `--project-root`
- `--agent` (optional model-probe agent; currently `codex`)
- `--model` (optional codex model probe)
- `--auto-model` (probe candidate models and select first supported)
- `--auto-model-candidates CSV` (default `gpt-5.3-codex,gpt-5-codex`)
- `--fast` (run reduced high-risk contract checks only)
- `--json`

Behavior:

- Runs doctor-equivalent environment checks (`tmux`, `claude`, `codex`).
- Validates critical parser/contract assumptions (mode aliases, monitor marker guard, capture delta parsing, nested codex hint routing).
- `--fast` keeps environment checks but runs a reduced contract set focused on high-risk monitor/capture/nested-bypass guards.
- Optional model probe: `--agent codex --model <NAME>` runs a real Codex model-availability check.
- Optional auto-model probe: `--auto-model` runs candidate model probes and selects first supported.
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
- `--active-only`: filter out sessions currently resolving to `not_found`
- `--with-next-action`: include per-session `status`, `sessionState`, `nextAction` data
- `--priority`: include priority fields and sort sessions descending by priority score (implies `--with-next-action`)
- `--stale`: include metadata historical/stale counts (+ stale list in full JSON/text)
- `--prune-preview`: include safe stale-session cleanup plan (requires `--stale`)
- `--delta-json`: include `delta.added|removed|changed` since prior list cursor snapshot
- `--cursor-file PATH`: cursor snapshot file for `--delta-json` (default `/tmp/.lisa-<project_hash>-list-delta.json`)
- `--project-root`
- `--json`
- `--json-min`: minimal JSON (`sessions`, `count`)
- `--json-min` with `--stale` includes `historicalCount` + `staleCount`.

Behavior note:

- Default `session list` is current-socket scoped.
- `--all-sockets` expands discovery across metadata-known project roots and includes sessions currently active on those roots.
- `--delta-json` implies JSON output.
- `--delta-json` cannot be combined with `--stale`.
- `--active-only` cannot be combined with `--stale`.
- `--cursor-file` requires `--delta-json` and is written on each `--delta-json` run.

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
- `--delta` (emit added/removed topology edges since previous tree snapshot)
- `--flat` (machine-friendly parent/child rows)
- `--with-state` (attach status/sessionState snapshot to tree rows/nodes)
- `--json`
- `--json-min`: minimal JSON (`nodeCount` plus session graph rows/roots; with `--with-state`, emits rows)

Behavior note:

- `session tree` is metadata-first and can show historical sessions.
- Use `--active-only` (or pair with `session list`) for active-only topology.

### `session smoke`

Deterministic nested Lisa smoke test (`L1 -> ... -> LN`) with marker assertions.

```bash
lisa session smoke --levels 3
lisa session smoke --levels 4 --chaos mixed --json
lisa session smoke --levels 4 --json
lisa session smoke --levels 2 --chaos drop-marker --chaos-report --report-min --json
```

Flags:

- `--project-root`
- `--levels N` (1-4, default `3`)
- `--prompt-style STYLE` (`none|dot-slash|spawn|nested|neutral`, default `none`)
- `--matrix-file PATH`: prompt regression matrix (`mode|prompt`, mode = `bypass|full-auto|any`)
- `--chaos MODE`: fault mode (`none|delay|drop-marker|fail-child|mixed`, default `none`)
- `--chaos-report`: normalize chaos outcomes against expected-failure contracts
- `--contract-profile NAME`: command contract profile (`none|full`, default `none`)
- `--llm-profile NAME`: profile preset (`none|codex|claude|mixed`)
- `--model NAME`: optional Codex model pin for smoke spawn sessions
- `--poll-interval N` (default `1`)
- `--max-polls N` (default `180`)
- `--keep-sessions`
- `--report-min`: compact CI-focused JSON summary (errorCode/finalState/missing markers only)
- `--export-artifacts PATH`: export smoke artifacts bundle to path
- `--json`

Behavior:

- Creates nested interactive sessions using `session spawn/monitor/capture`.
- Asserts deterministic markers from every level in `L1` capture.
- Returns non-zero on any missing marker, spawn/monitor failure, or timeout.
- Optional `--prompt-style` runs a nested-wording probe (`session spawn --dry-run --detect-nested --json`) before smoke execution and records probe result in JSON summary.
- Optional `--matrix-file` runs a multi-prompt regression sweep before smoke execution and fails on expectation mismatch.
- `--contract-profile full` runs deterministic command-contract probes and includes failures in smoke summary.
- `--chaos` modes:
  - `none`: normal deterministic run
  - `delay`: injects per-level sleep delays
  - `drop-marker`: removes deepest-level done marker to force marker assertion failure
  - `fail-child`: exits one child session non-zero to force monitor failure
  - `mixed`: combines delay + deepest-level marker drop
- `--chaos-report` adds `chaosResult` payload with expected/observed failure matching and can convert expected chaos failures to passing exit status when contracts match.
- With `--report-min`, chaos payload still includes `chaosReport` and `chaosResult` when enabled.

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

### `session state-sandbox`

Manage objective/lane registry state snapshots for deterministic orchestration tests.

```bash
lisa session state-sandbox list --json
lisa session state-sandbox snapshot --file /tmp/lisa-state-sandbox.json --json
lisa session state-sandbox restore --file /tmp/lisa-state-sandbox.json --json
lisa session state-sandbox clear --json
```

Flags:

- action selector: positional `list|snapshot|restore|clear` or `--action`
- `--project-root`
- `--file`: required for `restore`, optional for `snapshot`
- `--json`
- `--json-min`

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
- `capabilities`
- `cleanup`
- `oauth add`
- `oauth list`
- `oauth remove`
- `agent build-cmd`
- `skills sync`
- `skills doctor`
- `skills install`
- `session name`
- `session spawn`
- `session detect-nested`
- `session send`
- `session turn`
- `session snapshot`
- `session status`
- `session explain`
- `session monitor`
- `session capture`
- `session contract-check`
- `session schema`
- `session checkpoint`
- `session dedupe`
- `session next`
- `session aggregate`
- `session prompt-lint`
- `session diff-pack`
- `session loop`
- `session context-cache`
- `session anomaly`
- `session budget-observe`
- `session budget-enforce`
- `session budget-plan`
- `session replay`
- `session handoff`
- `session packet`
- `session context-pack`
- `session route`
- `session autopilot`
- `session objective`
- `session memory`
- `session lane`
- `session state-sandbox`
- `session guard`
- `session tree`
- `session smoke`
- `session preflight`
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
CLAUDE_CODE_OAUTH_TOKEN (when --agent claude and oauth pool has entries)
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

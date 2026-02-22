# Lisa Commands

Repo-local validation should pin to `./lisa` only: `test -x ./lisa || exit 1; LISA_BIN=./lisa`.
Subcommands are separate argv tokens: `$LISA_BIN session spawn ...` (not `$LISA_BIN "session spawn" ...`).
For `--json` workflows, parse `stdout` as contract data and treat `stderr` as advisory/logging.
Use `--project-root` across session flows (and optionally `agent build-cmd`) for deterministic root context.

## session spawn

Create/start an agent session.

| Flag | Default | Description |
|---|---|---|
| `--agent` | `claude` | `claude` or `codex` |
| `--mode` | `interactive` | `interactive` or `exec` (aliases: `execution`, `non-interactive`) |
| `--nested-policy` | `auto` | Codex nested bypass policy: `auto`, `force`, `off` |
| `--nesting-intent` | `auto` | Nested intent override: `auto`, `nested`, `neutral` |
| `--prompt` | `""` | Initial prompt |
| `--project-root` | cwd | Project directory |
| `--session` | auto | Override name (must start with `lisa-`) |
| `--command` | `""` | Custom command (overrides agent CLI) |
| `--agent-args` | `""` | Extra args passed to agent CLI |
| `--model` | `""` | Codex model name (supported when `--agent codex`) |
| `--width` | `220` | tmux pane width |
| `--height` | `60` | tmux pane height |
| `--no-dangerously-skip-permissions` | false | Disable Claude default skip-permissions flag |
| `--cleanup-all-hashes` | false | Clean artifacts across all project hashes |
| `--dry-run` | false | Print plan only; do not create session/artifacts |
| `--detect-nested` | false | Include nested bypass decision diagnostics in JSON output |
| `--json` | false | JSON output |

JSON: `{"session","agent","mode","runId","projectRoot","command"}`

Spawn notes:
- `exec` requires `--prompt` unless `--command` is provided.
- If `--session` absent, Lisa auto-generates one.
- Codex `exec` defaults: `--full-auto --skip-git-repo-check`.
- Nested Codex hints (`./lisa`, `lisa session spawn`, `nested lisa`) auto-enable `--dangerously-bypass-approvals-and-sandbox` and omit `--full-auto`.
- Plain mentions like `Use lisa for child orchestration.` do not trigger bypass unless they include one of the explicit hint patterns above.
- Nested hint matching is case-insensitive (`./LISA` still matches `./lisa`).
- `--nested-policy force` enables nested bypass without prompt hints (omits `--full-auto`).
- `--nested-policy off` disables prompt-based nested bypass heuristics.
- `--nesting-intent nested|neutral` explicitly overrides prompt heuristics.
- If `--agent-args '--dangerously-bypass-approvals-and-sandbox'` is passed, Lisa omits `--full-auto` automatically.
- `--model <NAME>` injects Codex model selection without embedding `--model` inside `--agent-args`.
- Unknown model aliases can still run via fallback metadata, but may warn and degrade behavior; run `session preflight --agent codex --model <NAME> --json` first.
- Non-nested Codex `exec` with `--full-auto` can block child Lisa tmux sockets (`Operation not permitted`); prefer interactive + `session send` or explicit bypass args.
- For deeply nested prompts, prefer heredoc injection (`PROMPT=$(cat <<'EOF' ... EOF)` then `--prompt "$PROMPT"`).
- `--dry-run` returns resolved `command`, wrapped `startupCommand`, `socketPath`, and injected `env` keys.
- `--detect-nested --json` adds `nestedDetection` (`autoBypass`, `reason`, `matchedHint`, arg/full-auto signals, effective command flags).
- Observed `nestedDetection.reason` values include: `prompt_contains_dot_slash_lisa`, `prompt_contains_lisa_session_spawn`, `prompt_contains_nested_lisa`, `no_nested_hint`, `not_codex_exec`.
- Explicit policy/intent reasons also appear: `nested_policy_force`, `nested_policy_off`, `nesting_intent_nested`, `nesting_intent_neutral`.
- Doc/quoted mentions such as `The string './lisa' appears in docs only.` are treated as non-executable and do not auto-bypass.

## session detect-nested

Inspect nested codex-bypass detection without creating sessions.

| Flag | Default | Description |
|---|---|---|
| `--agent` | `codex` | `claude` or `codex` |
| `--mode` | `exec` | `interactive` or `exec` |
| `--nested-policy` | `auto` | Codex nested bypass policy: `auto`, `force`, `off` |
| `--nesting-intent` | `auto` | Nested intent override: `auto`, `nested`, `neutral` |
| `--prompt` | `""` | Prompt text to inspect |
| `--agent-args` | `""` | Existing agent args to inspect |
| `--model` | `""` | Codex model name (supported when `--agent codex`) |
| `--project-root` | cwd | Project directory |
| `--rewrite` | false | Include trigger-safe prompt rewrite suggestions |
| `--json` | false | JSON output |

JSON: `{"nestedPolicy","nestingIntent","nestedDetection","agentArgs","effectiveAgentArgs","rewrites?","command?"}`

## session send

| Flag | Default | Description |
|---|---|---|
| `--session` | required | Session name |
| `--project-root` | cwd | Project directory |
| `--text` | `""` | Text to send (exclusive with `--keys`) |
| `--keys` | `""` | tmux key tokens (exclusive with `--text`) |
| `--enter` | false | Press Enter after send |
| `--json-min` | false | Minimal JSON ack (`session`,`ok`) |
| `--json` | false | JSON output |

JSON: `{"session","ok","enter"}`

## session snapshot

One-shot `status + capture + nextOffset` poll helper.

| Flag | Default | Description |
|---|---|---|
| `--session` | required | Session name |
| `--project-root` | cwd | Project directory |
| `--agent` | `auto` | Agent hint |
| `--mode` | `auto` | Mode hint |
| `--lines` | `200` | Pane lines for raw capture |
| `--delta-from` | `""` | Delta start (`offset`, `@unix`, RFC3339) |
| `--markers` | `""` | Marker-only extraction mode (`A,B,C`) |
| `--keep-noise` | false | Keep Codex/MCP startup noise |
| `--strip-noise` | n/a | Compatibility alias for default filtering |
| `--fail-not-found` | false | Exit `1` when resolved status is `not_found` |
| `--json-min` | false | Minimal JSON output |
| `--json` | false | JSON output |

JSON: `{"session","status","sessionState","capture","nextOffset"}` (marker mode returns marker summary instead of `capture`)

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
| `--json-min` | false | Minimal JSON output (`session`,`status`,`sessionState`,`todosDone`,`todosTotal`,`waitEstimate`) |
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
| `--until-state` | `""` | Stop when session reaches a target state |
| `--expect` | `any` | `any`, `terminal`, `marker` (`marker` requires `--until-marker`) |
| `--json-min` | false | Minimal JSON output (`session`,`finalState`,`exitReason`,`polls`,`nextOffset?`) |
| `--stream-json` | false | Emit line-delimited JSON poll events before final payload |
| `--emit-handoff` | false | Emit compact handoff packets per poll (`--stream-json` required) |
| `--verbose` | false | Progress details to stderr |
| `--json` | false | JSON output |

JSON: `{"finalState","session","todosDone","todosTotal","outputFile","nextOffset","exitReason","polls","finalStatus"}`

Exit reasons:
`completed`, `crashed`, `not_found`, `stuck`, `waiting_input`, `waiting_input_turn_complete`, `marker_found`, `max_polls_exceeded`, `degraded_max_polls_exceeded`

Monitor nuance:
- Timeout returns `finalState:"timeout"`, `exitReason:"max_polls_exceeded"`, `finalStatus:"timeout"`.
- `marker_found` is success, often before terminal completion (`in_progress`/`active`).
- `marker_found` can occur on echoed prompt text; use unique markers that are excluded from prompt content.
- `--until-state` can stop on non-terminal states (for example `waiting_input` or `in_progress`) and returns that state as `exitReason`.
- `--waiting-requires-turn-complete true` can timeout whenever turn-complete cannot be inferred (common in Codex/non-transcript flows).
- `--stream-json` emits one JSON poll object per loop (`type:"poll"`), then emits the standard final monitor payload.
- `--emit-handoff` adds one `type:"handoff"` packet per poll with `reason`, `nextAction`, and optional `nextOffset`.
- Final monitor payload includes `nextOffset` when pane capture is available.
- `--emit-handoff` without `--stream-json` is a usage error (exit `1`).
- `--expect terminal` on marker/waiting success returns `expected_terminal_got_*` (exit `2`).
- `--expect marker` when marker is not first success returns `expected_marker_got_*` (exit `2`).
- `--expect marker` without `--until-marker` is a usage error (exit `1`).

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
| `--delta-from` | `""` | Delta start (`offset`, `@unix`, RFC3339); requires `--raw` |
| `--cursor-file` | `""` | Persist/reuse raw capture offsets across polling loops (`--raw` only) |
| `--markers` | `""` | Marker-only extraction mode (`A,B,C`) |
| `--summary` | false | Return bounded summary instead of full capture |
| `--token-budget` | `320` | Summary token budget |
| `--keep-noise` | false | Keep Codex/MCP startup noise |
| `--strip-noise` | n/a | Compatibility alias for default filtering |
| `--project-root` | cwd | Project directory |
| `--json-min` | false | Minimal JSON output for compact polling workflows |
| `--json` | false | JSON output |

Capture behavior:
- Claude default reads `~/.claude/projects/{encoded-path}/{sessionId}.jsonl` for structured messages.
- Falls back to raw pane capture if transcript unavailable.
- Promptless/custom-command Claude sessions lacking prompt+createdAt metadata intentionally fall back to raw.
- Raw capture filters MCP startup/auth noise by default; use `--keep-noise` to preserve full pane output (including MCP startup chatter/errors).
- Delta capture (`--delta-from`) supports:
  - offset mode: integer byte offset into current capture
  - timestamp mode: `@unix` or RFC3339; returns full capture only if output changed after timestamp
  - JSON includes `deltaMode` + `nextOffset` for iterative polling (fields appear when `--delta-from` is provided)
- Cursor files (`--cursor-file`) auto-load prior offset when `--delta-from` is omitted and write back `nextOffset` after capture.
- `--summary` returns compact `summary` payloads (plus `tokenBudget`/`truncated`) instead of raw `capture`.
- `--summary` cannot be combined with marker mode.

JSON:
- transcript: `{"session","claudeSession","messages":[{"role","text","timestamp"}]}`
- raw: `{"session","capture"}`
- transcript `--json-min`: `{"session","claudeSession","messageCount","capture"}`
- raw `--json-min`: `{"session","capture"}` (+`nextOffset` when `--delta-from` is set)
- marker mode: `{"session","markers","markerMatches","foundMarkers","missingMarkers"}` (plus `markerCounts` in non-min JSON)

## session explain

Detailed diagnostics with recent event timeline.

| Flag | Default | Description |
|---|---|---|
| `--session` | required | Session name |
| `--project-root` | cwd | Project directory |
| `--agent` | `auto` | Agent hint |
| `--mode` | `auto` | Mode hint |
| `--events` | `10` | Number of events to show |
| `--recent` | `0` | Alias for compact recent event count |
| `--json-min` | false | Minimal JSON output (`session`,`status`,`sessionState`,`reason`,`recent`) |
| `--json` | false | JSON output |

JSON: `{"status":{...},"eventFile","events":[...],"droppedEventLines"}`

## session handoff

Compact handoff packet for another orchestrator/agent loop.

Flags: `--session` (required), `--project-root`, `--agent`, `--mode`, `--events`, `--delta-from`, `--json`, `--json-min`.

JSON: `{"session","status","sessionState","reason","nextAction","nextOffset","summary","recent?","deltaFrom?","nextDeltaOffset?","deltaCount?"}`.

## session context-pack

Token-budgeted context packet with state + recent events + capture tail.

Flags: `--for` (alias `--session`, required), `--project-root`, `--agent`, `--mode`, `--events`, `--lines`, `--token-budget`, `--strategy`, `--json`, `--json-min`.

JSON: `{"session","sessionState","status","reason","nextAction","nextOffset","strategy","pack","tokenBudget","truncated"}`.

## session route

Recommend mode/policy/model defaults for orchestration goal.

Flags: `--goal` (`nested|analysis|exec`), `--agent`, `--prompt`, `--model`, `--project-root`, `--emit-runbook`, `--json`.

JSON includes command preview + routing rationale:
`{"goal","agent","mode","nestedPolicy","nestingIntent","model","command","monitorHint","nestedDetection","rationale","runbook?"}`.

## session guard

Shared tmux safety guard.

Flags: `--shared-tmux` (required), `--command`, `--project-root`, `--json`.

JSON: `{"sharedTmux","defaultSessionCount","defaultSessions","commandRisk","safe","warnings"}`.

## session list / exists / kill / kill-all / name

| Command | Key Flags | Output |
|---|---|---|
| `session list` | `--all-sockets`, `--project-only`, `--stale`, `--prune-preview`, `--project-root`, `--json`, `--json-min` | names (text) or JSON |
| `session exists` | `--session`, `--project-root`, `--json` | `true`/`false` (exit 0/1) or JSON |
| `session kill` | `--session`, `--project-root`, `--cleanup-all-hashes`, `--json` | `ok` or JSON (`found:false` + exit `1` when missing) |
| `session kill-all` | `--project-only`, `--project-root`, `--cleanup-all-hashes`, `--json` | `killed N sessions` or JSON |
| `session name` | `--agent`, `--mode`, `--project-root`, `--tag`, `--json` | name string or JSON |

Scope/retention:
- `session kill`/`kill-all` preserve event files for post-mortem.
- `session list` is socket-bound; pass explicit `--project-root` for deterministic scope.
- `session list --all-sockets` scans metadata-known project roots and returns active sessions only.
- `session list --stale` adds metadata stale analysis (`historicalCount`, `staleCount`, and stale list in full JSON/text).
- `session list --stale --prune-preview` adds safe stale-session cleanup plans (`prunePreview`).

## session tree

Inspect metadata parent/child links for nested orchestration.

| Flag | Default | Description |
|---|---|---|
| `--session` | `""` | Optional root-session filter |
| `--project-root` | cwd | Project directory |
| `--all-hashes` | false | Include metadata from all project hashes |
| `--active-only` | false | Include only sessions currently active in tmux |
| `--delta` | false | Emit added/removed topology edges since last tree snapshot |
| `--flat` | false | Machine-friendly parent/child rows |
| `--with-state` | false | Attach `status` + `sessionState` snapshots |
| `--json-min` | false | Minimal JSON output (`nodeCount` + session graph) |
| `--json` | false | JSON output |

JSON: `{"session","projectRoot","allHashes","nodeCount","roots":[{"session","parentSession","agent","mode","projectRoot","createdAt","children":[...]}]}`

Tree semantics:
- `session tree` is metadata-first and can show historical roots even when no active session exists.
- For active-only checks, use `--active-only` (or pair with `session list` / `session exists`).
- `--delta` persists a previous topology snapshot per project hash and reports added/removed edges.
- `--with-state` can emit low-token `rows` payloads with topology + current status in one call.

## session smoke

Deterministic nested smoke (`L1 -> ... -> LN`) with marker assertions.

Flags: `--project-root`, `--levels` (1-4, default `3`), `--prompt-style` (`none|dot-slash|spawn|nested|neutral`), `--matrix-file` (`mode|prompt` lines; mode=`bypass|full-auto|any`), `--model`, `--poll-interval` (default `1`), `--max-polls` (default `180`), `--keep-sessions`, `--report-min`, `--json`.

Behavior: uses nested `session spawn/monitor/capture`, asserts all level markers, non-zero exit on spawn/monitor/marker failure.
`--prompt-style` adds a pre-smoke dry-run probe that validates nested wording detection.
`--matrix-file` adds multi-prompt expectation regression before smoke execution and fails on mismatches.
`--model` pins model on smoke `session spawn` calls. Smoke validates Lisa orchestration plumbing, not model answer quality.
Prompt-style JSON probe fields are under `promptProbe.detection.*` (not `promptProbe.nestedDetection.*`).
`--report-min` emits compact CI-focused JSON (`ok`,`errorCode`,`finalState`,`missingMarkers`,`failedMatrix?`).

## session preflight

Validate environment + contract assumptions in one command.

Flags: `--project-root`, `--agent`, `--model`, `--auto-model`, `--auto-model-candidates`, `--json`.

Behavior:
- Runs doctor-equivalent environment checks (`tmux`, `claude`, `codex`).
- Validates parser/contract assumptions (mode aliases, monitor marker guard, capture delta parsing, nested codex hint routing).
- Optional model probe: `--agent codex --model <NAME>` performs real model-availability check.
- `--auto-model` probes candidate models and selects first supported (`gpt-5.3-codex,gpt-5-codex` by default).
- Exit `0` when both environment + contracts are ready; otherwise exit `1`.

## cleanup

Clean detached tmux servers and stale socket files from Lisa runs.

| Flag | Default | Description |
|---|---|---|
| `--dry-run` | false | Show removals/kills without mutating |
| `--include-tmux-default` | false | Also sweep `/tmp/tmux-*` default sockets |
| `--json` | false | JSON output |

JSON: `{"dryRun","scanned","removed","wouldRemove","killedServers","wouldKillServers","keptActive"}` plus optional `errors`.

Non-JSON output: one-line summary. Exit `1` if any probe/kill/remove errors occurred.
Safety: in shared tmux environments, run `session guard --shared-tmux --json` and `cleanup --dry-run` before any cleanup mutation.

## Other commands

| Command | Purpose |
|---|---|
| `doctor [--json]` | Check prerequisites (tmux + at least one of claude/codex). Exit 0=ok, 1=missing |
| `capabilities [--json]` | Emit command/flag capability matrix for orchestrator contract checks |
| `agent build-cmd` | Preview agent CLI command (`--agent`, `--mode`, `--nested-policy`, `--nesting-intent`, `--project-root`, `--prompt`, `--agent-args`, `--model`, `--no-dangerously-skip-permissions`, `--json`) |
| `skills sync` | Sync external skill into repo `skills/lisa` (`--json`: `{"source","destination","files","directories","symlinks"}`) |
| `skills doctor` | Verify installed Codex/Claude skill drift vs repo capability contract (`--deep` adds recursive content hash checks) |
| `skills install` | Install repo `skills/lisa` to `codex`, `claude`, or `project` (`--json`: `{"source","destination","files","directories","symlinks","noop?"}`; same source/destination now returns `noop:true`) |
| `version` | Print build version (`version`, `--version`, `-v`) |

## Modes

| Mode | Agent runs as | Use for |
|---|---|---|
| `interactive` | REPL (default) | Multi-turn tasks, follow-up prompts |
| `exec` | One-shot (`claude -p` / `codex exec --full-auto`) | Single prompt, auto-exits |

Aliases `execution` and `non-interactive` map to `exec`.

## JSON Surface

`--json` exists on: `doctor`, `capabilities`, `cleanup`, `agent build-cmd`, `skills sync|doctor|install`, `session name|spawn|detect-nested|send|snapshot|status|explain|monitor|capture|handoff|context-pack|route|guard|tree|smoke|preflight|list|exists|kill|kill-all`.

JSON error contract:
- command/runtime failures emit `{"ok":false,"errorCode":"...","error":"..."}` when `--json` is enabled.
- state/result payload failures also include `errorCode` on non-success paths.
- JSON payloads include `stderrPolicy` so callers can treat stderr as diagnostics channel.

`agent build-cmd --json` also returns `nestedDetection` for Codex nesting diagnostics.

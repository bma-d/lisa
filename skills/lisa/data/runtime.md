# Lisa Runtime Semantics

## Session States

Classifier is process-first:
`pane status -> done file -> agent PID -> interactive idle checks -> heartbeat -> pane command -> degraded/grace fallback`

| State | Meaning | Terminal? |
|---|---|---|
| `just_started` | Grace period (polls 1-3, idle) | No |
| `in_progress` | Agent PID alive, heartbeat fresh, or non-shell command active | No |
| `waiting_input` | Interactive idle after grace period (low CPU / known shell idle) | No* |
| `completed` | Pane exited 0 or done file exit 0 | Yes |
| `crashed` | Pane exited non-zero or done file non-zero | Yes |
| `stuck` | No active process/heartbeat after grace period | Yes |
| `degraded` | Lock/read/scan infrastructure error | No** |
| `not_found` | Session absent in tmux | Yes |

- `*` Monitor can stop on `waiting_input` when `--stop-on-waiting true`.
- Monitor can also stop on explicit `--until-state <STATE>` gates.
- `**` `degraded` is retried; prolonged degradation yields `degraded_max_polls_exceeded`.

## Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success: `completed`, `waiting_input`, `waiting_input_turn_complete`, `marker_found` |
| `1` | Command/runtime error (flags, tmux/IO, command failure) |
| `2` | Monitor non-success: `crashed`, `stuck`, `not_found`, `max_polls_exceeded`, `degraded_max_polls_exceeded`, `expected_*` |

## State Detection Priority (`computeSessionStatus`)

1. pane status: tmux `pane_dead` + `pane_dead_status` (`alive`, `exited:N`, `crashed:N`)
2. done file: `{runId}:{exitCode}` from wrapper `EXIT` trap
3. agent process: BFS from pane PID matching claude/codex
4. interactive idle checks:
   - known interactive + agent PID alive + low CPU => `waiting_input`
   - known interactive shell + fresh heartbeat + no active non-shell descendants (passive `sleep` ignored) => `waiting_input`
5. heartbeat freshness: wrapper touch loop every 2s; stale threshold default 8s
6. pane command fallback: shell vs non-shell activity
7. grace window: `just_started` for polls 1-3
8. fallback: scan/read error => `degraded`, else `stuck`

Output capture is for artifact retrieval, not state inference.

## Session Artifacts

Stored in `/tmp/`, keyed by project-root hash (first 8 chars of MD5 of canonical root):

```text
/tmp/.lisa-{hash}-session-{id}-meta.json        # metadata: agent, mode, runId, prompt, createdAt
/tmp/.lisa-{hash}-session-{id}-state.json       # poll cache: resolved hints, scan results
/tmp/.lisa-{hash}-session-{id}-done.txt         # completion marker: {runId}:{exitCode}
/tmp/.lisa-{hash}-session-{id}-heartbeat.txt    # liveness signal (mtime)
/tmp/.lisa-{hash}-session-{id}-events.jsonl     # event log (auto-trim: 1MB / 2000 lines)
/tmp/.lisa-{hash}-tree-delta.json               # previous tree topology snapshot for `session tree --delta`
/tmp/lisa-{hash}-output-{id}.txt                # terminal pane capture
/tmp/lisa-cmd-{hash}-{id}-{nanos}.sh            # temp script for long command payloads (>500 chars)
```

Lifecycle:
- `session kill` / `session kill-all` clean core artifacts but preserve events for post-mortem.
- stale event files prune after 14 days.

## Environment Overrides

| Variable | Default | Purpose |
|---|---|---|
| `LISA_CMD_TIMEOUT_SECONDS` | `20` | tmux/ps subprocess timeout |
| `LISA_STATE_LOCK_TIMEOUT_MS` | `2500` | State-file lock timeout |
| `LISA_EVENT_LOCK_TIMEOUT_MS` | `2500` | Event-file lock timeout |
| `LISA_PROCESS_LIST_CACHE_MS` | `500` | Raw `ps` cache TTL |
| `LISA_PROCESS_SCAN_INTERVAL_SECONDS` | `8` | Minimum process-scan interval |
| `LISA_HEARTBEAT_STALE_SECONDS` | `8` | Heartbeat stale threshold |
| `LISA_OUTPUT_STALE_SECONDS` | `240` | Output stale threshold |
| `LISA_EVENTS_MAX_BYTES` | `1000000` | Event file max bytes before trim |
| `LISA_EVENTS_MAX_LINES` | `2000` | Event file max lines before trim |
| `LISA_EVENT_RETENTION_DAYS` | `14` | Event-prune retention window |
| `LISA_AGENT_PROCESS_MATCH` | - | Custom process match (all agents) |
| `LISA_AGENT_PROCESS_MATCH_CLAUDE` | - | Custom process match (claude only) |
| `LISA_AGENT_PROCESS_MATCH_CODEX` | - | Custom process match (codex only) |
| `LISA_CLEANUP_ALL_HASHES` | `false` | Default cleanup across hash variants |
| `LISA_PROJECT_ROOT` | internal | Canonical project-root routing value |
| `LISA_TMUX_SOCKET` | internal (`/tmp/lisa-tmux-<slug>-<hash>.sock`) | tmux socket path used by Lisa runtime |
| `LISA_TMUX_SOCKET_DIR` | `""` (`/tmp` fallback) | Base directory used when Lisa computes per-project tmux socket path |

## Operational Notes

- Sessions are isolated per project via project-root MD5 hash.
- Session naming format: `lisa-{projectSlug}-{timestamp}-{agent}-{mode}[-{tag}]`.
- Custom `--session` values must start with `lisa-`.
- Long command payloads (>500 chars) are sent via temp script, not `send-keys`.
- `session send --text` uses tmux `load-buffer`/`paste-buffer` for safe multiline delivery.
- Spawn wrapper injects heartbeat loop + `EXIT` trap (done file + marker).
- Claude sessions default to `--dangerously-skip-permissions` unless disabled.
- Runtime sets tmux env vars: `LISA_SESSION`, `LISA_SESSION_NAME`, `LISA_AGENT`, `LISA_MODE`, `LISA_PROJECT_HASH`, `LISA_HEARTBEAT_FILE`, `LISA_DONE_FILE`.
- Raw pane capture filters MCP startup/auth noise by default; opt out with `--keep-noise`.
- Raw capture `--delta-from` supports offset/timestamp incremental fetch; JSON responses include `nextOffset` for polling loops.
- Raw capture `--cursor-file` persists/reuses offsets for resume-safe incremental polling.
- `session capture --markers` / `session snapshot --markers` return compact marker-only summaries for gating.
- `session monitor` final payload includes `nextOffset` when capture is available.
- `session handoff` and `session context-pack` provide compressed transfer payloads for multi-agent orchestration loops.
- `session exists` supports `--project-root` for cross-root checks.
- `session tree` reads metadata graph and may include historical roots; use `session list` for active-only views.
- `session tree --delta` reports added/removed edges compared to previous topology snapshot.
- `session list --stale` reports metadata historical/stale counts relative to active tmux sessions.
- Nested runs should always pass `--project-root`; use `./lisa` in executable prompt wording when repo-local binary is known to exist.
- Session commands recompute `LISA_TMUX_SOCKET` from project-root context; for custom socket placement, set `LISA_TMUX_SOCKET_DIR` (not `LISA_TMUX_SOCKET`) before invoking Lisa.
- Use `--nested-policy force|off` to avoid prompt-heuristic ambiguity in Codex exec nesting.
- Use `--nesting-intent nested|neutral` to explicitly override prompt heuristics.
- Quote/doc prompt mentions (`The string './lisa' appears in docs only.`) are treated as non-executable for nested bypass.
- `Use lisa inside of lisa inside as well.` is a known non-trigger phrase (`reason=no_nested_hint`).
- JSON payloads include `stderrPolicy` so orchestrators can classify stderr as diagnostics channel.
- In shared tmux environments, run `session guard --shared-tmux` before cleanup/kill-all; then run `cleanup --dry-run` first.

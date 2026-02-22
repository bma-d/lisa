# SDK Usage Guide

Last Updated: 2026-02-22
Related Files: `USAGE.md`, `agent.md`, `src/commands_session.go`, `src/commands_agent.go`

## Overview

How to use Lisa as infrastructure from an LLM orchestrator or script.

## Integration Pattern

1. **Spawn** one session per task (`session spawn --json`), store returned session name (custom `--session` values must start with `lisa-`)
2. **Poll** with `session monitor --json` (blocking loop) or `session status --json` (one-shot)
   - In `session status` output, `sessionState` is lifecycle truth; `status` is normalized for terminal states (`completed`, `crashed`, `stuck`, `not_found`) to avoid idle-vs-terminal mismatches in JSON/CSV parsing.
   - In `session monitor` output, `finalState` is stop-state truth; `finalStatus` is normalized for terminal states (`completed`, `crashed`, `stuck`, `not_found`) and uses `timeout` for timeout exits.
   - For stricter interactive stop semantics, use `session monitor --stop-on-waiting true --waiting-requires-turn-complete true` to stop only after transcript turn completion is detected
   - For deterministic completion gates, use `session monitor --until-marker "<TOKEN>"` and stop on `exitReason=marker_found` (exit `0`)
   - To avoid ambiguous success semantics, add `session monitor --expect terminal` (or `--expect marker`) so monitor fails fast when a different success condition is hit first
   - For low-token one-shot snapshots, use `session status --json-min`
   - For low-token continuous polling, use `session monitor --json-min --stream-json` (line-delimited poll updates + final result)
   - For periodic compact relay packets during polling, add `session monitor --stream-json --emit-handoff --json-min`
3. If state is `stuck`, send next instruction with `session send --text "..." --enter`; if state is `degraded`, retry polling and inspect `signals.*Error`
4. Fetch artifacts with `session capture --lines N`
   - Raw capture now suppresses known Codex/MCP startup noise by default (including MCP OAuth refresh/auth-failure startup noise); use `session capture --raw --keep-noise` to keep full raw output
   - For incremental polling, use `session capture --raw --delta-from <offset|@unix|rfc3339> --json` and reuse returned `nextOffset`
   - For compact polling payloads, use `session capture --json-min` (or `--raw --delta-from ... --json-min`)
   - For bounded summaries, use `session capture --summary --token-budget <N> --json`
   - Claude transcript capture now requires session metadata to include prompt + createdAt; promptless/custom-command sessions automatically fall back to raw pane capture
5. Kill and clean up with `session kill --session NAME`

Nested Codex note: `codex exec --full-auto` runs sandboxed and can block tmux socket creation for child Lisa sessions. For deep nested orchestration (L1->L2->L3), prefer interactive sessions (`--mode interactive` + `session send`) or bypass mode.
Lisa now auto-enables Codex bypass (`--dangerously-bypass-approvals-and-sandbox`, no `--full-auto`) when exec prompts suggest nesting (`./lisa`, `lisa session spawn`, `nested lisa`).
You can set `--nested-policy force|off` to bypass prompt heuristics explicitly.
Use `--model <NAME>` on `session spawn` or `agent build-cmd` when `--agent codex` to inject Codex model selection without packing it into `--agent-args` (example: `--model gpt-5.3-codex-spark`).
You can still pass `--agent-args '--dangerously-bypass-approvals-and-sandbox'` explicitly; Lisa omits `--full-auto` automatically because Codex rejects combining both flags.
For deeply nested prompt chains, prefer heredoc prompt injection (`PROMPT=$(cat <<'EOF' ... EOF)` then `--prompt "$PROMPT"`) instead of highly escaped inline single-quoted chains.

Manual nested smoke command: run `./smoke-nested` from repo root to validate L1->L2->L3 interactive nesting end-to-end with deterministic markers.
Built-in nested smoke command: run `./lisa session smoke --levels 4 --json` for deterministic marker validation and JSON summary.
To validate nested wording detectors before smoke execution, use `./lisa session smoke --prompt-style dot-slash|spawn|nested|neutral --json`.

`session exists` now also accepts `--project-root` for explicit socket/project routing.
`session list --all-sockets` now discovers active sessions across metadata-known project roots/sockets in one call.
`session spawn --dry-run --json` now emits resolved command/socket/env planning output without creating tmux sessions/artifacts.
`session spawn --detect-nested --json` now emits `nestedDetection` diagnostics explaining bypass/full-auto decisions.
`session tree --json` now returns metadata parent/child hierarchy for nested orchestration introspection.
`session tree --active-only` now filters to sessions currently active in tmux.
`session tree --flat` now emits low-token machine-friendly parent/child rows.
`session tree --json-min` now emits low-token machine-readable graph payloads.
`session tree --with-state --json-min` now emits topology rows with current `status` + `sessionState` in one call.
`session list --json-min` now emits low-token machine-readable list payloads.
`session list --stale --prune-preview --json-min` now emits stale cleanup planning payloads (`pruneCmd`, `metaPath`, `projectRoot`).
`session monitor --json-min` now emits low-token machine-readable fields (`session`,`finalState`,`exitReason`,`polls`).
`session monitor --stream-json` now emits line-delimited poll events before the final monitor payload.
`session monitor --until-jsonpath '$.path=value'` now supports JSON-structured stop gates.
`session handoff --delta-from <N>` now emits incremental event packets with `nextDeltaOffset`.
`session handoff --cursor-file /tmp/handoff.cursor` now persists/reuses incremental handoff offsets.
`session context-pack --strategy terse|balanced|full` now applies deterministic packing defaults for token-sensitive routing.
`session context-pack --from-handoff <path|->` now repacks handoff JSON without live polling.
`session route --emit-runbook --json` now emits executable step plans (preflight/spawn/monitor/capture/handoff/cleanup).
`session route --budget <N>` now propagates token-budget hints into runbook capture/context-pack steps.
`session autopilot --json` now runs spawn->monitor->capture->handoff->optional cleanup in one command.
`session detect-nested --rewrite --json` now emits trigger-safe prompt rewrites for non-trigger wording.
`session smoke --report-min --json` now emits compact CI-oriented smoke payloads.
`session smoke --chaos delay|drop-marker|fail-child|mixed --json` now supports deterministic fault-injection probes.
`session guard --enforce` now hard-fails medium/high shared-tmux risk plans with remediation hints.
`session list --active-only --with-next-action --json-min` now emits triage-ready queue payloads.
`session capture --summary-style terse|ops|debug` now supports role-specific summary shaping.
`skills doctor --explain-drift --json` now includes remediation hints per target.
`session capture --json-min` now emits compact JSON payloads for transcript/raw capture paths.
`session preflight --json` now validates environment + parser/contract assumptions in one call.
JSON outputs now include `stderrPolicy` so orchestrators can classify stderr as diagnostics channel.
Session manage/name helpers now support `--json` (`session name|list|exists|kill|kill-all`).
JSON failure paths now include machine-readable `errorCode` fields across JSON-enabled commands.

## Command Contract Source

All CLI usage details live in `USAGE.md`:

- command syntax and flags
- state definitions
- exit-code contract
- JSON vs text output modes
- environment variable controls

## Related Context

- @AGENTS.md
- @src/AGENTS.md

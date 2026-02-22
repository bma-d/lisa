# Lisa Validation Snapshot

Validated: 2026-02-22

## Guardrails

- `session list` is source of truth for active sessions.
- `session tree` is metadata graph; can include historical roots. Use `session tree --active-only` for active-only topology.
- `session monitor --expect marker` requires `--until-marker`; missing marker target is usage error (exit `1`).
- `session monitor --until-jsonpath '$.sessionState=waiting_input'` can terminate before marker/state gates and returns `exitReason:"jsonpath_matched"`.
- `session monitor` exits `0` when `--until-state` or `--until-jsonpath` matches, even if the matched state is non-terminal.
- `session kill --json` missing session exits `1` with JSON payload including `found:false` (no human stderr line).
- `--waiting-requires-turn-complete true` may timeout (`max_polls_exceeded`) when turn-complete inference is unavailable.
- Timeout monitor payloads use `finalState:"timeout"` and `finalStatus:"timeout"`.
- `session status` returns exit `0` on `not_found` unless `--fail-not-found` is set.
- Low-token polling path: `session monitor --json-min` (optional `--stream-json`) and `session snapshot --json-min`.
- Low-token incremental capture path: `session capture --raw --delta-from <offset|@unix|rfc3339> --json-min`; reuse returned `nextOffset`.
- Cursor-file polling path: `session capture --raw --cursor-file /tmp/lisa.cursor --json-min`.
- State-gated polling path: `session monitor --until-state waiting_input|completed|crashed --json`.
- `session monitor` final payload includes `nextOffset` when capture is available.
- `session monitor --stream-json --emit-handoff` emits `type=handoff` packets per poll.
- `session monitor --stream-json --emit-handoff --event-budget <N>` emits bounded incremental handoff deltas tuned by token budget.
- `session monitor --handoff-cursor-file <PATH>` emits incremental handoff deltas and persists `nextDeltaOffset`.
- `session handoff --json-min` and `session context-pack --json-min` provide compact transfer payloads for multi-agent loops.
- `session handoff --delta-from <N>` returns incremental `recent` events + `nextDeltaOffset`.
- `session handoff --cursor-file /tmp/handoff.cursor` persists/reuses `nextDeltaOffset` across loops.
- `session packet --json-min` provides one-call status + summary + recent handoff items.
- `session packet --fields <csv>` projects payload to selected JSON fields for low-token transport.
- `session packet --cursor-file <PATH>` persists/reuses handoff event delta offsets.
- `session context-pack --strategy terse|balanced|full` applies deterministic default budgets.
- `session context-pack --redact <rules>` applies built-in payload redaction (`none|all|paths|emails|secrets|numbers|tokens`).
- `session context-pack --from-handoff <path|->` builds pack without live status polling.
- Nested diagnostics path: `session spawn --dry-run --detect-nested --json` or `session detect-nested --json`.
- `session detect-nested --rewrite` emits trigger-safe prompt rewrites.
- `session detect-nested --why` emits hint-span reasoning payload (`why.spans`).
- Deterministic nested override: `--nested-policy auto|force|off` and `--nesting-intent auto|nested|neutral`.
- Prompt-style smoke probes expose detection fields at `promptProbe.detection.*`.
- Quote/doc mentions like `The string "./lisa" appears in docs only.` are treated as non-executable nested hints.
- Codex model pinning: `--model <NAME>` on `session spawn` / `agent build-cmd`; verify support with `session preflight --agent codex --model <NAME> --json`.
- Model ids are case-sensitive in practice (`gpt-5.3-codex-spark` succeeded; `GPT-5.3-Codex-Spark` failed preflight).
- Model preflight probe can fail (`errorCode:"preflight_model_not_supported"`) for unknown aliases; treat this as capability signal, not parser failure.
- `session preflight --auto-model --json` selects first supported candidate model (`gpt-5.3-codex`, then `gpt-5-codex` by default).
- `session smoke --report-min --json` emits compact CI payloads.
- `session list --stale --prune-preview` emits safe stale cleanup commands.
- `session tree --with-state --json-min` emits rows with topology + status/sessionState.
- `session capture --summary --token-budget N` returns bounded summary payloads.
- `session capture --summary-style ops|debug` emits role-specific summary bodies.
- `session route --budget N --emit-runbook` propagates token-budget hints into capture/context-pack steps.
- `session route --topology planner,workers,reviewer` emits multi-agent topology graph payload.
- `session route --cost-estimate` emits per-step token/time prediction payload.
- `session route --from-state <PATH|->` routes from handoff/status JSON and emits `fromState` in payload.
- `session list --active-only --with-next-action --json-min` returns filtered sessions plus per-session next actions.
- `session list --cursor-file <PATH>` is meaningful only with `--delta-json` (cursor snapshot read/write).
- `session list --delta-json --cursor-file <PATH>` returns added/removed/changed session deltas with persisted cursor snapshots.
- `session guard --shared-tmux --enforce --command ...` returns `errorCode:"shared_tmux_guard_enforced"` on medium/high risk.
- `session guard --shared-tmux --advice-only --command ...` preserves diagnostics while always exiting `0`.
- `session smoke --chaos delay|drop-marker|fail-child|mixed --json` emits deterministic chaos metadata/results.
- `session smoke` terminal gating uses `--stop-on-waiting false` during root monitor to avoid waiting-input expectation mismatch in delayed chaos paths.
- `session smoke --chaos-report` normalizes expected-failure modes (`drop-marker|fail-child|mixed`) to pass when observed error code matches contract.
- `session autopilot --json` emits step-by-step orchestration payload with per-step exit statuses.
- `session autopilot --resume-from <PATH|->` resumes from first failed step (`resumedFrom`,`resumeStep`); `-` reads JSON from stdin.
- `session autopilot --resume-from` preserves resumed `goal/mode` when caller does not override flags.
- `session preflight --fast --json` runs reduced high-risk contract checks (`contract_count` lower than full mode).
- `skills doctor --explain-drift --json` includes remediation hints per target.
- `session schema --command <name> --json` emits JSON schema contract payloads for machine parsing.
- `session checkpoint save|resume --file <PATH>` saves/resumes atomic orchestration bundles.
- `session dedupe --task-hash <HASH>` enforces duplicate-task claims across active sessions.
- `session list --priority --json-min` emits priority score/label fields and sorted queue.
- `session capture --semantic-delta --cursor-file <PATH>` emits semantic deltas with persistent baseline.

## Observed Behaviors

- `session monitor --until-marker` succeeds with `exitReason:"marker_found"` and may still report state `in_progress`.
- `session monitor --until-marker` can match marker text echoed from prompt input; keep markers unique and out of prompt text.
- `session exists` prints `false` and exits `1` when session is absent.
- Nested wording detection is case-insensitive (`./LISA` matches `./lisa` hint).
- `Use lisa inside of lisa inside as well.` returns `nestedDetection.reason:"no_nested_hint"` (does not trigger bypass).
- `session smoke --levels 1..4 --json` passed in this repository with marker assertions.

## Fast Confidence Loop

```bash
ROOT="$(pwd)"
test -x ./lisa || { echo "missing ./lisa"; exit 1; }
LISA_BIN=./lisa

$LISA_BIN session preflight --json
MODEL="${MODEL:-gpt-5-codex}"
$LISA_BIN session preflight --agent codex --auto-model --json || \
  echo "auto-model probe failed; continue with explicit --model"
$LISA_BIN session detect-nested --prompt "Use ./lisa for child orchestration." --json

for p in \
  "Use ./lisa for child workers" \
  "Run lisa session spawn for child workers" \
  "Create nested lisa inside lisa inside lisa and report" \
  "Use lisa inside of lisa inside as well." \
  "Run ./LISA for children" \
  "No nesting requested here."
do
  $LISA_BIN session spawn --agent codex --mode exec --project-root "$ROOT" \
    --prompt "$p" --dry-run --detect-nested --json | jq '{command,nestedDetection}'
done

$LISA_BIN session smoke --project-root "$ROOT" --levels 4 --json
$LISA_BIN session smoke --project-root "$ROOT" --levels 4 --model "$MODEL" --json
$LISA_BIN session smoke --project-root "$ROOT" --matrix-file ./nested-matrix.txt --json
```

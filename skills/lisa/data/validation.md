# Lisa Validation Snapshot

Validated: 2026-02-21

## Guardrails

- `session list` is source of truth for active sessions.
- `session tree` is metadata graph; can include historical roots. Use `session tree --active-only` for active-only topology.
- `session monitor --expect marker` requires `--until-marker`; missing marker target is usage error (exit `1`).
- `session monitor --until-jsonpath '$.sessionState=waiting_input'` can terminate before marker/state gates and returns `exitReason:"jsonpath_matched"`.
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
- `session handoff --json-min` and `session context-pack --json-min` provide compact transfer payloads for multi-agent loops.
- `session handoff --delta-from <N>` returns incremental `recent` events + `nextDeltaOffset`.
- `session handoff --cursor-file /tmp/handoff.cursor` persists/reuses `nextDeltaOffset` across loops.
- `session context-pack --strategy terse|balanced|full` applies deterministic default budgets.
- `session context-pack --from-handoff <path|->` builds pack without live status polling.
- Nested diagnostics path: `session spawn --dry-run --detect-nested --json` or `session detect-nested --json`.
- `session detect-nested --rewrite` emits trigger-safe prompt rewrites.
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
- `session list --active-only --with-next-action --json-min` returns filtered sessions plus per-session next actions.
- `session guard --shared-tmux --enforce --command ...` returns `errorCode:"shared_tmux_guard_enforced"` on medium/high risk.
- `session smoke --chaos delay|drop-marker|fail-child|mixed --json` emits deterministic chaos metadata/results.
- `session autopilot --json` emits step-by-step orchestration payload with per-step exit statuses.
- `skills doctor --explain-drift --json` includes remediation hints per target.

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

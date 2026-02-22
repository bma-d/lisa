# Lisa Validation Snapshot

Validated: 2026-02-21

## Guardrails

- `session list` is source of truth for active sessions.
- `session tree` is metadata graph; can include historical roots. Use `session tree --active-only` for active-only topology.
- `session monitor --expect marker` requires `--until-marker`; missing marker target is usage error (exit `1`).
- `session kill --json` missing session exits `1` with JSON payload including `found:false` (no human stderr line).
- `--waiting-requires-turn-complete true` may timeout (`max_polls_exceeded`) when turn-complete inference is unavailable.
- Timeout monitor payloads use `finalState:"timeout"` and `finalStatus:"timeout"`.
- `session status` returns exit `0` on `not_found` unless `--fail-not-found` is set.
- Low-token polling path: `session monitor --json-min` (optional `--stream-json`) and `session snapshot --json-min`.
- Low-token incremental capture path: `session capture --raw --delta-from <offset|@unix|rfc3339> --json-min`; reuse returned `nextOffset`.
- `session monitor` final payload includes `nextOffset` when capture is available.
- Nested diagnostics path: `session spawn --dry-run --detect-nested --json` or `session detect-nested --json`.
- Deterministic nested override: `--nested-policy auto|force|off` and `--nesting-intent auto|nested|neutral`.
- Quote/doc mentions like `The string "./lisa" appears in docs only.` are treated as non-executable nested hints.
- Codex model pinning: `--model <NAME>` on `session spawn` / `agent build-cmd`; verify support with `session preflight --agent codex --model <NAME> --json`.
- Model preflight probe can fail (`errorCode:"preflight_model_not_supported"`) for unknown aliases; treat this as capability signal, not parser failure.

## Observed Behaviors

- `session monitor --until-marker` succeeds with `exitReason:"marker_found"` and may still report state `in_progress`.
- `session monitor --until-marker` can match marker text echoed from prompt input; keep markers unique and out of prompt text.
- `session exists` prints `false` and exits `1` when session is absent.
- Nested wording detection is case-insensitive (`./LISA` matches `./lisa` hint).
- `session smoke --levels 1..4 --json` passed in this repository with marker assertions.

## Fast Confidence Loop

```bash
ROOT="$(pwd)"
test -x ./lisa || { echo "missing ./lisa"; exit 1; }
LISA_BIN=./lisa

$LISA_BIN session preflight --json
MODEL="${MODEL:-gpt-5-codex}"
$LISA_BIN session preflight --agent codex --model "$MODEL" --json || \
  echo "model probe failed; continue without --model or pick supported model"
$LISA_BIN session detect-nested --prompt "Use ./lisa for child orchestration." --json

for p in \
  "Use ./lisa for child workers" \
  "Run lisa session spawn for child workers" \
  "Create nested lisa inside lisa inside lisa and report" \
  "Run ./LISA for children" \
  "No nesting requested here."
do
  $LISA_BIN session spawn --agent codex --mode exec --project-root "$ROOT" \
    --prompt "$p" --dry-run --detect-nested --json | jq '{command,nestedDetection}'
done

$LISA_BIN session smoke --project-root "$ROOT" --levels 4 --json
$LISA_BIN session smoke --project-root "$ROOT" --matrix-file ./nested-matrix.txt --json
```

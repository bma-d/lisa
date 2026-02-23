#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-$(pwd)}"
ROOT="$(cd "$ROOT" && pwd)"
BIN="${LISA_BIN:-$ROOT/lisa}"
if [[ ! -x "$BIN" ]]; then
  echo "missing executable: $BIN" >&2
  exit 1
fi

SOCKET_DIR_DEFAULT="/tmp/lisa-tmux-contract-$USER-$$"
export LISA_TMUX_SOCKET_DIR="${LISA_TMUX_SOCKET_DIR:-$SOCKET_DIR_DEFAULT}"
mkdir -p "$LISA_TMUX_SOCKET_DIR"

TMP_DIR="$(mktemp -d /tmp/lisa-contract-matrix.XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass=0
fail=0

run_case() {
  local name="$1"
  local expected_exit="$2"
  local expr="$3"
  shift 3

  local out rc
  set +e
  out="$($BIN "$@" 2>/dev/null)"
  rc=$?
  set -e

  if [[ "$rc" != "$expected_exit" ]]; then
    echo "FAIL [$name] exit=$rc expected=$expected_exit"
    fail=$((fail+1))
    return
  fi

  if ! python3 - "$expr" "$out" <<'PY'
import json
import sys

expr = sys.argv[1]
raw = sys.argv[2].strip()
if not raw:
    raise SystemExit(1)
data = json.loads(raw)
allowed = {"data": data, "len": len, "str": str, "int": int, "float": float, "bool": bool, "any": any, "all": all, "isinstance": isinstance, "dict": dict}
ok = bool(eval(expr, {"__builtins__": {}}, allowed))
if not ok:
    raise SystemExit(1)
PY
  then
    echo "FAIL [$name] json assertion"
    fail=$((fail+1))
    return
  fi

  echo "PASS [$name]"
  pass=$((pass+1))
}

POLICY_FILE="$TMP_DIR/policy.json"
cat > "$POLICY_FILE" <<'JSON'
{
  "machinePolicy": "strict",
  "requireProjectRoot": true,
  "requireProjectOnlyForKillAll": true,
  "allowCleanupIncludeTmuxDefault": false,
  "deniedCommands": ["cleanup --include-tmux-default"]
}
JSON

MATRIX_FILE="$TMP_DIR/matrix.txt"
cat > "$MATRIX_FILE" <<'MATRIX'
bypass|Use ./lisa for child orchestration.
full-auto|No nesting requested here.
MATRIX

run_case "capabilities-core" 0 "any(c.get('name') == 'session objective' for c in data.get('commands', [])) and any(c.get('name') == 'session budget-plan' for c in data.get('commands', []))" capabilities --json

run_case "objective-upsert" 0 "data.get('action') == 'upserted' and data.get('currentId') == 'contract-main'" session objective --project-root "$ROOT" --id contract-main --goal "Contract validation objective" --acceptance "All matrix checks pass" --budget 420 --activate --json

run_case "lane-upsert" 0 "data.get('action') == 'upserted' and data.get('name') == 'planner'" session lane --project-root "$ROOT" --name planner --goal analysis --agent codex --mode interactive --nested-policy off --nesting-intent neutral --contract "handoff_v2_required" --budget 420 --json

run_case "spawn-lane-objective-dryrun" 0 "bool(data.get('dryRun')) and data.get('lane') == 'planner' and isinstance(data.get('objective'), dict)" session spawn --project-root "$ROOT" --lane planner --agent codex --mode interactive --prompt "No nesting requested here." --dry-run --detect-nested --json

run_case "detect-nested-trigger" 0 "bool(data.get('nestedDetection', {}).get('autoBypass'))" session detect-nested --agent codex --mode exec --prompt "Use ./lisa for child orchestration." --json

run_case "detect-nested-nontrigger" 0 "(not bool(data.get('nestedDetection', {}).get('autoBypass'))) and data.get('nestedDetection', {}).get('reason') == 'no_nested_hint'" session detect-nested --agent codex --mode exec --prompt "Use lisa inside of lisa inside as well." --json

run_case "route-queue" 0 "'queue' in data and 'queueCount' in data" session route --goal analysis --project-root "$ROOT" --queue --json

run_case "budget-plan" 0 "isinstance(data.get('hardStop'), dict) and 'enforceCommand' in data.get('hardStop', {})" session budget-plan --goal analysis --project-root "$ROOT" --topology planner,workers --budget 500 --json

run_case "guard-policy" 1 "str(data.get('errorCode', '')) in ('shared_tmux_guard_enforced', 'shared_tmux_risk_detected') and bool(data.get('policyFile'))" session guard --shared-tmux --enforce --policy-file "$POLICY_FILE" --command "./lisa cleanup --include-tmux-default" --project-root "$ROOT" --json

run_case "smoke-llm-profile" 0 "'ok' in data and str(data.get('llmProfile')) == 'mixed'" session smoke --project-root "$ROOT" --levels 1 --llm-profile mixed --matrix-file "$MATRIX_FILE" --report-min --json

echo ""
echo "Contract matrix summary: pass=$pass fail=$fail"
if [[ "$fail" -gt 0 ]]; then
  exit 1
fi

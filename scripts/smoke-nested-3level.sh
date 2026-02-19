#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: ./scripts/smoke-nested-3level.sh [--project-root PATH] [--max-polls N] [--keep-sessions]

Runs a deterministic 3-level nested Lisa smoke test in interactive mode:
L1 -> L2 -> L3, each level uses `./lisa session spawn/monitor/capture`.
USAGE
}

PROJECT_ROOT=""
MAX_POLLS=180
KEEP_SESSIONS=0

while (($# > 0)); do
  case "$1" in
    --project-root)
      if (($# < 2)); then
        echo "error: --project-root requires a value" >&2
        exit 2
      fi
      PROJECT_ROOT="$2"
      shift 2
      ;;
    --max-polls)
      if (($# < 2)); then
        echo "error: --max-polls requires a value" >&2
        exit 2
      fi
      MAX_POLLS="$2"
      shift 2
      ;;
    --keep-sessions)
      KEEP_SESSIONS=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown arg: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$PROJECT_ROOT" ]]; then
  PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fi

BIN="$PROJECT_ROOT/lisa"
if [[ ! -x "$BIN" ]]; then
  echo "lisa binary missing at $BIN; building..." >&2
  (cd "$PROJECT_ROOT" && go build -o lisa .)
fi

for cmd in tmux bash; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found: $cmd" >&2
    exit 1
  fi
done

RUN_ID="$(date +%Y%m%d_%H%M%S)_$$"
WORKDIR="/tmp/lisa-smoke-nested-$RUN_ID"
mkdir -p "$WORKDIR"

SESSION_L1="lisa-smoke-l1-$RUN_ID"
SESSION_L2="lisa-smoke-l2-$RUN_ID"
SESSION_L3="lisa-smoke-l3-$RUN_ID"

L3_SCRIPT="$WORKDIR/l3.sh"
L2_SCRIPT="$WORKDIR/l2.sh"
L1_SCRIPT="$WORKDIR/l1.sh"

cat > "$L3_SCRIPT" <<L3
#!/usr/bin/env bash
set -euo pipefail
echo NESTED_L3_DONE=1
echo NESTED_L3_SESSION=$SESSION_L3
L3

cat > "$L2_SCRIPT" <<L2F
#!/usr/bin/env bash
set -euo pipefail
BIN="$BIN"
ROOT="$PROJECT_ROOT"
"\$BIN" session spawn --agent codex --mode interactive --project-root "\$ROOT" --session "$SESSION_L3" --command "/bin/bash $L3_SCRIPT" --json
"\$BIN" session monitor --session "$SESSION_L3" --project-root "\$ROOT" --poll-interval 1 --max-polls $MAX_POLLS --json
"\$BIN" session capture --session "$SESSION_L3" --project-root "\$ROOT" --raw --lines 120

echo NESTED_L2_DONE=1
echo NESTED_L2_SESSION=$SESSION_L2
L2F

cat > "$L1_SCRIPT" <<L1F
#!/usr/bin/env bash
set -euo pipefail
BIN="$BIN"
ROOT="$PROJECT_ROOT"
"\$BIN" session spawn --agent codex --mode interactive --project-root "\$ROOT" --session "$SESSION_L2" --command "/bin/bash $L2_SCRIPT" --json
"\$BIN" session monitor --session "$SESSION_L2" --project-root "\$ROOT" --poll-interval 1 --max-polls $MAX_POLLS --json
"\$BIN" session capture --session "$SESSION_L2" --project-root "\$ROOT" --raw --lines 220

echo NESTED_L1_DONE=1
echo NESTED_L1_SESSION=$SESSION_L1
L1F

chmod +x "$L3_SCRIPT" "$L2_SCRIPT" "$L1_SCRIPT"

cleanup() {
  if (( KEEP_SESSIONS == 0 )); then
    "$BIN" session kill --session "$SESSION_L3" --project-root "$PROJECT_ROOT" >/dev/null 2>&1 || true
    "$BIN" session kill --session "$SESSION_L2" --project-root "$PROJECT_ROOT" >/dev/null 2>&1 || true
    "$BIN" session kill --session "$SESSION_L1" --project-root "$PROJECT_ROOT" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

MONITOR_JSON_FILE="$WORKDIR/monitor-l1.json"
CAPTURE_RAW_FILE="$WORKDIR/capture-l1.txt"

"$BIN" session spawn --agent codex --mode interactive --project-root "$PROJECT_ROOT" --session "$SESSION_L1" --command "/bin/bash $L1_SCRIPT" --json >"$WORKDIR/spawn-l1.json"

if ! "$BIN" session monitor --session "$SESSION_L1" --project-root "$PROJECT_ROOT" --poll-interval 1 --max-polls "$MAX_POLLS" --json >"$MONITOR_JSON_FILE"; then
  echo "FAIL: L1 monitor returned non-zero" >&2
  "$BIN" session explain --session "$SESSION_L1" --project-root "$PROJECT_ROOT" --json >"$WORKDIR/explain-l1.json" || true
  echo "Artifacts: $WORKDIR" >&2
  exit 1
fi

if ! grep -q '"finalState":"completed"' "$MONITOR_JSON_FILE"; then
  echo "FAIL: expected L1 finalState completed" >&2
  cat "$MONITOR_JSON_FILE" >&2
  "$BIN" session explain --session "$SESSION_L1" --project-root "$PROJECT_ROOT" --json >"$WORKDIR/explain-l1.json" || true
  echo "Artifacts: $WORKDIR" >&2
  exit 1
fi

"$BIN" session capture --session "$SESSION_L1" --project-root "$PROJECT_ROOT" --raw --lines 360 >"$CAPTURE_RAW_FILE"

for marker in NESTED_L1_DONE=1 NESTED_L2_DONE=1 NESTED_L3_DONE=1; do
  if ! grep -q "$marker" "$CAPTURE_RAW_FILE"; then
    echo "FAIL: missing marker: $marker" >&2
    echo "Artifacts: $WORKDIR" >&2
    exit 1
  fi
done

echo "PASS: 3-level nested interactive Lisa smoke"
echo "L1=$SESSION_L1"
echo "L2=$SESSION_L2"
echo "L3=$SESSION_L3"
echo "Artifacts: $WORKDIR"

#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: ./scripts/deep-stress.sh [options]

Deep stress harness for Lisa command contracts and runtime behavior.

Options:
  --project-root PATH   Repo root containing ./lisa (default: script parent dir)
  --out-dir PATH        Output dir for logs/artifacts (default: /tmp/lisa-deep-stress-<run_id>)
  --socket-dir PATH     Isolated tmux socket dir (default: /tmp/lisa-tmux-deep-stress-<run_id>)
  --model NAME          Codex model alias/name (default: codex-spark)
  --poll-interval N     Poll interval seconds (default: 1)
  --max-polls N         Max polls for monitor-heavy calls (default: 220)
  --quick               Reduced matrix for faster verification
  --keep-sessions       Skip cleanup/kill steps at end
  -h, --help            Show help
USAGE
}

PROJECT_ROOT=""
OUT_DIR=""
SOCKET_DIR=""
MODEL="codex-spark"
POLL_INTERVAL=1
MAX_POLLS=220
QUICK=0
KEEP_SESSIONS=0

while (($# > 0)); do
  case "$1" in
    --project-root)
      PROJECT_ROOT="${2:-}"
      shift 2
      ;;
    --out-dir)
      OUT_DIR="${2:-}"
      shift 2
      ;;
    --socket-dir)
      SOCKET_DIR="${2:-}"
      shift 2
      ;;
    --model)
      MODEL="${2:-}"
      shift 2
      ;;
    --poll-interval)
      POLL_INTERVAL="${2:-}"
      shift 2
      ;;
    --max-polls)
      MAX_POLLS="${2:-}"
      shift 2
      ;;
    --quick)
      QUICK=1
      shift
      ;;
    --keep-sessions)
      KEEP_SESSIONS=1
      shift
      ;;
    -h|--help)
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
  echo "error: missing executable: $BIN" >&2
  exit 1
fi

RUN_ID="$(date +%Y%m%d-%H%M%S)-$$"
if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="/tmp/lisa-deep-stress-$RUN_ID"
fi
if [[ -z "$SOCKET_DIR" ]]; then
  SOCKET_DIR="/tmp/lisa-tmux-deep-stress-$RUN_ID"
fi

mkdir -p "$OUT_DIR" "$SOCKET_DIR"
RESULTS="$OUT_DIR/results.tsv"
printf "label\texpected_rc\trc\tresult\tdur_s\tcmd\tstdout\tstderr\n" > "$RESULTS"

# Session registry for cleanup.
declare -a SESSIONS=()
register_session() {
  local s="$1"
  if [[ -n "$s" ]]; then
    SESSIONS+=("$s")
  fi
}

run_expect() {
  local expected="$1"
  local label="$2"
  shift 2

  local out="$OUT_DIR/${label}.out"
  local err="$OUT_DIR/${label}.err"
  local start end rc dur result

  start=$(date +%s)
  set +e
  LISA_TMUX_SOCKET_DIR="$SOCKET_DIR" "$@" >"$out" 2>"$err"
  rc=$?
  set -e
  end=$(date +%s)
  dur=$((end - start))

  if [[ "$rc" -eq "$expected" ]]; then
    result="ok"
  else
    result="unexpected"
  fi

  printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n" \
    "$label" "$expected" "$rc" "$result" "$dur" "$*" "$out" "$err" >> "$RESULTS"
}

run_expect_shell() {
  local expected="$1"
  local label="$2"
  shift 2
  local script="$*"
  local out="$OUT_DIR/${label}.out"
  local err="$OUT_DIR/${label}.err"
  local start end rc dur result

  start=$(date +%s)
  set +e
  LISA_TMUX_SOCKET_DIR="$SOCKET_DIR" bash -lc "$script" >"$out" 2>"$err"
  rc=$?
  set -e
  end=$(date +%s)
  dur=$((end - start))

  if [[ "$rc" -eq "$expected" ]]; then
    result="ok"
  else
    result="unexpected"
  fi

  printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n" \
    "$label" "$expected" "$rc" "$result" "$dur" "bash -lc $script" "$out" "$err" >> "$RESULTS"
}

main_session="lisa-deep-main-$RUN_ID"
lane_session="lisa-deep-lane-$RUN_ID"
dedupe_session_1="lisa-deep-d1-$RUN_ID"
dedupe_session_2="lisa-deep-d2-$RUN_ID"
dedupe_hash="deep-hash-$RUN_ID"

register_session "$main_session"
register_session "$lane_session"
register_session "$dedupe_session_1"
register_session "$dedupe_session_2"

# Baseline.
run_expect 0 capabilities "$BIN" capabilities --json
run_expect 0 doctor "$BIN" doctor --json
run_expect 0 preflight_fast "$BIN" session preflight --project-root "$PROJECT_ROOT" --agent codex --model "$MODEL" --fast --json
run_expect 0 preflight_full "$BIN" session preflight --project-root "$PROJECT_ROOT" --agent codex --model "$MODEL" --json
run_expect 0 schema_guard "$BIN" session schema --command "session guard" --json
run_expect 1 schema_unknown "$BIN" session schema --command "session nope" --json

# Nested detection matrix.
prompts_file="$OUT_DIR/nested-prompts.txt"
cat > "$prompts_file" <<'EOF'
Use ./lisa for child orchestration.
Run lisa session spawn for child orchestration.
Create nested lisa inside lisa inside lisa and report.
Use lisa inside of lisa inside as well.
No nesting requested here.
The string "./lisa" appears in docs only.
Run ./LISA for child orchestration.
EOF

idx=0
while IFS= read -r prompt; do
  idx=$((idx + 1))
  run_expect 0 "detect_nested_${idx}" "$BIN" session detect-nested --agent codex --mode exec --project-root "$PROJECT_ROOT" --prompt "$prompt" --rewrite --why --json
  run_expect 0 "spawn_dry_nested_${idx}" "$BIN" session spawn --agent codex --mode exec --project-root "$PROJECT_ROOT" --model "$MODEL" --prompt "$prompt" --dry-run --detect-nested --json
done < "$prompts_file"

# Smoke matrix.
if [[ "$QUICK" -eq 1 ]]; then
  styles=(nested neutral)
  chaos_modes=(mixed)
  smoke_levels=2
else
  styles=(none dot-slash spawn nested neutral)
  chaos_modes=(delay drop-marker fail-child mixed)
  smoke_levels=4
fi

for style in "${styles[@]}"; do
  run_expect 0 "smoke_l${smoke_levels}_${style}" \
    "$BIN" session smoke --project-root "$PROJECT_ROOT" --levels "$smoke_levels" \
    --prompt-style "$style" --llm-profile codex --model "$MODEL" \
    --poll-interval "$POLL_INTERVAL" --max-polls "$MAX_POLLS" \
    --report-min --export-artifacts "$OUT_DIR/artifacts-${style}.json" --json
done

run_expect 0 smoke_contract_full \
  "$BIN" session smoke --project-root "$PROJECT_ROOT" --levels "$smoke_levels" \
  --prompt-style nested --contract-profile full --llm-profile codex --model "$MODEL" \
  --poll-interval "$POLL_INTERVAL" --max-polls "$MAX_POLLS" --report-min --json

for chaos in "${chaos_modes[@]}"; do
  run_expect 0 "smoke_chaos_${chaos}_report" \
    "$BIN" session smoke --project-root "$PROJECT_ROOT" --levels "$smoke_levels" \
    --prompt-style nested --chaos "$chaos" --chaos-report \
    --llm-profile codex --model "$MODEL" \
    --poll-interval "$POLL_INTERVAL" --max-polls "$MAX_POLLS" --report-min --json
done

matrix_file="$OUT_DIR/smoke-matrix.txt"
cat > "$matrix_file" <<'EOF'
bypass|Use ./lisa for child orchestration.
full-auto|No nesting requested here.
any|Use lisa inside of lisa inside as well.
bypass|Run lisa session spawn for child orchestration.
any|The string "./lisa" appears in docs only.
EOF
run_expect 0 smoke_matrix "$BIN" session smoke --project-root "$PROJECT_ROOT" --levels 2 --matrix-file "$matrix_file" --llm-profile codex --model "$MODEL" --poll-interval "$POLL_INTERVAL" --max-polls "$MAX_POLLS" --report-min --json

# Long interactive churn.
run_expect 0 spawn_main "$BIN" session spawn --session "$main_session" --agent codex --mode interactive --project-root "$PROJECT_ROOT" --model "$MODEL" --prompt "Output STRESS_READY then wait for inputs." --json
run_expect 0 monitor_main_ready "$BIN" session monitor --session "$main_session" --project-root "$PROJECT_ROOT" --until-state waiting_input --poll-interval "$POLL_INTERVAL" --max-polls "$MAX_POLLS" --json-min --stream-json --emit-handoff --handoff-cursor-file "$OUT_DIR/main.monitor.cursor"

if [[ "$QUICK" -eq 1 ]]; then
  turn_count=4
else
  turn_count=10
fi

for i in $(seq 1 "$turn_count"); do
  run_expect 0 "turn_${i}" "$BIN" session turn --session "$main_session" --project-root "$PROJECT_ROOT" --text "Respond exactly marker STRESS_TURN_${i} then wait." --enter --poll-interval "$POLL_INTERVAL" --max-polls "$MAX_POLLS" --json-min
  run_expect 0 "packet_${i}" "$BIN" session packet --session "$main_session" --project-root "$PROJECT_ROOT" --cursor-file "$OUT_DIR/main.packet.cursor" --delta-json --json
  run_expect 0 "handoff_${i}" "$BIN" session handoff --session "$main_session" --project-root "$PROJECT_ROOT" --cursor-file "$OUT_DIR/main.handoff.cursor" --schema v4 --json-min
done

run_expect 0 send_burst_1 "$BIN" session send --session "$main_session" --project-root "$PROJECT_ROOT" --text "Marker BURST_ONE then wait." --enter --json-min
run_expect 0 send_burst_2 "$BIN" session send --session "$main_session" --project-root "$PROJECT_ROOT" --text "Marker BURST_TWO then wait." --enter --json-min
run_expect 0 send_burst_enter "$BIN" session send --session "$main_session" --project-root "$PROJECT_ROOT" --keys Enter --json-min
run_expect 0 monitor_after_burst "$BIN" session monitor --session "$main_session" --project-root "$PROJECT_ROOT" --until-state waiting_input --poll-interval "$POLL_INTERVAL" --max-polls "$MAX_POLLS" --json-min --stream-json --emit-handoff --handoff-cursor-file "$OUT_DIR/main.monitor2.cursor"

markers_csv="STRESS_READY,STRESS_TURN_1,BURST_ONE,BURST_TWO"
if [[ "$turn_count" -ge 4 ]]; then
  markers_csv="$markers_csv,STRESS_TURN_4"
fi
if [[ "$turn_count" -ge 10 ]]; then
  markers_csv="$markers_csv,STRESS_TURN_10"
fi

run_expect 0 snapshot_main "$BIN" session snapshot --session "$main_session" --project-root "$PROJECT_ROOT" --json-min
run_expect 0 status_main "$BIN" session status --session "$main_session" --project-root "$PROJECT_ROOT" --json-min
run_expect 0 explain_main "$BIN" session explain --session "$main_session" --project-root "$PROJECT_ROOT" --events 12 --recent 8 --json-min
run_expect 0 capture_markers "$BIN" session capture --session "$main_session" --project-root "$PROJECT_ROOT" --raw --markers "$markers_csv" --markers-json --json
run_expect 0 capture_summary "$BIN" session capture --session "$main_session" --project-root "$PROJECT_ROOT" --raw --summary --summary-style ops --token-budget 420 --json
run_expect 0 capture_semantic_1 "$BIN" session capture --session "$main_session" --project-root "$PROJECT_ROOT" --raw --semantic-delta --cursor-file "$OUT_DIR/main.semantic.cursor" --json-min
run_expect 0 capture_semantic_2 "$BIN" session capture --session "$main_session" --project-root "$PROJECT_ROOT" --raw --semantic-delta --cursor-file "$OUT_DIR/main.semantic.cursor" --json-min

run_expect 0 diff_pack_main "$BIN" session diff-pack --session "$main_session" --project-root "$PROJECT_ROOT" --strategy balanced --events 10 --lines 160 --token-budget 500 --cursor-file "$OUT_DIR/main.diff.cursor" --json-min
run_expect 0 context_pack_main "$BIN" session context-pack --for "$main_session" --project-root "$PROJECT_ROOT" --strategy balanced --token-budget 500 --json-min
run_expect 0 handoff_main_v4 "$BIN" session handoff --session "$main_session" --project-root "$PROJECT_ROOT" --schema v4 --json
run_expect 0 context_pack_from_handoff "$BIN" session context-pack --from-handoff "$OUT_DIR/handoff_main_v4.out" --project-root "$PROJECT_ROOT" --strategy terse --json-min

run_expect 0 checkpoint_save_main "$BIN" session checkpoint save --session "$main_session" --file "$OUT_DIR/main.checkpoint.json" --project-root "$PROJECT_ROOT" --events 12 --lines 160 --strategy balanced --token-budget 500 --json
run_expect 0 checkpoint_resume_main "$BIN" session checkpoint resume --file "$OUT_DIR/main.checkpoint.json" --project-root "$PROJECT_ROOT" --json
run_expect 0 replay_main "$BIN" session replay --from-checkpoint "$OUT_DIR/main.checkpoint.json" --project-root "$PROJECT_ROOT" --json

loop_steps=3
if [[ "$QUICK" -eq 1 ]]; then
  loop_steps=1
fi
run_expect 0 loop_main "$BIN" session loop --session "$main_session" --project-root "$PROJECT_ROOT" --poll-interval "$POLL_INTERVAL" --max-polls "$MAX_POLLS" --strategy balanced --events 10 --lines 160 --token-budget 500 --cursor-file "$OUT_DIR/main.loop.diff.cursor" --handoff-cursor-file "$OUT_DIR/main.loop.handoff.cursor" --schema v3 --steps "$loop_steps" --max-tokens 100000 --max-seconds 900 --max-steps 999 --json-min

run_expect 0 next_main "$BIN" session next --session "$main_session" --project-root "$PROJECT_ROOT" --budget 400 --json
run_expect 0 aggregate_main "$BIN" session aggregate --sessions "$main_session" --project-root "$PROJECT_ROOT" --strategy balanced --events 10 --lines 160 --token-budget 500 --delta-json --cursor-file "$OUT_DIR/main.aggregate.cursor" --json-min
run_expect 0 anomaly_main "$BIN" session anomaly --session "$main_session" --project-root "$PROJECT_ROOT" --events 12 --auto-remediate --json
run_expect 0 memory_main "$BIN" session memory --session "$main_session" --project-root "$PROJECT_ROOT" --refresh --semantic-diff --ttl-hours 24 --max-lines 120 --json
run_expect 0 context_cache_main "$BIN" session context-cache --session "$main_session" --project-root "$PROJECT_ROOT" --refresh --ttl-hours 24 --max-lines 180 --json
run_expect 0 context_cache_list "$BIN" session context-cache --project-root "$PROJECT_ROOT" --list --json
run_expect 0 context_cache_clear "$BIN" session context-cache --key "session:$main_session" --project-root "$PROJECT_ROOT" --clear --json

run_expect 0 budget_observe_main "$BIN" session budget-observe --from-jsonl "$OUT_DIR/monitor_after_burst.out" --json
run_expect 0 budget_enforce_main "$BIN" session budget-enforce --from-jsonl "$OUT_DIR/monitor_after_burst.out" --max-tokens 999999 --max-seconds 999999 --max-steps 999999 --json
run_expect 0 budget_plan "$BIN" session budget-plan --goal analysis --agent codex --profile codex-spark --budget 700 --topology planner,workers,reviewer --project-root "$PROJECT_ROOT" --json

run_expect 0 route_queue "$BIN" session route --goal analysis --agent codex --model "$MODEL" --project-root "$PROJECT_ROOT" --queue --sessions "$main_session" --queue-limit 5 --concurrency 2 --topology planner,workers,reviewer --cost-estimate --budget 700 --emit-runbook --json
run_expect 0 route_from_state "$BIN" session route --goal analysis --agent codex --project-root "$PROJECT_ROOT" --from-state "$OUT_DIR/handoff_main_v4.out" --strict --emit-runbook --json

# Autopilot fail+resume (resume-safe: no kill on fail).
run_expect 2 autopilot_fail_fast_nokill "$BIN" session autopilot --goal analysis --agent codex --mode interactive --model "$MODEL" --project-root "$PROJECT_ROOT" --prompt "Output AUTO_RESUME_OK then continue." --poll-interval "$POLL_INTERVAL" --max-polls 1 --capture-lines 120 --summary --summary-style ops --token-budget 200 --kill-after false --json
run_expect_shell 0 autopilot_resume_success "$BIN session autopilot --resume-from '$OUT_DIR/autopilot_fail_fast_nokill.out' --project-root '$PROJECT_ROOT' --poll-interval '$POLL_INTERVAL' --max-polls '$MAX_POLLS' --json"

auto_resume_session=""
if command -v jq >/dev/null 2>&1; then
  auto_resume_session="$(jq -r '.session // empty' "$OUT_DIR/autopilot_fail_fast_nokill.out" 2>/dev/null || true)"
fi
if [[ -n "$auto_resume_session" ]]; then
  register_session "$auto_resume_session"
fi

# Objective/lane contracts.
run_expect 0 objective_set "$BIN" session objective --project-root "$PROJECT_ROOT" --id deep-obj --goal "Deep stress capabilities" --acceptance "No unexpected failures" --budget 900 --activate --json
run_expect 0 objective_list "$BIN" session objective --project-root "$PROJECT_ROOT" --list --json
run_expect 0 lane_set "$BIN" session lane --project-root "$PROJECT_ROOT" --name deep-lane --goal analysis --agent codex --mode interactive --model "$MODEL" --nested-policy off --nesting-intent neutral --contract handoff_v2_required --budget 900 --json
run_expect 0 lane_list "$BIN" session lane --project-root "$PROJECT_ROOT" --list --json

run_expect 0 spawn_lane "$BIN" session spawn --session "$lane_session" --lane deep-lane --agent codex --mode interactive --project-root "$PROJECT_ROOT" --model "$MODEL" --prompt "Output LANE_READY then wait." --json
run_expect 0 monitor_lane "$BIN" session monitor --session "$lane_session" --project-root "$PROJECT_ROOT" --until-state waiting_input --poll-interval "$POLL_INTERVAL" --max-polls "$MAX_POLLS" --json-min
run_expect 1 handoff_lane_no_schema "$BIN" session handoff --session "$lane_session" --project-root "$PROJECT_ROOT" --json-min
run_expect 0 handoff_lane_v2 "$BIN" session handoff --session "$lane_session" --project-root "$PROJECT_ROOT" --schema v2 --json-min

# Dedupe contention.
run_expect 0 spawn_d1 "$BIN" session spawn --session "$dedupe_session_1" --agent codex --mode interactive --project-root "$PROJECT_ROOT" --model "$MODEL" --prompt "D1 ready." --json
run_expect 0 spawn_d2 "$BIN" session spawn --session "$dedupe_session_2" --agent codex --mode interactive --project-root "$PROJECT_ROOT" --model "$MODEL" --prompt "D2 ready." --json
run_expect 0 dedupe_claim_d1 "$BIN" session dedupe --task-hash "$dedupe_hash" --session "$dedupe_session_1" --project-root "$PROJECT_ROOT" --json
run_expect 1 dedupe_claim_d2_conflict "$BIN" session dedupe --task-hash "$dedupe_hash" --session "$dedupe_session_2" --project-root "$PROJECT_ROOT" --json
run_expect 0 dedupe_release "$BIN" session dedupe --task-hash "$dedupe_hash" --release --project-root "$PROJECT_ROOT" --json

# Guard/list/tree edge contracts.
policy_file="$OUT_DIR/guard-policy.json"
cat > "$policy_file" <<'EOF'
{
  "machinePolicy": "strict",
  "requireProjectRoot": true,
  "requireProjectOnlyForKillAll": true,
  "allowCleanupIncludeTmuxDefault": false,
  "deniedCommands": ["cleanup --include-tmux-default"]
}
EOF

run_expect 1 guard_strict_risk "$BIN" session guard --shared-tmux --command "./lisa cleanup --include-tmux-default" --project-root "$PROJECT_ROOT" --json
run_expect 1 guard_enforce_risk "$BIN" session guard --shared-tmux --enforce --command "./lisa cleanup --include-tmux-default" --project-root "$PROJECT_ROOT" --json
run_expect 0 guard_advice_only "$BIN" session guard --shared-tmux --advice-only --command "./lisa cleanup --include-tmux-default" --project-root "$PROJECT_ROOT" --json
run_expect 1 guard_policy_file "$BIN" session guard --shared-tmux --enforce --policy-file "$policy_file" --command "./lisa cleanup --include-tmux-default" --project-root "$PROJECT_ROOT" --json

run_expect 0 list_delta_1 "$BIN" session list --project-only --project-root "$PROJECT_ROOT" --delta-json --cursor-file "$OUT_DIR/list.cursor" --json-min
run_expect 0 tree_delta_1 "$BIN" session tree --project-root "$PROJECT_ROOT" --delta --json
run_expect 1 list_cursor_without_delta "$BIN" session list --project-only --project-root "$PROJECT_ROOT" --cursor-file "$OUT_DIR/list.bad.cursor" --json-min
run_expect 1 tree_cursor_without_delta "$BIN" session tree --project-root "$PROJECT_ROOT" --cursor-file "$OUT_DIR/tree.bad.cursor" --json

# OAuth + skills.
run_expect 0 oauth_list_before "$BIN" oauth list --json
run_expect 0 oauth_add "$BIN" oauth add --token "deep-dummy-token-$RUN_ID" --json
oauth_id=""
if command -v jq >/dev/null 2>&1; then
  oauth_id="$(jq -r '.id // empty' "$OUT_DIR/oauth_add.out" 2>/dev/null || true)"
fi
if [[ -n "$oauth_id" ]]; then
  run_expect 0 oauth_remove "$BIN" oauth remove --id "$oauth_id" --json
else
  run_expect 1 oauth_remove_missing "$BIN" oauth remove --id "missing-$RUN_ID" --json
fi
run_expect 0 oauth_list_after "$BIN" oauth list --json
run_expect 0 skills_sync_codex "$BIN" skills sync --from codex --repo-root "$PROJECT_ROOT" --json
run_expect 0 skills_sync_path "$BIN" skills sync --from path --path "$PROJECT_ROOT/skills/lisa" --repo-root "$PROJECT_ROOT" --json
run_expect 0 skills_doctor "$BIN" skills doctor --repo-root "$PROJECT_ROOT" --deep --explain-drift --contract-check --sync-plan --json
run_expect 0 skills_install_project "$BIN" skills install --to project --project-path "$OUT_DIR/install-project" --repo-root "$PROJECT_ROOT" --json

# State sandbox at end to avoid lane-state interference.
run_expect 0 sandbox_list "$BIN" session state-sandbox list --project-root "$PROJECT_ROOT" --json
run_expect 0 sandbox_snapshot "$BIN" session state-sandbox snapshot --project-root "$PROJECT_ROOT" --file "$OUT_DIR/state-sandbox.json" --json
run_expect 0 sandbox_restore "$BIN" session state-sandbox restore --file "$OUT_DIR/state-sandbox.json" --json
run_expect 0 sandbox_clear "$BIN" session state-sandbox clear --project-root "$PROJECT_ROOT" --json

if [[ "$KEEP_SESSIONS" -eq 0 ]]; then
  for s in "${SESSIONS[@]}"; do
    run_expect 0 "kill_${s}" "$BIN" session kill --session "$s" --project-root "$PROJECT_ROOT" --json
  done
  run_expect 0 kill_all_project "$BIN" session kill-all --project-only --project-root "$PROJECT_ROOT" --json
  run_expect 0 list_final "$BIN" session list --project-only --project-root "$PROJECT_ROOT" --json-min
fi

OUT_ENV="$OUT_DIR" python3 - <<'PY' > "$OUT_DIR/summary.json"
import csv
import json
import os

out = os.environ["OUT_ENV"]
rows = list(csv.DictReader(open(f"{out}/results.tsv"), delimiter="\t"))
unexpected = [r for r in rows if r["result"] != "ok"]

rc_counts = {}
error_codes = {}
for row in rows:
    rc_counts[row["rc"]] = rc_counts.get(row["rc"], 0) + 1
    try:
        raw = open(row["stdout"]).read().strip()
        if not raw:
            continue
        payload = json.loads(raw.splitlines()[-1])
        code = payload.get("errorCode")
        if code:
            error_codes[code] = error_codes.get(code, 0) + 1
    except Exception:
        pass

summary = {
    "total": len(rows),
    "ok": len(rows) - len(unexpected),
    "unexpected": len(unexpected),
    "rc_counts": rc_counts,
    "error_codes": error_codes,
    "unexpected_labels": [
        {
            "label": r["label"],
            "expected_rc": int(r["expected_rc"]),
            "rc": int(r["rc"]),
            "cmd": r["cmd"],
        }
        for r in unexpected
    ],
}
print(json.dumps(summary, indent=2, sort_keys=True))
PY

cat > "$OUT_DIR/run-info.txt" <<EOF
OUT_DIR=$OUT_DIR
SOCKET_DIR=$SOCKET_DIR
PROJECT_ROOT=$PROJECT_ROOT
MODEL=$MODEL
QUICK=$QUICK
KEEP_SESSIONS=$KEEP_SESSIONS
EOF

echo "Deep stress complete"
echo "out: $OUT_DIR"
cat "$OUT_DIR/summary.json"

unexpected_count="$(python3 - <<'PY' "$OUT_DIR/summary.json"
import json
import sys
print(json.load(open(sys.argv[1]))["unexpected"])
PY
)"

if [[ "$unexpected_count" != "0" ]]; then
  exit 1
fi


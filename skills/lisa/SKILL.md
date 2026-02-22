---
name: lisa
description: lisa, tmux, orchestration, claude, codex, spawn, monitor, capture, nested, smoke, skills
author: Claude Code
version: 5.2.0
date: 2026-02-22
tags: [lisa, tmux, orchestration, claude, codex, agents]
---

# Lisa — tmux AI Agent Orchestrator

Axiom: minimum context, deterministic command contracts.

## Always-Load

1. Repo-local pin: `test -x ./lisa || exit 1; LISA_BIN=./lisa`.
2. Use tokenized subcommands: `$LISA_BIN session spawn ...` (never `"session spawn"`).
3. In multi-step/nested flows, always pass `--project-root` on `session *` commands.
4. `cleanup` does not accept `--project-root`; scope cleanup via `session kill|kill-all --project-only --project-root`.
5. For isolated validation (and to avoid touching default tmux), set `LISA_TMUX_SOCKET=/tmp/lisa-tmux-<tag>.sock`.
6. For JSON workflows, parse `stdout` contract payloads; treat `stderr` as diagnostics and use `stderrPolicy`.
7. For marker-gated monitor (`--until-marker`), choose a unique marker not present in prompt text.
8. In shared tmux environments, run `session guard --shared-tmux --json` before cleanup/kill-all actions.
9. `session guard --shared-tmux` returning `safe:false` is expected risk signaling (often exit `1`); switch to `--project-only` flows.

## Crucial Commands

```bash
test -x ./lisa || { echo "missing ./lisa"; exit 1; }
LISA_BIN=./lisa
ROOT=/path/to/project
MODEL="${MODEL:-gpt-5.3-codex-spark}"

# preflight
$LISA_BIN session preflight --project-root "$ROOT" --json
$LISA_BIN session preflight --agent codex --model "$MODEL" --project-root "$ROOT" --json || \
  echo "explicit model preflight failed; probing auto-model candidates"
$LISA_BIN session preflight --agent codex --auto-model --project-root "$ROOT" --json || \
  echo "auto-model probe failed; set --model explicitly"

# spawn + monitor + capture + cleanup
SESSION=$($LISA_BIN session spawn --agent codex --mode interactive --project-root "$ROOT" --prompt "Do X, then wait." --json | jq -r .session)
$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --until-state waiting_input --json-min --stream-json
$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --until-jsonpath '$.sessionState=waiting_input' --json-min
$LISA_BIN session snapshot --session "$SESSION" --project-root "$ROOT" --json-min
$LISA_BIN session capture --session "$SESSION" --project-root "$ROOT" --raw --cursor-file /tmp/lisa.cursor --json-min
$LISA_BIN session capture --session "$SESSION" --project-root "$ROOT" --raw --summary --summary-style ops --token-budget 320 --json
$LISA_BIN session handoff --session "$SESSION" --project-root "$ROOT" --cursor-file /tmp/lisa.handoff.cursor --json-min
$LISA_BIN session context-pack --for "$SESSION" --project-root "$ROOT" --strategy balanced --json-min
$LISA_BIN session kill --session "$SESSION" --project-root "$ROOT" --json

# shared tmux safety gate + cleanup planning
$LISA_BIN session guard --shared-tmux --enforce --command "./lisa session kill-all --project-only --project-root $ROOT" --json
$LISA_BIN cleanup --dry-run --json

# policy-driven single-command loop
$LISA_BIN session autopilot --goal analysis --agent codex --project-root "$ROOT" \
  --summary --summary-style ops --token-budget 320 --kill-after true --json
```

## Nested Diagnostics

```bash
$LISA_BIN session detect-nested --prompt "Use ./lisa for child orchestration." --json
$LISA_BIN session detect-nested --prompt "Use lisa inside of lisa inside as well." --json
# expected: reason=no_nested_hint (non-trigger phrase)
$LISA_BIN session detect-nested --prompt "Use lisa inside of lisa inside as well." --rewrite --json
# expected: rewrites[] provides trigger-safe prompt alternatives
$LISA_BIN session detect-nested --prompt "Use ./LISA for child orchestration." --json
# expected: case-insensitive match => reason=prompt_contains_dot_slash_lisa
$LISA_BIN session spawn --agent codex --mode exec --project-root "$ROOT" \
  --prompt "Create nested lisa inside lisa inside lisa and report" \
  --dry-run --detect-nested --json
$LISA_BIN session smoke --project-root "$ROOT" --levels 4 --prompt-style nested --model "$MODEL" --json
$LISA_BIN session smoke --project-root "$ROOT" --levels 4 --report-min --json
$LISA_BIN session route --goal nested --project-root "$ROOT" --emit-runbook --json
```

Trigger wording quick-map:
- Trigger nested bypass: `Use ./lisa ...`, `Run lisa session spawn ...`, `nested lisa ...`
- Non-trigger phrase: `Use lisa inside of lisa inside as well.` (`reason=no_nested_hint`)
- Quote/doc mention is non-trigger: `The string "./lisa" appears in docs only.`

Model note:
- Prefer lowercase model ids (`gpt-5.3-codex-spark`); mixed-case aliases may fail model preflight.

Exit/contract quick-map:
- `monitor` success exit `0`: `completed|waiting_input|waiting_input_turn_complete|marker_found`
- `monitor` non-success exit `2`: `crashed|stuck|not_found|max_polls_exceeded|degraded_max_polls_exceeded|expected_*`
- Usage/flag/runtime errors exit `1` (for example `--expect marker` without `--until-marker`)
- `autopilot` propagates failing step exit (`monitor` often `2` for timeout/terminal mismatch)

## Router (JIT)

| Need | Load |
|---|---|
| Command contracts: flags/defaults/exits/JSON schemas | `data/commands.md` |
| Runbooks: orchestration flows, nested prompts, smoke/matrix recipes | `data/recipes.md` |
| Runtime semantics: state machine/artifacts/env overrides/operational notes | `data/runtime.md` |
| Verified behavior + fast confidence loops | `data/validation.md` |
| Context-optimization feature backlog (scored) | `data/feature-ideas.md` |

Load only needed files; never bulk-load all data docs by default.

## Control Flow

```text
if need_new_worker: session spawn
if need_progress_or_gate: session monitor (optional --expect/--until-marker)
if need_low_token_status: session snapshot or status --json-min
if need_output: session capture (--delta-from for incremental)
if need_handoff_packet: session handoff or session context-pack
if done_or_abort: session kill or session kill-all --project-only
if shared tmux: session guard --shared-tmux, then cleanup --dry-run
```

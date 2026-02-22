---
name: lisa
description: lisa, tmux, orchestration, claude, codex, spawn, monitor, capture, nested, smoke, skills
author: Claude Code
version: 5.0.0
date: 2026-02-21
tags: [lisa, tmux, orchestration, claude, codex, agents]
---

# Lisa — tmux AI Agent Orchestrator

Axiom: minimum context, deterministic command contracts.

## Always-Load

1. Repo-local pin: `test -x ./lisa || exit 1; LISA_BIN=./lisa`.
2. Use tokenized subcommands: `$LISA_BIN session spawn ...` (never `"session spawn"`).
3. In multi-step/nested flows, always pass `--project-root` on `session *` and cleanup calls.
4. For JSON workflows, parse `stdout` contract payloads; treat `stderr` as diagnostics and use `stderrPolicy`.
5. For marker-gated monitor (`--until-marker`), choose a unique marker not present in prompt text.
6. In shared tmux environments, run `session guard --shared-tmux --json` before cleanup/kill-all actions.

## Crucial Commands

```bash
test -x ./lisa || { echo "missing ./lisa"; exit 1; }
LISA_BIN=./lisa
ROOT=/path/to/project
MODEL="${MODEL:-gpt-5.3-codex-spark}"

# preflight
$LISA_BIN session preflight --json
$LISA_BIN session preflight --agent codex --auto-model --json || \
  echo "auto-model probe failed; set --model explicitly"

# spawn + monitor + capture + cleanup
SESSION=$($LISA_BIN session spawn --agent codex --mode interactive --project-root "$ROOT" --prompt "Do X, then wait." --json | jq -r .session)
$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --until-state waiting_input --json-min --stream-json
$LISA_BIN session snapshot --session "$SESSION" --project-root "$ROOT" --json-min
$LISA_BIN session capture --session "$SESSION" --project-root "$ROOT" --raw --cursor-file /tmp/lisa.cursor --json-min
$LISA_BIN session capture --session "$SESSION" --project-root "$ROOT" --raw --summary --token-budget 320 --json
$LISA_BIN session handoff --session "$SESSION" --project-root "$ROOT" --delta-from 0 --json-min
$LISA_BIN session context-pack --for "$SESSION" --project-root "$ROOT" --strategy balanced --json-min
$LISA_BIN session kill --session "$SESSION" --project-root "$ROOT"
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

Model note:
- Prefer lowercase model ids (`gpt-5.3-codex-spark`); mixed-case aliases may fail model preflight.

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

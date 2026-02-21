---
name: lisa
description: lisa, tmux, orchestration, claude, codex, spawn, monitor, capture, nested, smoke, skills
author: Claude Code
version: 4.0.0
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

## Crucial Commands

```bash
test -x ./lisa || { echo "missing ./lisa"; exit 1; }
LISA_BIN=./lisa
ROOT=/path/to/project

# preflight
$LISA_BIN session preflight --json

# spawn + monitor + capture + cleanup
SESSION=$($LISA_BIN session spawn --agent codex --mode interactive --project-root "$ROOT" --prompt "Do X, then wait." --json | jq -r .session)
$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --json-min --stream-json
$LISA_BIN session snapshot --session "$SESSION" --project-root "$ROOT" --json-min
$LISA_BIN session capture --session "$SESSION" --project-root "$ROOT" --raw --delta-from 0 --json-min
$LISA_BIN session kill --session "$SESSION" --project-root "$ROOT"
```

## Nested Diagnostics

```bash
$LISA_BIN session detect-nested --prompt "Use ./lisa for child orchestration." --json
$LISA_BIN session spawn --agent codex --mode exec --project-root "$ROOT" \
  --prompt "Create nested lisa inside lisa inside lisa and report" \
  --dry-run --detect-nested --json
$LISA_BIN session smoke --project-root "$ROOT" --levels 4 --prompt-style nested --json
```

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
if done_or_abort: session kill or session kill-all
if shared tmux: cleanup --dry-run before cleanup
```

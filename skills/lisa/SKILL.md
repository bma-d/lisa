---
name: lisa
description: lisa, tmux, orchestration, claude, codex, spawn, monitor, capture, nested, smoke, skills
author: Claude Code
version: 3.0.0
date: 2026-02-21
tags: [lisa, tmux, orchestration, claude, codex, agents]
---

# Lisa — tmux AI Agent Orchestrator

Axiom: load minimal context first, then route to one targeted data file.

## Always-Load Rules

1. In this repo, run `./lisa` (not `lisa`) to avoid PATH/version drift.
2. Use real subcommands: `./lisa session spawn ...` (not `"session spawn"` as one token).
3. In multi-step or nested flows, always pass `--project-root` so socket/hash routing stays consistent.
4. Use `./lisa cleanup --include-tmux-default` only when explicitly requested.

## Prerequisites

```bash
brew install bma-d/tap/lisa   # or: go install github.com/bma-d/lisa@latest
LISA_BIN=./lisa
$LISA_BIN doctor
```

## Router (JIT)

| Need | Load |
|---|---|
| Command flags, defaults, JSON schemas, mode aliases, command matrix | `data/commands.md` |
| Execution playbooks, nested orchestration prompts, marker/waiting recipes | `data/recipes.md` |
| State machine semantics, exit reasons/codes, artifacts, env overrides | `data/runtime.md` |

Load only the referenced file(s); do not bulk-load all `data/*.md` unless task spans multiple domains.

## LLM Fast Path

```bash
ROOT=/path/to/project
LISA_BIN=./lisa

SESSION=$($LISA_BIN session spawn \
  --agent codex --mode interactive \
  --project-root "$ROOT" \
  --prompt "Do X, then wait." \
  --json | jq -r .session)

$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --json
$LISA_BIN session capture --session "$SESSION" --project-root "$ROOT" --raw --lines 300
$LISA_BIN session kill --session "$SESSION" --project-root "$ROOT"
```

## Control Flow (Pseudo)

```text
if need_new_worker: session spawn
if need_progress_or_gate: session monitor (set --expect/--until-marker/--stop-on-waiting)
if need_output: session capture
if done_or_abort: session kill or session kill-all
if finishing_runbook: cleanup --dry-run, then cleanup
```

## Observed Behavior (Validated 2026-02-21)

- `session monitor --until-marker` returns success with `exitReason:"marker_found"`, often while state remains `in_progress`/`active`.
- `--waiting-requires-turn-complete true` can timeout (`max_polls_exceeded`) for custom-command flows without transcript turn boundaries.
- `session exists` prints `false` and exits `1` when missing.

## Data File Map

- `data/commands.md`: authoritative command contracts.
- `data/recipes.md`: copy/paste orchestration runbooks.
- `data/runtime.md`: process-first classifier, runtime internals, operational constraints.

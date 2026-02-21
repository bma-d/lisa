---
name: lisa
description: lisa, tmux, orchestration, claude, codex, spawn, monitor, capture, nested, smoke, skills
author: Claude Code
version: 3.6.1
date: 2026-02-21
tags: [lisa, tmux, orchestration, claude, codex, agents]
---

# Lisa — tmux AI Agent Orchestrator

Axiom: load minimal context first, then route to one targeted data file.

## Always-Load Rules

1. For repo-local runs, always use `./lisa` (never `lisa` from `PATH`); fail fast if missing.
2. Use real subcommands: `$LISA_BIN session spawn ...` (not `"session spawn"` as one token).
3. In multi-step or nested flows, always pass `--project-root` on `session *` and `cleanup` commands so socket/hash routing stays consistent.
4. Use `$LISA_BIN cleanup --include-tmux-default` only when explicitly requested.
5. In `--json` mode, parse `stdout`; use `stderrPolicy` in payload to classify stderr as diagnostic stream.

## LLM Guardrails (Validated 2026-02-21)

- Treat `session list` as source of truth for active sessions.
- Treat `session tree` as metadata graph; it can include historical/stale roots. Use `session tree --active-only` for active-only topology.
- `session monitor --expect marker` requires `--until-marker`; otherwise usage error (exit `1`).
- `session kill --json` for missing session exits `1` and emits JSON with `found:false` (no human stderr line).
- `--waiting-requires-turn-complete true` can timeout (`max_polls_exceeded`) when turn-complete cannot be inferred (common in Codex flows).
- Timeout payloads use `finalState:"timeout"` and `finalStatus:"timeout"`.
- For low-token polling, use `session monitor --json-min` or `session monitor --json-min --stream-json`.
- For low-token snapshots, use `session snapshot --json-min` (or `session status --json-min`, `session list --json-min`, `session tree --json-min`).
- `session status` returns exit `0` on `not_found` unless `--fail-not-found` is set.
- For nested Codex diagnostics, use `session spawn --detect-nested --json` and inspect `nestedDetection`.
- Use `session detect-nested --json` for prompt-policy diagnostics without spawning tmux sessions.
- For deterministic nested policy, use `session spawn --nested-policy auto|force|off`.
- Use `--nesting-intent auto|nested|neutral` to override prompt heuristics deterministically.
- For Codex model pinning, use `--model <NAME>` on `session spawn` / `agent build-cmd` (example: `GPT-5.3-Codex-Spark`).
- Validate model availability with `session preflight --agent codex --model <NAME> --json` before large runs.
- For low-token capture polling, use `session capture --raw --delta-from <offset|@unix|rfc3339> --json-min` and reuse `nextOffset` (emitted when `--delta-from` is set).
- `session monitor` final JSON now includes `nextOffset` when capture is available.
- Raw capture default filters MCP startup/auth noise; use `--keep-noise` for full startup logs.
- Use `session preflight --json` before complex orchestration to validate environment + command contracts in one call.
- `agent build-cmd` accepts `--project-root` for context parity with session commands.
- `session send --json-min` returns tiny ack payload (`session`,`ok`).

## Prerequisites

```bash
brew install bma-d/tap/lisa   # or: go install github.com/bma-d/lisa@latest
test -x ./lisa || { echo "missing ./lisa (build/install first)"; exit 1; }
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
test -x ./lisa || { echo "missing ./lisa"; exit 1; }
LISA_BIN=./lisa

SESSION=$($LISA_BIN session spawn \
  --agent codex --mode interactive \
  --project-root "$ROOT" \
  --prompt "Do X, then wait." \
  --json | jq -r .session)

$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --json
$LISA_BIN session monitor --session "$SESSION" --project-root "$ROOT" --json-min --stream-json
$LISA_BIN session capture --session "$SESSION" --project-root "$ROOT" --raw --lines 300
$LISA_BIN session capture --session "$SESSION" --project-root "$ROOT" --raw --delta-from 0 --json-min
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
- `--waiting-requires-turn-complete true` can timeout (`max_polls_exceeded`) when transcript turn boundaries are unavailable.
- `session exists` prints `false` and exits `1` when missing.
- `session smoke --levels 1..4 --json` passed in this repo (nested markers validated at each level).
- `session smoke --prompt-style dot-slash|spawn|nested|neutral` now validates nested wording triggers via dry-run probe.
- Nested wording detection is case-insensitive (`./LISA` still matches `./lisa` hint).
- Doc/quoted mentions (for example `The string './lisa' appears in docs only.`) do not trigger nested bypass.
- Some account contexts reject specific Codex models; preflight model probe catches this early.

## Quick Verification Loop

```bash
ROOT="$(pwd)"
test -x ./lisa || { echo "missing ./lisa"; exit 1; }
LISA_BIN=./lisa

# Fast contract checks
$LISA_BIN session preflight --json
$LISA_BIN session preflight --agent codex --model GPT-5.3-Codex-Spark --json
$LISA_BIN session monitor --help
$LISA_BIN session smoke --help
$LISA_BIN session detect-nested --prompt "Use ./lisa for child orchestration." --json

# Nested wording probes (dry-run, no sessions created)
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

# Deterministic nested validation
$LISA_BIN session smoke --project-root "$ROOT" --levels 4 --json
$LISA_BIN session smoke --project-root "$ROOT" --matrix-file ./nested-matrix.txt --json
```

## Feature Backlog (Context-Optimized)

Scored by LLM-context impact (100 = highest):

- `96/100` Prompt linter mode (`session lint-prompt`) that predicts nested policy, marker hygiene, and likely monitor stop reason before spawn.
- `93/100` Spawn templates (`session spawn --template nested-4`) with strict, tested prompt blocks and required markers.
- `91/100` Contract self-check (`skills verify`) that compares SKILL contracts against live `capabilities`/help output and reports drift.
- `89/100` Monitor digest mode (`session monitor --digest`) returning compact state deltas only (changed fields) for low-token polling.
- `87/100` Tree focus mode (`session tree --focus <session>`) returning only ancestor/descendant slice for large graphs.
- `85/100` Capture semantic filters (`session capture --section errors|markers|decisions`) for targeted extraction.
- `84/100` Preflight model matrix (`session preflight --agent codex --model-file <txt>`) with ranked supported models.
- `82/100` Session resume macro (`session resume-plan`) that rehydrates waiting sessions with next-step prompts from event tail.
- `80/100` Orchestration scorecard (`session score`) combining success rate, timeout rate, and stale-session debt by project hash.
- `78/100` Deterministic redaction profiles (`capture --redact-profile`) for sharing outputs without leaking tokens/secrets.

## Data File Map

- `data/commands.md`: authoritative command contracts.
- `data/recipes.md`: copy/paste orchestration runbooks.
- `data/runtime.md`: process-first classifier, runtime internals, operational constraints.

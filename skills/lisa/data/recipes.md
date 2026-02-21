# Lisa Recipes

Use `LISA_BIN=./lisa` in this repo.

## Core Pattern

Spawn -> Monitor -> Capture -> Cleanup.

```bash
# 1) spawn
S=$($LISA_BIN session spawn \
  --agent claude --mode interactive \
  --prompt "Refactor auth module" \
  --project-root /path/to/project \
  --json | jq -r .session)

# 2) monitor
$LISA_BIN session monitor \
  --session "$S" \
  --project-root /path/to/project \
  --poll-interval 30 --max-polls 120 \
  --json

# 3) capture
$LISA_BIN session capture --session "$S" --json
$LISA_BIN session capture --session "$S" --raw --lines 200
$LISA_BIN session capture --session "$S" --raw --keep-noise --lines 200

# 4) cleanup
$LISA_BIN cleanup --dry-run
$LISA_BIN cleanup
```

## Preflight Contract Check (Fast)

Use before complex orchestration to lock command assumptions:

```bash
$LISA_BIN doctor --json
$LISA_BIN session preflight --json
$LISA_BIN session spawn --help
$LISA_BIN session monitor --help
$LISA_BIN session tree --help
```

## Parallel Workers

```bash
S1=$($LISA_BIN session spawn --agent claude --mode exec \
  --prompt "Write unit tests for auth.go" \
  --project-root . --json | jq -r .session)

S2=$($LISA_BIN session spawn --agent claude --mode exec \
  --prompt "Add input validation to handlers.go" \
  --project-root . --json | jq -r .session)

$LISA_BIN session monitor --session "$S1" --project-root . --json &
$LISA_BIN session monitor --session "$S2" --project-root . --json &
wait
```

## Capture Transcript vs Raw

```bash
# Structured Claude transcript
$LISA_BIN session capture --session "$S" --json

# Plain text transcript
$LISA_BIN session capture --session "$S"

# Raw pane (noise-filtered default)
$LISA_BIN session capture --session "$S" --raw --lines 200 --json

# Raw pane with startup noise/chrome
$LISA_BIN session capture --session "$S" --raw --keep-noise --lines 200 --json

# Low-token incremental raw capture (offset polling)
$LISA_BIN session capture --session "$S" --raw --delta-from 0 --json
# use returned nextOffset for subsequent polls
$LISA_BIN session capture --session "$S" --raw --delta-from "$NEXT_OFFSET" --json

# Compact JSON payload variants
$LISA_BIN session capture --session "$S" --json-min
$LISA_BIN session capture --session "$S" --raw --delta-from "$NEXT_OFFSET" --json-min
```

Transcript resolution path:
- match session prompt + timestamp in `~/.claude/history.jsonl`
- resolve session id
- read `~/.claude/projects/{encoded-path}/{sessionId}.jsonl`

If prompt+createdAt metadata is absent, default capture falls back to raw pane capture.

## Send Follow-up to Interactive Session

```bash
$LISA_BIN session send --session "$S" --text "Now add error handling" --enter
$LISA_BIN session send --session "$S" --keys "Escape"
```

## Poll + Diagnose

```bash
# one-shot CSV
$LISA_BIN session status --session "$S" --project-root .

# low-token status snapshot
$LISA_BIN session status --session "$S" --project-root . --json-min

# deep diagnostics
$LISA_BIN session explain --session "$S" --project-root . --events 20

# monitor with progress logs to stderr
$LISA_BIN session monitor --session "$S" --project-root . --verbose --json

# line-delimited low-token poll stream + final result
$LISA_BIN session monitor --session "$S" --project-root . --json-min --stream-json
```

Expectation patterns:

```bash
# strict terminal completion (fails on waiting/marker)
$LISA_BIN session monitor --session "$S" --project-root . --expect terminal --json

# marker-gated success (requires --until-marker)
$LISA_BIN session monitor --session "$S" --project-root . \
  --until-marker "DONE_MARKER" --expect marker --json
```

## Cleanup Patterns

```bash
$LISA_BIN session kill --session "$S" --project-root .
$LISA_BIN session kill-all --project-only --project-root .
$LISA_BIN session kill-all --cleanup-all-hashes

$LISA_BIN cleanup --dry-run
$LISA_BIN cleanup
$LISA_BIN cleanup --include-tmux-default
```

Safety: prefer `cleanup --dry-run` first in shared tmux environments.

## Nested Orchestration (Lisa-in-Lisa)

```bash
PARENT=$($LISA_BIN session spawn --agent codex --mode interactive \
  --project-root . \
  --nested-policy auto \
  --prompt "Use ./lisa only. Spawn 2 exec workers, monitor both, then summarize findings." \
  --detect-nested \
  --json | jq -r .session)

$LISA_BIN session monitor --session "$PARENT" --project-root . --stop-on-waiting true --json
$LISA_BIN session monitor --session "$PARENT" --project-root . --stop-on-waiting true --json-min

$LISA_BIN session monitor --session "$PARENT" --project-root . \
  --stop-on-waiting true --waiting-requires-turn-complete true --json

$LISA_BIN session send --session "$PARENT" \
  --text "If incomplete, run ./lisa session list --project-root . and continue." \
  --enter
```

Nested Codex exec trigger wording (auto-bypass + omit `--full-auto`):
- `Use ./lisa for all child orchestration.`
- `Run ./lisa session spawn inside the spawned agent.`
- `Build a nested lisa chain and report markers.`
- `Create nested lisa inside lisa inside lisa and report.`

Wording that does not trigger nested bypass:
- `No nesting requested here.`

Tip: validate trigger intent with `session spawn --dry-run --json` and inspect `command` for
`--dangerously-bypass-approvals-and-sandbox` vs `--full-auto`.
Matcher notes: matching is case-insensitive, and bare `lisa session spawn` still matches, but prefer `./lisa` phrasing in prompts.

Explicit nested policy controls:

```bash
# force bypass even when prompt has no nesting hint
$LISA_BIN session spawn --agent codex --mode exec --nested-policy force \
  --prompt "No nesting requested here." --dry-run --detect-nested --json

# disable prompt-triggered bypass
$LISA_BIN session spawn --agent codex --mode exec --nested-policy off \
  --prompt "Use ./lisa for child orchestration." --dry-run --detect-nested --json
```

Trigger calibration sweep:

```bash
for p in \
  "Use ./lisa for all child orchestration." \
  "Run ./lisa session spawn inside the spawned agent." \
  "Build a nested lisa chain and report markers." \
  "Create nested lisa inside lisa inside lisa and report." \
  "Run ./LISA for children." \
  "No nesting requested here."
do
  $LISA_BIN session spawn --agent codex --mode exec --project-root . \
    --prompt "$p" --dry-run --detect-nested --json | jq --arg prompt "$p" '{prompt:$prompt,command,nestedDetection}'
done
```

Deterministic nested validation:

```bash
./lisa session smoke --project-root "$(pwd)" --levels 4 --json
./smoke-nested --project-root "$(pwd)" --max-polls 120

# include nested wording probe in smoke summary
./lisa session smoke --project-root "$(pwd)" --levels 4 --prompt-style dot-slash --json
```

Four-level matrix (quick confidence loop):

```bash
for L in 1 2 3 4; do
  ./lisa session smoke --project-root "$(pwd)" --levels "$L" --json
done
```

## Creative Nested Chain (Parent -> Child -> Grandchild)

```bash
ROOT="$(pwd)"
PARENT=$($LISA_BIN session spawn --agent codex --mode exec --project-root "$ROOT" \
  --prompt "Use ./lisa only. Spawn one child codex exec session that asks that child to spawn one grandchild codex exec session. In each level emit markers N1_OK, N2_OK, N3_OK into output and finish." \
  --json | jq -r .session)

$LISA_BIN session monitor --session "$PARENT" --project-root "$ROOT" \
  --poll-interval 2 --max-polls 120 --expect terminal --json

$LISA_BIN session capture --session "$PARENT" --project-root "$ROOT" --raw --lines 260 --json
$LISA_BIN session tree --project-root "$ROOT" --active-only --json
```

JSON parsing hygiene:
- parse JSON from `stdout` only
- use `stderrPolicy` in payload to classify stderr as diagnostics channel

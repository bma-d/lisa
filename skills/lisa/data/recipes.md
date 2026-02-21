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

# deep diagnostics
$LISA_BIN session explain --session "$S" --project-root . --events 20

# monitor with progress logs to stderr
$LISA_BIN session monitor --session "$S" --project-root . --verbose --json
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
  --prompt "Use ./lisa only. Spawn 2 exec workers, monitor both, then summarize findings." \
  --json | jq -r .session)

$LISA_BIN session monitor --session "$PARENT" --project-root . --stop-on-waiting true --json

$LISA_BIN session monitor --session "$PARENT" --project-root . \
  --stop-on-waiting true --waiting-requires-turn-complete true --json

$LISA_BIN session send --session "$PARENT" \
  --text "If incomplete, run ./lisa session list --project-root . and continue." \
  --enter
```

Nested Codex exec trigger wording (auto-bypass + omit `--full-auto`):
- `Use ./lisa for all child orchestration.`
- `Run lisa session spawn inside the spawned agent.`
- `Build a nested lisa chain and report markers.`

Deterministic nested validation:

```bash
./lisa session smoke --project-root "$(pwd)" --levels 4 --json
./smoke-nested --project-root "$(pwd)" --max-polls 120
```

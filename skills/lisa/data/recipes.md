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
- `Run lisa session spawn inside the spawned agent.`
- `Build a nested lisa chain and report markers.`

Wording that does not trigger nested bypass:
- `No nesting requested here.`

Tip: validate trigger intent with `session spawn --dry-run --json` and inspect `command` for
`--dangerously-bypass-approvals-and-sandbox` vs `--full-auto`.

Deterministic nested validation:

```bash
./lisa session smoke --project-root "$(pwd)" --levels 4 --json
./smoke-nested --project-root "$(pwd)" --max-polls 120
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

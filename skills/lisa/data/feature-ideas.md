# Lisa Context-Optimization Feature Ideas

Scoring: LLM-context impact out of 100.

- `96/100` Prompt linter mode: `session lint-prompt` predicts nested policy, marker hygiene, and likely monitor stop reason before spawn.
- `93/100` Spawn templates: `session spawn --template nested-4` with strict prompt scaffolds + required markers.
- `91/100` Contract drift self-check: `skills verify` compares skill docs against `capabilities`/help output.
- `89/100` Monitor digest mode: `session monitor --digest` returns changed fields only for low-token loops.
- `87/100` Tree focus mode: `session tree --focus <session>` returns ancestor/descendant slice only.
- `85/100` Capture semantic filters: `session capture --section errors|markers|decisions`.
- `84/100` Preflight model matrix: `session preflight --agent codex --model-file <txt>` ranks supported models.
- `82/100` Resume macro: `session resume-plan` rehydrates waiting sessions using event tail.
- `80/100` Orchestration scorecard: `session score` tracks completion/timeout/stale debt by project hash.
- `78/100` Redaction profiles: `session capture --redact-profile` for safe sharing.

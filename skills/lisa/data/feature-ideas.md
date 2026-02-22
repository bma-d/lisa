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
- `95/100` Smoke model pin: `session smoke --model <NAME>` to force consistent Codex model across all nested levels.
- `92/100` Marker-safe monitor: `session monitor --until-marker X --ignore-prompt-echo` to prevent false-positive marker hits from echoed prompts.
- `90/100` Nested wording helper: `session detect-nested --suggest-trigger "<prompt>"` returns minimal rewrite that deterministically hits bypass/full-auto intent.
- `88/100` Model discovery command: `session models --agent codex --json` outputs supported aliases + preferred defaults for current Codex binary.
- `86/100` Cross-socket provenance: `session list --all-sockets --with-project` includes project root/hash per row for faster multi-project disambiguation.

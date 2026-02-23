# Lisa Context-Optimization Features (Implemented)

Scoring: LLM-context impact out of 100.

- `96/100` `skills doctor --deep`: recursive content-hash drift detection (beyond version/capability checks).
- `94/100` `session tree --with-state --json-min`: topology + current status/sessionState in one call.
- `93/100` `session handoff --delta-from <N>`: incremental event handoff with `nextDeltaOffset`.
- `92/100` `session monitor --stream-json --emit-handoff`: periodic compact handoff packets during polling.
- `91/100` `session detect-nested --rewrite`: trigger-safe prompt rewrite suggestions.
- `89/100` `session context-pack --strategy terse|balanced|full`: deterministic packing profiles.
- `88/100` `session route --emit-runbook`: executable spawn/monitor/capture/handoff plan JSON.
- `87/100` `session capture --summary --token-budget N`: bounded built-in capture summary.
- `85/100` `session smoke --report-min`: CI-focused compact smoke failure payload.
- `84/100` `session list --stale --prune-preview`: safe stale-session cleanup planning payload.
- `97/100` `session autopilot`: single-command spawn/monitor/capture/handoff/cleanup orchestration loop.
- `95/100` `session handoff --cursor-file`: resume-safe incremental handoff offsets.
- `93/100` `session context-pack --from-handoff`: repack state without live tmux polling.
- `91/100` `session monitor --until-jsonpath`: structured stop conditions from status payload fields.
- `90/100` `session route --budget`: token-budget-aware capture/context-pack runbook emission.
- `89/100` `session list --active-only --with-next-action`: triage-ready session queue payload.
- `87/100` `session capture --summary-style terse|ops|debug`: role-specific summary shaping.
- `86/100` `session smoke --chaos`: deterministic failure-mode smoke coverage.
- `85/100` `session guard --enforce`: explicit hard-fail policy for risky shared-tmux plans.
- `84/100` `skills doctor --explain-drift`: remediation hints embedded in drift diagnostics.

Additional round (2026-02-22):

- `95/100` `session packet`: one-call status + capture summary + handoff events with optional delta cursor.
- `94/100` `session monitor --handoff-cursor-file`: resume-safe incremental handoff stream packets.
- `93/100` `session list --delta-json`: incremental added/removed/changed session queue for low-noise orchestration loops.
- `92/100` `session autopilot --resume-from`: step-level resume from prior JSON summary (`resumedFrom`,`resumeStep`).
- `91/100` `session route --from-state`: route computation from handoff/status payloads without re-querying live state.
- `90/100` `session detect-nested --why`: hint-span explainability payload for nested bypass decisions.
- `88/100` `session capture --markers-json`: structured marker hit offsets/lines/timestamps for parser-safe gates.
- `87/100` `session guard --advice-only`: non-blocking safety diagnostics for high-churn orchestrator loops.
- `85/100` `session preflight --fast`: reduced high-risk contract checks for tight startup budgets.
- `83/100` `session tree --json-min` total/filtered counts: explicit topology cardinality for cheap health probes.

## Proposed Additions (Not Implemented)

Scoring: projected LLM-context impact out of 100.

- `98/100` `session objective`: first-class objective register (`id`, `goal`, `acceptance`, `budget`) with automatic propagation into `spawn/send/handoff/context-pack`.
- `96/100` `session memory`: rolling per-session semantic memory with TTL + size caps; emits compact “what changed since last loop” blocks.
- `95/100` `session handoff --schema v2`: strict typed payloads (`state`, `nextAction`, `risks`, `openQuestions`) for parser-safe multi-agent routers.
- `94/100` `session router --queue`: queue-aware routing that reads `session list --with-next-action` + budgets and proposes prioritized dispatch order.
- `93/100` `session monitor --auto-recover`: policy-based retries on transient `degraded|max_polls_exceeded` with bounded recovery budget.
- `92/100` `session lane`: named orchestration lanes (planner|worker|reviewer) with lane-local defaults and handoff contracts.
- `91/100` `session diff-pack --semantic-only`: AST/symbol-level delta summaries to suppress unchanged boilerplate in loops.
- `90/100` `session budget-plan`: pre-run budget simulation from route + topology + historical durations, with hard-stop plan generation.
- `89/100` `session guard --policy-file`: declarative org policy (allowed commands, shared-tmux constraints, cleanup rules) loaded from file.
- `88/100` `session smoke --llm-profile`: profile packs for common orchestrators (Codex, Claude, mixed) with expected detection + routing assertions.

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

Feature bundle implemented (2026-02-22, LLM context-optimization):

- `98/100` `session schema --command <name> --json`: machine-readable payload contracts for parser-safe orchestrators.
- `96/100` `session packet --fields <csv>`: server-side JSON field projection for low-token loops.
- `95/100` `session monitor --event-budget <N>`: adaptive handoff delta compression in stream mode.
- `95/100` `session checkpoint save|resume`: atomic state bundles for resume-safe orchestration.
- `93/100` `session dedupe --task-hash`: duplicate-work guardrail via task claim registry.
- `92/100` `session route --topology planner,workers,reviewer`: topology graph payload for multi-agent planning.
- `91/100` `session context-pack --redact <rules>`: built-in redaction for safe handoff transfer.
- `90/100` `session smoke --chaos-report`: normalized expected-failure chaos contracts.
- `89/100` `session guard --machine-policy strict|warn|off`: deterministic CI exit behavior.
- `88/100` `session list --priority`: triage-ready ordering with priority score/label.
- `87/100` `session capture --semantic-delta`: meaning-level delta extraction with semantic cursor persistence.
- `86/100` `session route --cost-estimate`: per-step token/time estimates for runbook planning.

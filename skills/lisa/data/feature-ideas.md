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

# Lisa Context-Optimization Features (Implemented)

Scoring: LLM-context impact out of 100.

- `94/100` `session handoff --json-min`: compact transfer packet (`state`, `reason`, `nextAction`, `nextOffset`, summary).
- `92/100` `session preflight --auto-model`: probes/selects first supported model from candidate list.
- `91/100` `session monitor --until-state`: state-gated orchestration stop conditions.
- `90/100` `session capture --cursor-file`: persisted incremental capture cursors across runs.
- `89/100` `session route --goal nested|analysis|exec`: route recommender with mode/policy/model rationale.
- `88/100` `session tree --delta --json-min`: topology diff-only payload for low-token graph polling.
- `87/100` `session context-pack --for <session>`: token-budgeted context bundle for handoffs.
- `86/100` `session smoke --model <NAME>`: deterministic model pin on smoke spawn sessions.
- `84/100` `session guard --shared-tmux`: shared tmux risk detection before cleanup/kill-all.
- `83/100` `skills doctor`: installed-skill drift check against repo version + capability contract.

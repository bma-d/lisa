# AGENTS

## Flow and Runtime
- Use the repo's package manager/runtime; no swaps without approval.
- Use Codex background for long jobs; use tmux only when persistence/interaction is required.

## Critical Thinking
- Fix root causes, not band-aids.
- If unsure, read more code first; if still blocked, ask with short options.
- If instructions conflict, call it out and pick the safer path.
- If you see unexpected changes, assume another agent edited them; keep focused on your scope.

## tmux
- Use only when persistence or interaction is needed.
- Quick refs:
  - `tmux new -d -s codex-shell`
  - `tmux attach -t codex-shell`
  - `tmux list-sessions`
  - `tmux kill-session -t codex-shell`

## Frontend Aesthetics
- Avoid generic UI output.
- Use intentional typography and a clear visual palette.
- Prefer 1-2 meaningful motion moments over random micro-interactions.
- Add depth to backgrounds with gradients/patterns instead of flat defaults.

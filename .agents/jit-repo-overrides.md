# JIT Repo Overrides

Last Updated: 2026-02-14
Related Files: `AGENTS.md`, `src/AGENTS.md`, `CLAUDE.md`, `src/CLAUDE.md`

## Overview

Repository-specific constraints for JIT context maintenance.

## Overrides

- Keep `AGENTS.md` files index-only, no procedures.
- Max 50 lines for every `AGENTS.md`.
- No code fences in any `AGENTS.md`.
- Use root-level table headers: `File | Description | Read When`.
- Use one-line key-decision bullets; detail belongs in `.agents/*.md`.
- Keep `CLAUDE.md` as single line `@AGENTS.md`.
- Prefer flat `.agents/` topics; add nested `AGENTS.md` only when nesting.

## Related Context

- @AGENTS.md
- @.agents/agentic-jit-framework.md

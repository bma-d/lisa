# Agent Implementation Protocol

Last Updated: 2026-02-14
Related Files: `AGENTS.md`, `.agents/agentic-jit-framework.md`, `.breadcrumbs/`

## Overview

Mandatory preflight/postflight protocol for ALL implementation tasks. Agents MUST consume context before coding and update context after changes.

---

## PREFLIGHT: Context Consumption (MANDATORY)

### Before ANY implementation, you MUST:

1. **Read the nearest AGENTS.md** in or above your working directory
2. **Follow ALL references** listed in that AGENTS.md:
   - If `project-overview.md` is listed → READ IT
   - If nested `AGENTS.md` exists for your area → READ IT
   - If specific topic files match your task → READ THEM
3. **Cascade upward**: If local AGENTS.md references parent context, follow those too
4. **Log what you consumed**: Note which context files informed your approach

### Context Consumption Rules

| Working In | MUST Read | Should Read |
|------------|-----------|-------------|
| `frontend/` | `frontend/AGENTS.md`, root `AGENTS.md` | `project-overview.md`, relevant `frontend/.agents/*.md` |
| `backend/` | `backend/AGENTS.md`, root `AGENTS.md` | `project-overview.md`, relevant `backend/.agents/*.md` |
| Root/cross-cutting | Root `AGENTS.md`, `project-overview.md` | Area-specific AGENTS.md if touching those dirs |
| Any implementation | `project-overview.md` | Tech stack, patterns, conventions |

### Why This Matters

- Context files contain architectural decisions, patterns, conventions
- Skipping context leads to inconsistent implementations
- 2 minutes of reading saves 20 minutes of rework
- You cannot claim "I didn't know" if context was available

### Preflight Checklist

Before writing ANY code:
- [ ] Read nearest AGENTS.md
- [ ] Read `project-overview.md` (almost always relevant)
- [ ] Read topic-specific `.agents/*.md` files for your task
- [ ] Identify patterns/conventions that apply
- [ ] Note any unclear areas to ask about

---

## POSTFLIGHT: Context Updates (MANDATORY)

### When to Run

- End of task before final response or handoff
- After changes that: add/remove files, change APIs/contracts, introduce new patterns, change config/runtime, or alter user-visible behavior

### Inputs

- Latest `.breadcrumbs/*.txt` entries for this task
- Any changelog entry or PR summary you wrote
- `git diff --name-only` or `git status -sb` to map touched paths
- If no breadcrumbs exist, use `git diff --stat` as primary input

### Retroactive Context Sync (RCS) Steps

1. **Summarize** changes from breadcrumbs/changelog into 3-7 bullets
2. **Map** each bullet to the nearest `AGENTS.md` (same dir or parent)
3. **Decide** if context update is required:
   - **Required**: new file/dir, API/schema change, new dependency, new workflow, new convention
   - **Optional**: internal refactor that changes "how" but not "what" (update only if guidance becomes misleading)
   - **Skip**: typo fixes, simple bugfixes without new patterns
4. **Apply** minimal edits:
   - Update existing `.agents/*.md` with new behavior/patterns
   - Update `AGENTS.md` index if files are added/removed or a new topic is created
   - Run a quick index sync check: every `.agents/*.md` you touched appears in the nearest `AGENTS.md`
   - Keep `AGENTS.md` index-only and <50 lines
   - Update the `Last Updated` line in any modified `.agents` file
5. **Log** a breadcrumb that context was refreshed

### Scope Hints

| Touched Path | Update Target |
|--------------|---------------|
| `frontend/...` | `frontend/AGENTS.md` or nearest parent |
| `backend/...` | `backend/AGENTS.md` or nearest parent |
| Tenancy/DB schema | Root `AGENTS.md` key decisions or backend context |
| Overrides conflict | Re-check `jit-repo-overrides.md` before changing formats |

### Guardrails

- Timebox to ~5 minutes
- Avoid copying diff/code; write "what changed and why" only
- Do not create new context files unless 3+ distinct subtopics exist
- If unsure, add a short breadcrumb note in the relevant `.agents` file rather than expanding scope
- If timeboxed and still uncertain, note possible staleness in the breadcrumb and move on

### Postflight Checklist

Before marking task complete:
- [ ] Breadcrumb logged for code changes
- [ ] Context files updated if warranted
- [ ] `Last Updated` dates refreshed
- [ ] AGENTS.md index updated if files added/removed
- [ ] Breadcrumb logged for context refresh

---

## Anti-Patterns (DO NOT)

- ❌ Start implementing without reading context
- ❌ Read AGENTS.md but ignore the files it references
- ❌ Assume you know the patterns without verifying
- ❌ Complete a task without checking if context needs updates
- ❌ Skip postflight because "it was a small change"
- ❌ Create new patterns without documenting them

---

## Implementation Quality Gate

Your implementation is NOT complete until:
1. Preflight context was consumed and informed your approach
2. Code is written following discovered patterns
3. Postflight context sync was performed
4. Breadcrumbs were logged

---

## Sub-Agent Directive

When spawning sub-agents for implementation tasks, include this preamble:

```
CONTEXT PROTOCOL: Before implementing, you MUST:
1. Read the nearest AGENTS.md
2. Read project-overview.md from .agents/
3. Read any topic-specific .agents/*.md files relevant to your task
4. Follow patterns and conventions discovered in context

After implementing:
1. Log breadcrumbs for changes
2. Update relevant .agents/*.md if you introduced new patterns
3. Verify AGENTS.md index is current
```

---

## Maintenance

Revisit this protocol after major repo restructuring or quarterly.

## Related Context

- @.agents/agentic-jit-framework.md
- @.agents/jit-repo-overrides.md
- @AGENTS.md

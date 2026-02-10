# Agentic JIT Framework

**Sub-Agent Context Preamble** - Prepend to ALL sub-agent tasks.

Updated: 2026-01-30

## Overview

JIT context injection framework for AI agents. **Never load all context upfront.** Index → Evaluate → Cherry-pick.

## Core Rules

### 1. AGENTS.md (Index Only)
```markdown
# [Name] Context

[1-2 sentence purpose]

## Context Files

| File | Description | Read When |
|------|-------------|-----------|
| `.agents/topic.md` | What it covers | When to read |
| `.agents/nested/` | Category | → See @.agents/nested/AGENTS.md} |

## Key Decisions
- [Brief bullet only]
```
- Max 50 lines
- NO detailed content
- NO code examples

### 2. CLAUDE.md
Single line only:
```
@AGENTS.md
```
Place `CLAUDE.md` alongside the target `AGENTS.md` (same directory).

### 3. .agents/ Structure
```
.agents/
├── flat-topic.md          # Listed directly in parent
└── nested-topic/          # Has own AGENTS.md
    ├── AGENTS.md          # Lists siblings only
    └── subtopic.md
```
- Max 2 levels deep
- Nested folders MUST have AGENTS.md
- Parent references folder, NOT contents

### 4. .agents/*.md Format
```markdown
# Topic

Last Updated: [Date]
Related Files: `file1.ts`, `file2.ts`

## Overview
[Brief]

## Details
[Comprehensive]

## Examples
[If needed]

## Related Context
- @path/to/AGENTS.md}
```

### 5. Reference Syntax
```
@path-from-project-root}/file.md
```

### 6. Breadcrumbs (MANDATORY)
After ANY code change:
```bash
python .breadcrumbs/add-breadcrumb.py "Description" "Details"
python .breadcrumbs/add-breadcrumb.py --file path/file.ts "Description"
```

## Agent Behavior

1. **Read** AGENTS.md on directory entry
2. **Evaluate** Description + ReadWhen columns
3. **Cherry-pick** ONLY relevant .agents/ files
4. **Follow** nested AGENTS.md for folders
5. **Never** load all context upfront

## Update Rules

**Always update**: New/delete files, API changes, new patterns, architecture
**Discretion**: Internal changes, bugfixes, refactors

**Rules**:
1. Update same/nearest parent level
2. Minimal changes only
3. Propagate up if parent affected
4. 3+ subtopics → nest with own AGENTS.md
5. Log ALL changes via breadcrumb

## Verification Codes

| Code | Issue |
|------|-------|
| MISSING_AGENTS | No AGENTS.md in code dir |
| MISSING_CLAUDE | No CLAUDE.md |
| INVALID_CLAUDE | Bad format |
| WRONG_REF | Incorrect path |
| MISSING_NESTED_AGENTS | Nested folder lacks AGENTS.md |
| DEPTH_VIOLATION | .agents/ >2 levels |
| PARENT_LISTS_NESTED | Parent lists nested file |
| ORPHANED | File not in index |
| CONTENT_IN_INDEX | AGENTS.md >50 lines |

## Sub-Agent Task Template

````markdown
## Task: [Implement/Verify] Context Framework

### Framework Context
[Copy this entire file's content here]

### Assignment
Directories: [list]
Mode: [docs-only|exhaustive] or [strict|auto-fix]

### Steps
1. [Specific instructions]

### Report
```
DIRECTORY: path/
  STATUS: PASS|ISSUES
  Created/Fixed: [list]
  Issues: [list]
  Manual Review: [list]
```
````

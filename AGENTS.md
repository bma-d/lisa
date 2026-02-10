# Context Management Framework

JIT context injection — agents load only needed context, never everything upfront.

**Key Insight**: Read index -> Evaluate relevance -> Cherry-pick specific files.

## Core Principles

1. **JIT Loading**: Context files are pointers. Load per task requirement.
2. **Hierarchical Inheritance**: Deeper folders inherit parent AGENTS.md context.
3. **Agent-Driven Selection**: Evaluate descriptions, choose from `.agents/`—never load all.
4. **Breadcrumb Logging**: Log changes via `python3 .breadcrumbs/add-breadcrumb.py "Description" "Details"`

## Agent Behavior

1. **Read AGENTS.md** on directory entry
2. **Evaluate** Description/ReadWhen columns
3. **Cherry-pick** only relevant `.agents/` files
4. **Follow** nested AGENTS.md for subdirectories

Reference syntax: `@path/file.md`

---

## Project Overview

**Lisa** — standalone Go CLI for orchestrating Claude/Codex AI agent sessions inside tmux. Zero external dependencies. Infrastructure for LLM orchestrators that manage concurrent AI workers.

Commands: `doctor` | `session {name,spawn,send,status,monitor,capture,list,exists,kill,kill-all}` | `agent build-cmd`

## Context Files

| File | Description | Read When |
|------|-------------|-----------|
| `.agents/project-overview.md` | File structure, tech stack, key decisions, breadcrumb usage | Onboarding or architectural questions |
| `.agents/architecture.md` | State machine, session lifecycle, tmux integration | Understanding system design |
| `.agents/conventions.md` | Go patterns, build/test, agent guidelines | Writing or reviewing code |
| `.agents/sdk-usage.md` | CLI usage, integration patterns, exit codes | Using Lisa as orchestrator |
| `.agents/agent-implementation-protocol.md` | Mandatory preflight/postflight rules | ANY implementation task |
| `.agents/agentic-jit-framework.md` | JIT context injection rules | Managing context files |

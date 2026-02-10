# Project Overview

Last Updated: 2026-02-10

## Technology Stack

- Go 1.21 (stdlib only)
- tmux (session management runtime)
- Claude Code / OpenAI Codex (agent targets)

## File Structure

```
lisa/
├── AGENTS.md              # Root index (framework + project overview)
├── CLAUDE.md              # → @AGENTS.md
├── README.md              # User-facing usage + install methods
├── .goreleaser.yaml       # Multi-channel release packaging
├── .github/workflows/     # CI + release automation
├── .breadcrumbs/          # Change tracking
│   ├── add-breadcrumb.py
│   └── YYMMDD.md
├── .agents/               # Detailed root context
│   ├── architecture.md
│   ├── conventions.md
│   ├── project-overview.md
│   └── sdk-usage.md
└── src/
    ├── AGENTS.md
    ├── CLAUDE.md
    └── .agents/
```

## Key Decisions

- Zero external deps — stdlib only for portability
- Function variable pattern for test mocking (`var tmuxFooFn = tmuxFoo`)
- Hand-rolled flag parsing (no flag library)
- All commands support `--json` for machine consumption
- Session artifacts in `/tmp/` keyed by project hash
- Release artifacts and package-manager distribution are handled via GoReleaser (Homebrew, Scoop, deb/rpm/apk, archives)

## Breadcrumb System

```bash
python3 .breadcrumbs/add-breadcrumb.py "Description" "Details"
python3 .breadcrumbs/add-breadcrumb.py --file path/file.go "Description" "Details"
```

## Related Context

- @AGENTS.md

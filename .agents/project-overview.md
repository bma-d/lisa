# Project Overview

Last Updated: 2026-02-21

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
├── smoke-nested           # Repo-local nested tmux smoke command
├── scripts/
│   └── smoke-nested-3level.sh
├── skills/
│   └── lisa/
│       └── SKILL.md        # vendored Lisa skill for Codex/Claude installs
└── src/
    ├── AGENTS.md
    ├── CLAUDE.md
    └── .agents/
```

## Key Decisions

- Zero external deps — stdlib only for portability
- Function variable pattern for test mocking (`var tmuxFooFn = tmuxFoo`)
- Hand-rolled flag parsing (no flag library)
- Machine-readable `--json` is available on `doctor`, `agent build-cmd`, and all major session commands (`name|spawn|send|status|explain|monitor|capture|tree|smoke|list|exists|kill|kill-all`)
- Session artifacts in `/tmp/` keyed by project hash
- `skills` command manages bidirectional Lisa skill sync/install (`skills sync`, `skills install`)
- Release artifacts and package-manager distribution are handled via GoReleaser (Homebrew, deb/rpm/apk, archives)

## Breadcrumb System

```bash
python3 .breadcrumbs/add-breadcrumb.py "Description" "Details"
python3 .breadcrumbs/add-breadcrumb.py --file path/file.go "Description" "Details"
```

## Related Context

- @AGENTS.md

# Latest Repository Scan Handoff

Last updated: 2026-02-15
Repo: `/Users/joon/projects/tools/lisa`
Branch: `main`
Purpose: handoff doc so next agent can continue without rescanning.

## 1) What was requested + what was done

- User asked for exhaustive scan + comprehensive overview.
- Full repository scan performed (source, tests, docs, CI/release, context framework, BMAD assets).
- Drift identified in generated docs vs actual code/runtime behavior.
- Drift remediation completed for current docs and SDK guide.

## 2) Current repository state (critical)

### Git status at handoff

Uncommitted changes currently present:

- Modified:
  - `/Users/joon/projects/tools/lisa/agent.md`
  - `/Users/joon/projects/tools/lisa/docs/architecture.md`
  - `/Users/joon/projects/tools/lisa/docs/development-guide.md`
  - `/Users/joon/projects/tools/lisa/docs/index.md`
  - `/Users/joon/projects/tools/lisa/docs/project-overview.md`
  - `/Users/joon/projects/tools/lisa/docs/project-scan-report.json`
  - `/Users/joon/projects/tools/lisa/docs/source-tree-analysis.md`
- New:
  - `/Users/joon/projects/tools/lisa/docs/changelog/260215.md`
  - `/Users/joon/projects/tools/lisa/.breadcrumbs/260215.md`

No code changes were made in `src/` during drift-fix pass.

## 3) High-confidence project summary

- Product: standalone Go CLI for orchestrating Claude/Codex sessions in tmux.
- Entrypoint: `/Users/joon/projects/tools/lisa/main.go` -> `/Users/joon/projects/tools/lisa/src/run.go`.
- Module: `github.com/bma-d/lisa`, Go 1.21, stdlib-only runtime dependencies.
- Runtime external dependencies: `tmux`, `claude` and/or `codex` binaries on PATH.
- Platform target: macOS/Linux.

## 4) Repository structure snapshot

Top-level important dirs/files:

- `/Users/joon/projects/tools/lisa/src` (app code + tests)
- `/Users/joon/projects/tools/lisa/docs` (human docs/changelogs)
- `/Users/joon/projects/tools/lisa/.agents` and `/Users/joon/projects/tools/lisa/src/.agents` (JIT context docs)
- `/Users/joon/projects/tools/lisa/.github/workflows` (CI + release)
- `/Users/joon/projects/tools/lisa/_bmad` (large BMAD workflow corpus)
- `/Users/joon/projects/tools/lisa/_bmad-output` (empty directories; no artifacts currently)

File counts from scan:

- `rg --files | wc -l` -> `496` tracked files.
- Dominant tracked subtree: `_bmad` (`443` files).
- `src` tracked files: `35` (includes `AGENTS.md`, `CLAUDE.md`, Go files).

## 5) `src/` inventory (live)

- Go files total: `33`
- Non-test Go files: `19`
- Test Go files: `14`
- `src/*.go` LOC total: `12,450`
- Non-test function count (`^func`): `369`
- Test count (`^func Test`): `184`

Largest files:

- `/Users/joon/projects/tools/lisa/src/regressions_test.go` (2646)
- `/Users/joon/projects/tools/lisa/src/session_wrapper_test.go` (1217)
- `/Users/joon/projects/tools/lisa/src/claude_session_test.go` (579)
- `/Users/joon/projects/tools/lisa/src/hardening_test.go` (567)
- `/Users/joon/projects/tools/lisa/src/tmux.go` (534)

## 6) Command surface (current)

Top-level router in `/Users/joon/projects/tools/lisa/src/run.go`:

- `doctor`
- `version`
- `session`
- `agent`
- `help`

`session` subcommands in `/Users/joon/projects/tools/lisa/src/commands_session.go` + split files:

- `name`
- `spawn`
- `send`
- `status`
- `explain`
- `monitor`
- `capture`
- `list`
- `exists`
- `kill`
- `kill-all`

Agent commands:

- `agent build-cmd`

Help system:

- centralized in `/Users/joon/projects/tools/lisa/src/help.go`
- per-command `--help` / `-h` supported across handlers.

## 7) Runtime architecture (source-of-truth behavior)

Primary source: `/Users/joon/projects/tools/lisa/src/status.go` and related helpers.

### Session classification model

- Current model is process-first.
- Signals prioritized from infra/process/pane state, not transcript/prompt text by default.
- Main decisive signals:
  - pane terminal status (`exited:*`, `crashed:*`)
  - done sidecar (`.done.txt`) with run-id matching
  - process detection (`detectAgentProcess`)
  - heartbeat freshness
  - shell vs non-shell pane command classification
  - lock/timeouts/read-error degradations

States observed in code/payload model:

- `just_started`
- `in_progress`
- `completed`
- `crashed`
- `stuck`
- `degraded`
- `not_found`
- `waiting_input` remains in compatibility model, currently non-emitting by default classifier path.

### Wrapper + done/heartbeat

Startup command wrapping in `/Users/joon/projects/tools/lisa/src/session_wrapper.go` adds:

- run-id start/done markers
- heartbeat loop touching heartbeat file
- done sidecar write on exit trap
- signal trap handling

### Artifact paths

Managed in `/Users/joon/projects/tools/lisa/src/session_files.go`:

- hash-scoped under `/tmp`, keyed by canonicalized project hash + session artifact id.
- meta/state/output/heartbeat/done/events + lock/count sidecars + command scripts.

### Observability

`/Users/joon/projects/tools/lisa/src/session_observability.go`:

- event JSONL append + trim
- shared/exclusive file locks
- state lock timeout support
- bounded events by bytes/lines
- stale event retention pruning

## 8) Transcript behavior (current)

Files:

- `/Users/joon/projects/tools/lisa/src/claude_session.go`
- `/Users/joon/projects/tools/lisa/src/codex_session.go`
- `/Users/joon/projects/tools/lisa/src/commands_session_state.go`

Behavior:

- `session capture` default tries transcript path for Claude sessions (based on resolved metadata).
- If transcript capture fails, command falls back to raw tmux pane capture.
- `--raw` forces raw pane capture.

Note: old docs/changelog mention `--transcript`; current CLI help in code does not expose `--transcript` flag anymore.

## 9) Key environment variables found in non-test code

From code scan of `src/*.go`:

- `LISA_CMD_TIMEOUT_SECONDS`
- `LISA_OUTPUT_STALE_SECONDS`
- `LISA_HEARTBEAT_STALE_SECONDS`
- `LISA_PROCESS_SCAN_INTERVAL_SECONDS`
- `LISA_PROCESS_LIST_CACHE_MS`
- `LISA_STATE_LOCK_TIMEOUT_MS`
- `LISA_EVENT_LOCK_TIMEOUT_MS`
- `LISA_EVENTS_MAX_BYTES`
- `LISA_EVENTS_MAX_LINES`
- `LISA_EVENT_RETENTION_DAYS`
- `LISA_CLEANUP_ALL_HASHES`
- `LISA_AGENT_PROCESS_MATCH`
- `LISA_AGENT_PROCESS_MATCH_CLAUDE`
- `LISA_AGENT_PROCESS_MATCH_CODEX`
- tmux/env propagation keys: `LISA_SESSION`, `LISA_SESSION_NAME`, `LISA_AGENT`, `LISA_MODE`, `LISA_PROJECT_HASH`, `LISA_HEARTBEAT_FILE`, `LISA_DONE_FILE`

## 10) Test posture + latest execution

Executed during scan:

- `go test ./...` -> pass
- `go test -race ./...` -> pass
- `go test -cover ./src` -> pass (`79.9%` statements)

CI files reviewed:

- `/Users/joon/projects/tools/lisa/.github/workflows/ci.yml`
- `/Users/joon/projects/tools/lisa/.github/workflows/release.yml`

CI includes:

- standard tests
- race tests
- hermetic interactive e2e test
- optional real-agent e2e (gated by repo variable)

## 11) Release/distribution pipeline

- `/Users/joon/projects/tools/lisa/.goreleaser.yaml`
- Builds darwin/linux, amd64/arm64.
- Artifacts: archives + checksums + Homebrew formula + `deb/rpm/apk` packages.

## 12) Context framework and process scaffolding

JIT context docs consumed:

- `/Users/joon/projects/tools/lisa/AGENTS.md`
- `/Users/joon/projects/tools/lisa/.agents/*`
- `/Users/joon/projects/tools/lisa/src/AGENTS.md`
- `/Users/joon/projects/tools/lisa/src/.agents/*`

BMAD corpus snapshot:

- `/Users/joon/projects/tools/lisa/_bmad` contains many workflows/agent/task manifests.
- `/Users/joon/projects/tools/lisa/_bmad-output` currently contains only empty artifact directories.

## 13) Drift that was found and addressed

### Identified drift classes

- stale file counts/LOC in docs
- stale architecture narrative (older prompt/output-driven framing)
- stale state descriptions inconsistent with process-first refactor
- stale SDK text around state behavior
- stale machine report (`docs/project-scan-report.json` from older scan)

### Files rewritten/updated to reconcile drift

- `/Users/joon/projects/tools/lisa/docs/index.md`
- `/Users/joon/projects/tools/lisa/docs/project-overview.md`
- `/Users/joon/projects/tools/lisa/docs/architecture.md`
- `/Users/joon/projects/tools/lisa/docs/development-guide.md`
- `/Users/joon/projects/tools/lisa/docs/source-tree-analysis.md`
- `/Users/joon/projects/tools/lisa/docs/project-scan-report.json`
- `/Users/joon/projects/tools/lisa/agent.md`
- `/Users/joon/projects/tools/lisa/docs/changelog/260215.md` (new entry)
- `/Users/joon/projects/tools/lisa/.breadcrumbs/260215.md` (breadcrumb)

## 14) Remaining known caveats (important for next agent)

- Historical changelog entries in older files intentionally kept unchanged; they may mention features/flags no longer current. Treat as historical timeline, not active spec.
- Root runtime source of truth remains code + `.agents` context docs.
- If continuing work, verify no new drift between rewritten docs and code before commit (especially after any code edits).

## 15) Recommended continuation checklist

1. Re-run quick validation before commit:
   - `go test ./...`
   - `go test -race ./...`
   - `go test -cover ./src`
2. Review `git diff` for doc wording accuracy and style consistency.
3. If requested, commit with conventional message (likely `docs: reconcile latest scan drift`).
4. If additional drift scope requested, include README/changelog historical normalization strategy explicitly (decide whether to rewrite historical changelog lines or add “historical note” section).

## 16) Command outputs captured during this scan (key facts)

- `rg --files | wc -l` -> `496`
- `ls src/*.go | rg -v '_test\\.go$' | wc -l` -> `19`
- `ls src/*_test.go | wc -l` -> `14`
- `rg -n '^func ' src/*.go | wc -l` -> `369`
- `rg -n '^func Test' src/*_test.go | wc -l` -> `184`
- `go test -cover ./src` -> `coverage: 79.9% of statements`


package app

import (
	"fmt"
	"os"
)

func isHelpFlag(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

var helpFuncs = map[string]func(){
	"":                 helpTop,
	"doctor":           helpDoctor,
	"cleanup":          helpCleanup,
	"session":          helpSession,
	"session name":     helpSessionName,
	"session spawn":    helpSessionSpawn,
	"session send":     helpSessionSend,
	"session status":   helpSessionStatus,
	"session explain":  helpSessionExplain,
	"session monitor":  helpSessionMonitor,
	"session capture":  helpSessionCapture,
	"session tree":     helpSessionTree,
	"session list":     helpSessionList,
	"session exists":   helpSessionExists,
	"session kill":     helpSessionKill,
	"session kill-all": helpSessionKillAll,
	"agent":            helpAgent,
	"agent build-cmd":  helpAgentBuildCmd,
	"skills":           helpSkills,
	"skills sync":      helpSkillsSync,
	"skills install":   helpSkillsInstall,
}

func showHelp(cmdPath string) int {
	fn, ok := helpFuncs[cmdPath]
	if !ok {
		fmt.Fprintf(os.Stderr, "no help available for %q\n", cmdPath)
		return 1
	}
	fn()
	return 0
}

func helpTop() {
	fmt.Fprintln(os.Stderr, "lisa <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  doctor               Check prerequisites (tmux, claude, codex)")
	fmt.Fprintln(os.Stderr, "  cleanup              Clean stale tmux socket residue")
	fmt.Fprintln(os.Stderr, "  version              Print version info")
	fmt.Fprintln(os.Stderr, "  session name          Generate unique session name")
	fmt.Fprintln(os.Stderr, "  session spawn         Create and start an agent session")
	fmt.Fprintln(os.Stderr, "  session send          Send text or keys to a running session")
	fmt.Fprintln(os.Stderr, "  session status        Get current session status")
	fmt.Fprintln(os.Stderr, "  session explain       Detailed session diagnostics")
	fmt.Fprintln(os.Stderr, "  session monitor       Poll session until terminal state")
	fmt.Fprintln(os.Stderr, "  session capture       Capture session pane output or transcript")
	fmt.Fprintln(os.Stderr, "  session tree          Show parent/child session tree")
	fmt.Fprintln(os.Stderr, "  session list          List lisa sessions")
	fmt.Fprintln(os.Stderr, "  session exists        Check if a session exists")
	fmt.Fprintln(os.Stderr, "  session kill          Kill a session and clean artifacts")
	fmt.Fprintln(os.Stderr, "  session kill-all      Kill all lisa sessions")
	fmt.Fprintln(os.Stderr, "  agent build-cmd       Build agent CLI command string")
	fmt.Fprintln(os.Stderr, "  skills sync           Sync lisa skill into repo skills/lisa")
	fmt.Fprintln(os.Stderr, "  skills install        Install repo lisa skill to codex/claude/project")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Run 'lisa <command> --help' for details on a specific command.")
}

func helpDoctor() {
	fmt.Fprintln(os.Stderr, "lisa doctor — check prerequisites")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa doctor [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --json    JSON output")
}

func helpCleanup() {
	fmt.Fprintln(os.Stderr, "lisa cleanup — clean stale tmux socket residue")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa cleanup [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --dry-run                Show what would be removed/killed without mutating")
	fmt.Fprintln(os.Stderr, "  --include-tmux-default   Also sweep tmux default sockets (/tmp/tmux-*)")
	fmt.Fprintln(os.Stderr, "  --json                   JSON output")
}

func helpSession() {
	fmt.Fprintln(os.Stderr, "lisa session — manage agent sessions")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  name       Generate unique session name")
	fmt.Fprintln(os.Stderr, "  spawn      Create and start an agent session")
	fmt.Fprintln(os.Stderr, "  send       Send text or keys to a running session")
	fmt.Fprintln(os.Stderr, "  status     Get current session status")
	fmt.Fprintln(os.Stderr, "  explain    Detailed session diagnostics")
	fmt.Fprintln(os.Stderr, "  monitor    Poll session until terminal state")
	fmt.Fprintln(os.Stderr, "  capture    Capture session pane output or transcript")
	fmt.Fprintln(os.Stderr, "  tree       Show parent/child session tree")
	fmt.Fprintln(os.Stderr, "  list       List lisa sessions")
	fmt.Fprintln(os.Stderr, "  exists     Check if a session exists")
	fmt.Fprintln(os.Stderr, "  kill       Kill a session and clean artifacts")
	fmt.Fprintln(os.Stderr, "  kill-all   Kill all lisa sessions")
}

func helpSessionName() {
	fmt.Fprintln(os.Stderr, "lisa session name — generate unique session name")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session name [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --agent NAME          AI agent: claude|codex (default: claude)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Session mode: interactive|exec (default: interactive)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --tag TEXT            Tag to include in name")
}

func helpSessionSpawn() {
	fmt.Fprintln(os.Stderr, "lisa session spawn — create and start an agent session")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session spawn [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --agent NAME          AI agent: claude|codex (default: claude)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Session mode: interactive|exec (default: interactive)")
	fmt.Fprintln(os.Stderr, "  --session NAME        Override session name (must start with \"lisa-\")")
	fmt.Fprintln(os.Stderr, "  --prompt TEXT          Initial prompt for the agent")
	fmt.Fprintln(os.Stderr, "  --command TEXT         Custom command instead of agent CLI")
	fmt.Fprintln(os.Stderr, "  --agent-args TEXT      Extra args passed to agent CLI")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory for isolation (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --width N             Tmux pane width (default: 220)")
	fmt.Fprintln(os.Stderr, "  --height N            Tmux pane height (default: 60)")
	fmt.Fprintln(os.Stderr, "  --cleanup-all-hashes  Clean artifacts across all project hashes")
	fmt.Fprintln(os.Stderr, "  --dry-run             Print resolved spawn plan without creating session")
	fmt.Fprintln(os.Stderr, "  --no-dangerously-skip-permissions")
	fmt.Fprintln(os.Stderr, "                        Don't add --dangerously-skip-permissions to claude")
	fmt.Fprintln(os.Stderr, "  note                  Nested codex exec prompts (./lisa, lisa session spawn)")
	fmt.Fprintln(os.Stderr, "                        auto-enable '--dangerously-bypass-approvals-and-sandbox'")
	fmt.Fprintln(os.Stderr, "                        and omit --full-auto")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionSend() {
	fmt.Fprintln(os.Stderr, "lisa session send — send text or keys to a running session")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session send [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --text TEXT            Text to send (mutually exclusive with --keys)")
	fmt.Fprintln(os.Stderr, "  --keys \"KEYS...\"      Tmux keys to send (mutually exclusive with --text)")
	fmt.Fprintln(os.Stderr, "  --enter               Press Enter after sending")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionStatus() {
	fmt.Fprintln(os.Stderr, "lisa session status — get current session status")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session status [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --agent NAME          Agent hint: auto|claude|codex (default: auto)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Mode hint: auto|interactive|exec (default: auto)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --full                Include classification/signal columns")
	fmt.Fprintln(os.Stderr, "  --fail-not-found      Exit 1 when session resolves to not_found")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionExplain() {
	fmt.Fprintln(os.Stderr, "lisa session explain — detailed session diagnostics")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session explain [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --agent NAME          Agent hint: auto|claude|codex (default: auto)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Mode hint: auto|interactive|exec (default: auto)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --events N            Number of recent events to show (default: 10)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionMonitor() {
	fmt.Fprintln(os.Stderr, "lisa session monitor — poll session until terminal state")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session monitor [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --agent NAME          Agent hint: auto|claude|codex (default: auto)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Mode hint: auto|interactive|exec (default: auto)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --poll-interval N     Seconds between polls (default: 30)")
	fmt.Fprintln(os.Stderr, "  --max-polls N         Maximum number of polls (default: 120)")
	fmt.Fprintln(os.Stderr, "  --stop-on-waiting BOOL  Stop on waiting_input (default: true)")
	fmt.Fprintln(os.Stderr, "  --waiting-requires-turn-complete BOOL  Require transcript turn-complete before stopping on waiting_input (default: false)")
	fmt.Fprintln(os.Stderr, "  --until-marker TEXT   Stop when raw pane output contains marker text")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
	fmt.Fprintln(os.Stderr, "  --verbose             Print poll details to stderr")
}

func helpSessionCapture() {
	fmt.Fprintln(os.Stderr, "lisa session capture — capture session pane output or transcript")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session capture [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --raw                 Raw tmux pane capture instead of transcript")
	fmt.Fprintln(os.Stderr, "  --keep-noise          Keep Codex/MCP startup noise in pane capture")
	fmt.Fprintln(os.Stderr, "  --strip-noise         Compatibility alias for default noise filtering")
	fmt.Fprintln(os.Stderr, "  --lines N             Number of pane lines for raw capture (default: 200)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionList() {
	fmt.Fprintln(os.Stderr, "lisa session list — list lisa sessions")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session list [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --all-sockets         Discover active sessions across project sockets")
	fmt.Fprintln(os.Stderr, "  --project-only        Only show sessions for current project")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
}

func helpSessionTree() {
	fmt.Fprintln(os.Stderr, "lisa session tree — show parent/child session hierarchy")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session tree [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Root session filter (optional)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --all-hashes          Include metadata from all project hashes")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionExists() {
	fmt.Fprintln(os.Stderr, "lisa session exists — check if a session exists")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session exists [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
}

func helpSessionKill() {
	fmt.Fprintln(os.Stderr, "lisa session kill — kill a session and clean artifacts")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session kill [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --cleanup-all-hashes  Clean artifacts across all project hashes")
}

func helpSessionKillAll() {
	fmt.Fprintln(os.Stderr, "lisa session kill-all — kill all lisa sessions")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session kill-all [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --project-only        Only kill sessions for current project")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --cleanup-all-hashes  Clean artifacts across all project hashes")
}

func helpAgent() {
	fmt.Fprintln(os.Stderr, "lisa agent — agent utilities")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa agent <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  build-cmd   Build agent CLI command string")
}

func helpAgentBuildCmd() {
	fmt.Fprintln(os.Stderr, "lisa agent build-cmd — build agent CLI command string")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa agent build-cmd [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --agent NAME          AI agent: claude|codex (default: claude)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Session mode: interactive|exec (default: interactive)")
	fmt.Fprintln(os.Stderr, "  --prompt TEXT          Prompt for the agent")
	fmt.Fprintln(os.Stderr, "  --agent-args TEXT      Extra args passed to agent CLI")
	fmt.Fprintln(os.Stderr, "  --no-dangerously-skip-permissions")
	fmt.Fprintln(os.Stderr, "                        Don't add --dangerously-skip-permissions to claude")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSkills() {
	fmt.Fprintln(os.Stderr, "lisa skills — manage lisa skill files")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa skills <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  sync      Copy skill from codex/claude/path into repo skills/lisa")
	fmt.Fprintln(os.Stderr, "  install   Copy repo skills/lisa into codex/claude/project path")
}

func helpSkillsSync() {
	fmt.Fprintln(os.Stderr, "lisa skills sync — sync external skill into repo skills/lisa")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa skills sync [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --from SOURCE         Source: codex|claude|path (default: codex)")
	fmt.Fprintln(os.Stderr, "  --path PATH           Source path when --from path")
	fmt.Fprintln(os.Stderr, "  --repo-root PATH      Repo root that contains skills/ (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSkillsInstall() {
	fmt.Fprintln(os.Stderr, "lisa skills install — install repo skills/lisa to destination")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa skills install [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --to TARGET           Destination: codex|claude|project (default: auto)")
	fmt.Fprintln(os.Stderr, "  --project-path PATH   Project root when --to project")
	fmt.Fprintln(os.Stderr, "  --path PATH           Explicit destination path override")
	fmt.Fprintln(os.Stderr, "  --repo-root PATH      Repo root that contains skills/ (default: cwd)")
	fmt.Fprintln(os.Stderr, "  note                  Auto mode installs to all available ~/.codex and")
	fmt.Fprintln(os.Stderr, "                        ~/.claude targets")
	fmt.Fprintln(os.Stderr, "  note                  Source is repo skills/lisa for dev builds; tagged")
	fmt.Fprintln(os.Stderr, "                        release builds fetch skill from GitHub by version")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

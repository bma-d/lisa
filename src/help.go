package app

import (
	"fmt"
	"os"
)

func isHelpFlag(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

var helpFuncs = map[string]func(){
	"":                      helpTop,
	"doctor":                helpDoctor,
	"cleanup":               helpCleanup,
	"session":               helpSession,
	"session name":          helpSessionName,
	"session spawn":         helpSessionSpawn,
	"session detect-nested": helpSessionDetectNested,
	"session send":          helpSessionSend,
	"session snapshot":      helpSessionSnapshot,
	"session status":        helpSessionStatus,
	"session explain":       helpSessionExplain,
	"session monitor":       helpSessionMonitor,
	"session capture":       helpSessionCapture,
	"session handoff":       helpSessionHandoff,
	"session context-pack":  helpSessionContextPack,
	"session route":         helpSessionRoute,
	"session guard":         helpSessionGuard,
	"session tree":          helpSessionTree,
	"session smoke":         helpSessionSmoke,
	"session preflight":     helpSessionPreflight,
	"session exists":        helpSessionExists,
	"session kill":          helpSessionKill,
	"session kill-all":      helpSessionKillAll,
	"agent":                 helpAgent,
	"agent build-cmd":       helpAgentBuildCmd,
	"skills":                helpSkills,
	"skills sync":           helpSkillsSync,
	"skills doctor":         helpSkillsDoctor,
	"session list":          helpSessionList,
	"capabilities":          helpCapabilities,
	"skills install":        helpSkillsInstall,
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
	fmt.Fprintln(os.Stderr, "  session detect-nested Inspect nested-codex bypass detection")
	fmt.Fprintln(os.Stderr, "  session send          Send text or keys to a running session")
	fmt.Fprintln(os.Stderr, "  session snapshot      One-shot status + capture + nextOffset")
	fmt.Fprintln(os.Stderr, "  session status        Get current session status")
	fmt.Fprintln(os.Stderr, "  session explain       Detailed session diagnostics")
	fmt.Fprintln(os.Stderr, "  session monitor       Poll session until terminal state")
	fmt.Fprintln(os.Stderr, "  session capture       Capture session pane output or transcript")
	fmt.Fprintln(os.Stderr, "  session handoff       Build compact handoff payload for another agent")
	fmt.Fprintln(os.Stderr, "  session context-pack  Build token-budgeted context packet")
	fmt.Fprintln(os.Stderr, "  session route         Recommend mode/policy/model for orchestration goal")
	fmt.Fprintln(os.Stderr, "  session guard         Shared-tmux safety guardrails")
	fmt.Fprintln(os.Stderr, "  session tree          Show parent/child session tree")
	fmt.Fprintln(os.Stderr, "  session smoke         Run deterministic nested smoke test")
	fmt.Fprintln(os.Stderr, "  session preflight     Validate env + contract assumptions")
	fmt.Fprintln(os.Stderr, "  session list          List lisa sessions")
	fmt.Fprintln(os.Stderr, "  session exists        Check if a session exists")
	fmt.Fprintln(os.Stderr, "  session kill          Kill a session and clean artifacts")
	fmt.Fprintln(os.Stderr, "  session kill-all      Kill all lisa sessions")
	fmt.Fprintln(os.Stderr, "  capabilities          Describe lisa CLI commands and flags")
	fmt.Fprintln(os.Stderr, "  agent build-cmd       Build agent CLI command string")
	fmt.Fprintln(os.Stderr, "  skills sync           Sync lisa skill into repo skills/lisa")
	fmt.Fprintln(os.Stderr, "  skills doctor         Verify installed lisa skill drift")
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
	fmt.Fprintln(os.Stderr, "  detect-nested Inspect nested-codex bypass detection")
	fmt.Fprintln(os.Stderr, "  send       Send text or keys to a running session")
	fmt.Fprintln(os.Stderr, "  snapshot   One-shot status + capture + nextOffset")
	fmt.Fprintln(os.Stderr, "  status     Get current session status")
	fmt.Fprintln(os.Stderr, "  explain    Detailed session diagnostics")
	fmt.Fprintln(os.Stderr, "  monitor    Poll session until terminal state")
	fmt.Fprintln(os.Stderr, "  capture    Capture session pane output or transcript")
	fmt.Fprintln(os.Stderr, "  handoff    Build compact handoff payload for another agent")
	fmt.Fprintln(os.Stderr, "  context-pack Build token-budgeted context packet")
	fmt.Fprintln(os.Stderr, "  route      Recommend mode/policy/model for orchestration goal")
	fmt.Fprintln(os.Stderr, "  guard      Shared-tmux safety guardrails")
	fmt.Fprintln(os.Stderr, "  tree       Show parent/child session tree")
	fmt.Fprintln(os.Stderr, "  smoke      Run deterministic nested smoke test")
	fmt.Fprintln(os.Stderr, "  preflight  Validate env + contract assumptions")
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
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionSpawn() {
	fmt.Fprintln(os.Stderr, "lisa session spawn — create and start an agent session")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session spawn [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --agent NAME          AI agent: claude|codex (default: claude)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Session mode: interactive|exec (default: interactive)")
	fmt.Fprintln(os.Stderr, "  --nested-policy MODE  Nested codex bypass policy: auto|force|off (default: auto)")
	fmt.Fprintln(os.Stderr, "  --nesting-intent MODE Nested intent override: auto|nested|neutral (default: auto)")
	fmt.Fprintln(os.Stderr, "  --session NAME        Override session name (must start with \"lisa-\")")
	fmt.Fprintln(os.Stderr, "  --prompt TEXT          Initial prompt for the agent")
	fmt.Fprintln(os.Stderr, "  --command TEXT         Custom command instead of agent CLI")
	fmt.Fprintln(os.Stderr, "  --agent-args TEXT      Extra args passed to agent CLI")
	fmt.Fprintln(os.Stderr, "  --model NAME           Codex model name (for --agent codex)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory for isolation (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --width N             Tmux pane width (default: 220)")
	fmt.Fprintln(os.Stderr, "  --height N            Tmux pane height (default: 60)")
	fmt.Fprintln(os.Stderr, "  --cleanup-all-hashes  Clean artifacts across all project hashes")
	fmt.Fprintln(os.Stderr, "  --dry-run             Print resolved spawn plan without creating session")
	fmt.Fprintln(os.Stderr, "  --detect-nested       Include nested-bypass detection diagnostics in JSON output")
	fmt.Fprintln(os.Stderr, "  --no-dangerously-skip-permissions")
	fmt.Fprintln(os.Stderr, "                        Don't add --dangerously-skip-permissions to claude")
	fmt.Fprintln(os.Stderr, "  note                  Nested codex exec prompts (./lisa, lisa session spawn)")
	fmt.Fprintln(os.Stderr, "                        auto-enable '--dangerously-bypass-approvals-and-sandbox'")
	fmt.Fprintln(os.Stderr, "                        and omit --full-auto")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionDetectNested() {
	fmt.Fprintln(os.Stderr, "lisa session detect-nested — inspect nested-codex bypass detection")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session detect-nested [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --agent NAME          AI agent: claude|codex (default: codex)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Session mode: interactive|exec (default: exec)")
	fmt.Fprintln(os.Stderr, "  --nested-policy MODE  Nested codex bypass policy: auto|force|off (default: auto)")
	fmt.Fprintln(os.Stderr, "  --nesting-intent MODE Nested intent override: auto|nested|neutral (default: auto)")
	fmt.Fprintln(os.Stderr, "  --prompt TEXT         Prompt text used for hint detection")
	fmt.Fprintln(os.Stderr, "  --agent-args TEXT     Existing agent args to evaluate")
	fmt.Fprintln(os.Stderr, "  --model NAME          Codex model name (for --agent codex)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory context (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --rewrite             Suggest trigger-safe prompt rewrites")
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
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON ack: session/ok")
}

func helpSessionSnapshot() {
	fmt.Fprintln(os.Stderr, "lisa session snapshot — one-shot status + raw capture + nextOffset")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session snapshot [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --agent NAME          Agent hint: auto|claude|codex (default: auto)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Mode hint: auto|interactive|exec (default: auto)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --lines N             Pane lines for capture (default: 200)")
	fmt.Fprintln(os.Stderr, "  --delta-from VALUE    Delta start: offset integer, @unix timestamp, or RFC3339")
	fmt.Fprintln(os.Stderr, "  --markers CSV         Marker-only extraction (comma-separated)")
	fmt.Fprintln(os.Stderr, "  --keep-noise          Keep Codex/MCP startup noise")
	fmt.Fprintln(os.Stderr, "  --strip-noise         Compatibility alias for default filtering")
	fmt.Fprintln(os.Stderr, "  --fail-not-found      Exit 1 when session resolves to not_found")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON output")
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
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON output: session/status/state/todos/wait")
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
	fmt.Fprintln(os.Stderr, "  --recent N            Alias for --events with compact intent")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON output: session/state/reason/recent events")
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
	fmt.Fprintln(os.Stderr, "  --until-state STATE   Stop when session state is reached")
	fmt.Fprintln(os.Stderr, "  --expect MODE         Success expectation: any|terminal|marker (default: any)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON output: session/finalState/exitReason/polls")
	fmt.Fprintln(os.Stderr, "  --stream-json         Emit line-delimited JSON poll events before final result")
	fmt.Fprintln(os.Stderr, "  --emit-handoff        Emit compact handoff JSON events on each poll (requires --stream-json)")
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
	fmt.Fprintln(os.Stderr, "  --delta-from VALUE    Delta start: offset integer, @unix timestamp, or RFC3339")
	fmt.Fprintln(os.Stderr, "  --cursor-file PATH    Persist/reuse delta offset cursor (raw capture only)")
	fmt.Fprintln(os.Stderr, "  --markers CSV         Marker-only extraction (comma-separated)")
	fmt.Fprintln(os.Stderr, "  --summary             Return bounded summary instead of full capture")
	fmt.Fprintln(os.Stderr, "  --token-budget N      Summary token budget (default: 320)")
	fmt.Fprintln(os.Stderr, "  --keep-noise          Keep Codex/MCP startup noise in pane capture")
	fmt.Fprintln(os.Stderr, "  --strip-noise         Compatibility alias for default noise filtering")
	fmt.Fprintln(os.Stderr, "  --lines N             Number of pane lines for raw capture (default: 200)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON output for compact polling workflows")
}

func helpSessionHandoff() {
	fmt.Fprintln(os.Stderr, "lisa session handoff — build compact handoff payload")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session handoff [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --agent NAME          Agent hint: auto|claude|codex (default: auto)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Mode hint: auto|interactive|exec (default: auto)")
	fmt.Fprintln(os.Stderr, "  --events N            Number of recent events to include (default: 8)")
	fmt.Fprintln(os.Stderr, "  --delta-from N        Incremental event offset (non-negative integer)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON output")
}

func helpSessionContextPack() {
	fmt.Fprintln(os.Stderr, "lisa session context-pack — build token-budgeted context packet")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session context-pack [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --for NAME            Session name (required; alias: --session)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --agent NAME          Agent hint: auto|claude|codex (default: auto)")
	fmt.Fprintln(os.Stderr, "  --mode MODE           Mode hint: auto|interactive|exec (default: auto)")
	fmt.Fprintln(os.Stderr, "  --events N            Number of recent events to include (default: 8)")
	fmt.Fprintln(os.Stderr, "  --lines N             Raw capture lines for context tail (default: 120)")
	fmt.Fprintln(os.Stderr, "  --token-budget N      Approx token budget for pack body (default: 700)")
	fmt.Fprintln(os.Stderr, "  --strategy MODE       Pack strategy: terse|balanced|full (default: balanced)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON output")
}

func helpSessionRoute() {
	fmt.Fprintln(os.Stderr, "lisa session route — recommend orchestration route")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session route [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --goal GOAL           Orchestration goal: nested|analysis|exec (default: analysis)")
	fmt.Fprintln(os.Stderr, "  --agent NAME          AI agent: claude|codex (default: codex)")
	fmt.Fprintln(os.Stderr, "  --prompt TEXT         Optional prompt override")
	fmt.Fprintln(os.Stderr, "  --model NAME          Optional codex model override")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory context (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --emit-runbook        Include executable runbook JSON steps")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionGuard() {
	fmt.Fprintln(os.Stderr, "lisa session guard — shared tmux safety guardrails")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session guard [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --shared-tmux         Validate safety assumptions for shared tmux environments")
	fmt.Fprintln(os.Stderr, "  --command TEXT        Optional command string to risk-check")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory context (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpSessionPreflight() {
	fmt.Fprintln(os.Stderr, "lisa session preflight — validate environment and command contracts")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session preflight [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --agent NAME          Optional model-probe agent (codex)")
	fmt.Fprintln(os.Stderr, "  --model NAME          Optional codex model probe")
	fmt.Fprintln(os.Stderr, "  --auto-model          Probe/select first supported model from candidate list")
	fmt.Fprintln(os.Stderr, "  --auto-model-candidates CSV")
	fmt.Fprintln(os.Stderr, "                        Candidate model list (default: gpt-5.3-codex,gpt-5-codex)")
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
	fmt.Fprintln(os.Stderr, "  --stale               Include stale metadata-only session counts/list")
	fmt.Fprintln(os.Stderr, "  --prune-preview       Include safe stale-session prune commands (requires --stale)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON output: sessions/count")
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
	fmt.Fprintln(os.Stderr, "  --active-only         Include only sessions currently active in tmux")
	fmt.Fprintln(os.Stderr, "  --delta               Output topology changes since previous tree snapshot")
	fmt.Fprintln(os.Stderr, "  --flat                Print machine-friendly parent/child rows")
	fmt.Fprintln(os.Stderr, "  --with-state          Enrich rows with status/sessionState snapshots")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
	fmt.Fprintln(os.Stderr, "  --json-min            Minimal JSON output: nodeCount + session graph")
}

func helpSessionSmoke() {
	fmt.Fprintln(os.Stderr, "lisa session smoke — deterministic nested lisa smoke test")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session smoke [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --levels N            Nested depth (1-4, default: 3)")
	fmt.Fprintln(os.Stderr, "  --prompt-style STYLE  Nested hint probe: none|dot-slash|spawn|nested|neutral")
	fmt.Fprintln(os.Stderr, "  --matrix-file PATH    Prompt matrix file: mode|prompt (mode=bypass|full-auto|any)")
	fmt.Fprintln(os.Stderr, "  --model NAME          Optional codex model pin for smoke spawn sessions")
	fmt.Fprintln(os.Stderr, "  --poll-interval N     Seconds between monitor polls (default: 1)")
	fmt.Fprintln(os.Stderr, "  --max-polls N         Maximum polls per nested monitor (default: 180)")
	fmt.Fprintln(os.Stderr, "  --keep-sessions       Keep spawned smoke sessions for inspection")
	fmt.Fprintln(os.Stderr, "  --report-min          Emit compact CI-focused JSON summary")
	fmt.Fprintln(os.Stderr, "  --json                JSON summary output")
}

func helpSessionExists() {
	fmt.Fprintln(os.Stderr, "lisa session exists — check if a session exists")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa session exists [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --session NAME        Session name (required)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
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
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
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
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

func helpCapabilities() {
	fmt.Fprintln(os.Stderr, "lisa capabilities — describe lisa CLI commands/flags")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa capabilities [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
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
	fmt.Fprintln(os.Stderr, "  --nested-policy MODE  Nested codex bypass policy: auto|force|off (default: auto)")
	fmt.Fprintln(os.Stderr, "  --nesting-intent MODE Nested intent override: auto|nested|neutral (default: auto)")
	fmt.Fprintln(os.Stderr, "  --project-root PATH   Project directory context (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --prompt TEXT          Prompt for the agent")
	fmt.Fprintln(os.Stderr, "  --agent-args TEXT      Extra args passed to agent CLI")
	fmt.Fprintln(os.Stderr, "  --model NAME           Codex model name (for --agent codex)")
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
	fmt.Fprintln(os.Stderr, "  doctor    Verify installed skill versions + command contract drift")
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

func helpSkillsDoctor() {
	fmt.Fprintln(os.Stderr, "lisa skills doctor — verify skill install drift and command contract")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: lisa skills doctor [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --repo-root PATH      Repo root that contains skills/ (default: cwd)")
	fmt.Fprintln(os.Stderr, "  --deep                Include recursive content-hash drift checks")
	fmt.Fprintln(os.Stderr, "  --json                JSON output")
}

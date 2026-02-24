package app

import (
	"fmt"
	"strings"
	"time"
)

// commandCapability describes a CLI subcommand and the flags it accepts.
type commandCapability struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Flags       []string `json:"flags"`
}

var commandCapabilities = []commandCapability{
	{
		Name:        "capabilities",
		Description: "List lisa commands and flags (use --json for structured data)",
		Flags:       []string{"--json"},
	},
	{
		Name:        "doctor",
		Description: "Check tmux + agent prerequisites",
		Flags:       []string{"--json"},
	},
	{
		Name:        "cleanup",
		Description: "Sweep stalled tmux sockets",
		Flags:       []string{"--dry-run", "--include-tmux-default", "--json"},
	},
	{
		Name:        "version",
		Description: "Print lisa version info",
		Flags:       []string{"--version", "-version", "-v"},
	},
	{
		Name:  "session name",
		Flags: []string{"--agent", "--mode", "--project-root", "--tag", "--json"},
	},
	{
		Name: "session spawn",
		Flags: []string{
			"--agent",
			"--mode",
			"--lane",
			"--nested-policy",
			"--nesting-intent",
			"--session",
			"--prompt",
			"--command",
			"--agent-args",
			"--model",
			"--project-root",
			"--width",
			"--height",
			"--cleanup-all-hashes",
			"--dry-run",
			"--detect-nested",
			"--no-dangerously-skip-permissions",
			"--json",
		},
	},
	{
		Name:  "session detect-nested",
		Flags: []string{"--agent", "--mode", "--nested-policy", "--nesting-intent", "--prompt", "--agent-args", "--model", "--project-root", "--rewrite", "--why", "--json"},
	},
	{
		Name:  "session send",
		Flags: []string{"--session", "--project-root", "--text", "--keys", "--enter", "--json", "--json-min"},
	},
	{
		Name: "session snapshot",
		Flags: []string{
			"--session",
			"--project-root",
			"--agent",
			"--mode",
			"--lines",
			"--delta-from",
			"--markers",
			"--keep-noise",
			"--strip-noise",
			"--fail-not-found",
			"--json",
			"--json-min",
		},
	},
	{
		Name: "session status",
		Flags: []string{
			"--session",
			"--agent",
			"--mode",
			"--project-root",
			"--full",
			"--fail-not-found",
			"--json",
			"--json-min",
		},
	},
	{
		Name:  "session explain",
		Flags: []string{"--session", "--agent", "--mode", "--project-root", "--events", "--recent", "--since", "--json", "--json-min"},
	},
	{
		Name: "session monitor",
		Flags: []string{
			"--session",
			"--project-root",
			"--agent",
			"--mode",
			"--poll-interval",
			"--adaptive-poll",
			"--max-polls",
			"--timeout-seconds",
			"--stop-on-waiting",
			"--waiting-requires-turn-complete",
			"--until-marker",
			"--until-state",
			"--until-jsonpath",
			"--expect",
			"--json",
			"--json-min",
			"--stream-json",
			"--emit-handoff",
			"--handoff-cursor-file",
			"--event-budget",
			"--webhook",
			"--verbose",
			"--auto-recover",
			"--recover-max",
			"--recover-budget",
		},
	},
	{
		Name: "session capture",
		Flags: []string{
			"--session",
			"--project-root",
			"--lines",
			"--raw",
			"--delta-from",
			"--cursor-file",
			"--markers",
			"--markers-json",
			"--summary",
			"--summary-style",
			"--token-budget",
			"--semantic-delta",
			"--keep-noise",
			"--strip-noise",
			"--strip-banner",
			"--json",
			"--json-min",
		},
	},
	{
		Name:  "session packet",
		Flags: []string{"--session", "--project-root", "--agent", "--mode", "--lines", "--events", "--token-budget", "--summary-style", "--cursor-file", "--delta-json", "--fields", "--json", "--json-min"},
	},
	{
		Name:  "session turn",
		Flags: []string{"--session", "--project-root", "--text", "--keys", "--enter", "--agent", "--mode", "--expect", "--poll-interval", "--max-polls", "--timeout-seconds", "--stop-on-waiting", "--waiting-requires-turn-complete", "--until-marker", "--until-state", "--until-jsonpath", "--auto-recover", "--recover-max", "--recover-budget", "--lines", "--events", "--token-budget", "--summary-style", "--cursor-file", "--fields", "--json", "--json-min"},
	},
	{
		Name:  "session contract-check",
		Flags: []string{"--project-root", "--json"},
	},
	{
		Name:  "session schema",
		Flags: []string{"--command", "--json"},
	},
	{
		Name:  "session checkpoint",
		Flags: []string{"--action", "--session", "--file", "--project-root", "--events", "--lines", "--strategy", "--token-budget", "--json"},
	},
	{
		Name:  "session dedupe",
		Flags: []string{"--task-hash", "--session", "--release", "--project-root", "--json"},
	},
	{
		Name:  "session next",
		Flags: []string{"--session", "--project-root", "--budget", "--json"},
	},
	{
		Name:  "session aggregate",
		Flags: []string{"--sessions", "--project-root", "--strategy", "--events", "--lines", "--token-budget", "--dedupe", "--delta-json", "--cursor-file", "--json", "--json-min"},
	},
	{
		Name:  "session prompt-lint",
		Flags: []string{"--agent", "--mode", "--nested-policy", "--nesting-intent", "--prompt", "--model", "--project-root", "--markers", "--budget", "--strict", "--rewrite", "--json"},
	},
	{
		Name:  "session diff-pack",
		Flags: []string{"--session", "--project-root", "--strategy", "--events", "--lines", "--token-budget", "--cursor-file", "--redact", "--semantic-only", "--json", "--json-min"},
	},
	{
		Name:  "session loop",
		Flags: []string{"--session", "--project-root", "--poll-interval", "--max-polls", "--strategy", "--events", "--lines", "--token-budget", "--cursor-file", "--handoff-cursor-file", "--schema", "--steps", "--max-tokens", "--max-seconds", "--max-steps", "--json", "--json-min"},
	},
	{
		Name:  "session context-cache",
		Flags: []string{"--key", "--session", "--project-root", "--refresh", "--from", "--ttl-hours", "--max-lines", "--list", "--clear", "--json"},
	},
	{
		Name:  "session anomaly",
		Flags: []string{"--session", "--project-root", "--events", "--auto-remediate", "--json"},
	},
	{
		Name:  "session budget-observe",
		Flags: []string{"--from", "--from-jsonl", "--tokens", "--seconds", "--steps", "--json"},
	},
	{
		Name:  "session budget-enforce",
		Flags: []string{"--from", "--from-jsonl", "--max-tokens", "--max-seconds", "--max-steps", "--tokens", "--seconds", "--steps", "--json"},
	},
	{
		Name:  "session budget-plan",
		Flags: []string{"--goal", "--agent", "--profile", "--budget", "--topology", "--from-state", "--project-root", "--json"},
	},
	{
		Name:  "session replay",
		Flags: []string{"--from-checkpoint", "--project-root", "--json"},
	},
	{
		Name:  "session handoff",
		Flags: []string{"--session", "--project-root", "--agent", "--mode", "--events", "--delta-from", "--cursor-file", "--compress", "--schema", "--json", "--json-min"},
	},
	{
		Name:  "session context-pack",
		Flags: []string{"--for", "--session", "--project-root", "--agent", "--mode", "--events", "--lines", "--token-budget", "--strategy", "--from-handoff", "--redact", "--json", "--json-min"},
	},
	{
		Name:  "session route",
		Flags: []string{"--goal", "--agent", "--lane", "--prompt", "--model", "--profile", "--budget", "--queue", "--sessions", "--queue-limit", "--concurrency", "--topology", "--cost-estimate", "--from-state", "--strict", "--project-root", "--emit-runbook", "--json"},
	},
	{
		Name:  "session autopilot",
		Flags: []string{"--goal", "--agent", "--lane", "--mode", "--nested-policy", "--nesting-intent", "--session", "--prompt", "--model", "--project-root", "--poll-interval", "--max-polls", "--capture-lines", "--summary", "--summary-style", "--token-budget", "--kill-after", "--resume-from", "--json"},
	},
	{
		Name:  "session guard",
		Flags: []string{"--shared-tmux", "--enforce", "--advice-only", "--machine-policy", "--command", "--policy-file", "--project-root", "--json"},
	},
	{
		Name:  "session objective",
		Flags: []string{"--project-root", "--id", "--goal", "--acceptance", "--budget", "--status", "--ttl-hours", "--activate", "--clear", "--list", "--json"},
	},
	{
		Name:  "session memory",
		Flags: []string{"--session", "--project-root", "--refresh", "--semantic-diff", "--ttl-hours", "--max-lines", "--json"},
	},
	{
		Name:  "session lane",
		Flags: []string{"--project-root", "--name", "--goal", "--agent", "--mode", "--nested-policy", "--nesting-intent", "--prompt", "--model", "--budget", "--topology", "--contract", "--clear", "--list", "--json"},
	},
	{
		Name:  "session state-sandbox",
		Flags: []string{"--action", "--project-root", "--file", "--json", "--json-min"},
	},
	{
		Name:  "session tree",
		Flags: []string{"--session", "--project-root", "--all-hashes", "--active-only", "--delta", "--delta-json", "--cursor-file", "--flat", "--with-state", "--json", "--json-min"},
	},
	{
		Name:  "session smoke",
		Flags: []string{"--project-root", "--levels", "--prompt-style", "--matrix-file", "--chaos", "--chaos-report", "--contract-profile", "--llm-profile", "--model", "--poll-interval", "--max-polls", "--keep-sessions", "--report-min", "--export-artifacts", "--json"},
	},
	{
		Name:  "session preflight",
		Flags: []string{"--project-root", "--agent", "--model", "--auto-model", "--auto-model-candidates", "--fast", "--json"},
	},
	{
		Name:  "session list",
		Flags: []string{"--all-sockets", "--project-only", "--active-only", "--with-next-action", "--priority", "--stale", "--prune-preview", "--delta-json", "--cursor-file", "--watch-json", "--watch-interval", "--watch-cycles", "--project-root", "--json", "--json-min"},
	},
	{
		Name:  "session exists",
		Flags: []string{"--session", "--project-root", "--json"},
	},
	{
		Name:  "session kill",
		Flags: []string{"--session", "--project-root", "--cleanup-all-hashes", "--json"},
	},
	{
		Name:  "session kill-all",
		Flags: []string{"--project-root", "--cleanup-all-hashes", "--project-only", "--json"},
	},
	{
		Name:  "agent build-cmd",
		Flags: []string{"--agent", "--mode", "--nested-policy", "--nesting-intent", "--prompt", "--project-root", "--agent-args", "--model", "--no-dangerously-skip-permissions", "--json"},
	},
	{
		Name:  "oauth add",
		Flags: []string{"--token", "--stdin", "--json"},
	},
	{
		Name:  "oauth list",
		Flags: []string{"--json"},
	},
	{
		Name:  "oauth remove",
		Flags: []string{"--id", "--json"},
	},
	{
		Name:  "skills sync",
		Flags: []string{"--from", "--path", "--repo-root", "--json"},
	},
	{
		Name:  "skills doctor",
		Flags: []string{"--repo-root", "--deep", "--explain-drift", "--fix", "--contract-check", "--sync-plan", "--json"},
	},
	{
		Name:  "skills install",
		Flags: []string{"--to", "--path", "--project-path", "--repo-root", "--json"},
	},
}

func cmdCapabilities(args []string) int {
	jsonOut := hasJSONFlag(args)
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			return showHelp("capabilities")
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", arg)
		}
	}

	if jsonOut {
		payload := map[string]any{
			"version":     BuildVersion,
			"commit":      BuildCommit,
			"date":        BuildDate,
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"commands":    commandCapabilities,
		}
		writeJSON(payload)
		return 0
	}

	fmt.Println("lisa CLI capabilities (use --json for structured output):")
	fmt.Println()
	for _, entry := range commandCapabilities {
		if len(entry.Flags) == 0 {
			fmt.Printf("- %s", entry.Name)
		} else {
			fmt.Printf("- %-16s flags: %s", entry.Name, strings.Join(entry.Flags, ", "))
		}
		if entry.Description != "" {
			fmt.Printf("\n    %s", entry.Description)
		}
		fmt.Println()
	}
	return 0
}

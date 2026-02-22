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
		Flags: []string{"--agent", "--mode", "--nested-policy", "--nesting-intent", "--prompt", "--agent-args", "--model", "--project-root", "--rewrite", "--json"},
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
		Flags: []string{"--session", "--agent", "--mode", "--project-root", "--events", "--recent", "--json", "--json-min"},
	},
	{
		Name: "session monitor",
		Flags: []string{
			"--session",
			"--project-root",
			"--agent",
			"--mode",
			"--poll-interval",
			"--max-polls",
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
			"--verbose",
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
			"--summary",
			"--summary-style",
			"--token-budget",
			"--keep-noise",
			"--strip-noise",
			"--json",
			"--json-min",
		},
	},
	{
		Name:  "session handoff",
		Flags: []string{"--session", "--project-root", "--agent", "--mode", "--events", "--delta-from", "--cursor-file", "--json", "--json-min"},
	},
	{
		Name:  "session context-pack",
		Flags: []string{"--for", "--session", "--project-root", "--agent", "--mode", "--events", "--lines", "--token-budget", "--strategy", "--from-handoff", "--json", "--json-min"},
	},
	{
		Name:  "session route",
		Flags: []string{"--goal", "--agent", "--prompt", "--model", "--budget", "--project-root", "--emit-runbook", "--json"},
	},
	{
		Name:  "session autopilot",
		Flags: []string{"--goal", "--agent", "--mode", "--nested-policy", "--nesting-intent", "--session", "--prompt", "--model", "--project-root", "--poll-interval", "--max-polls", "--capture-lines", "--summary", "--summary-style", "--token-budget", "--kill-after", "--json"},
	},
	{
		Name:  "session guard",
		Flags: []string{"--shared-tmux", "--enforce", "--command", "--project-root", "--json"},
	},
	{
		Name:  "session tree",
		Flags: []string{"--session", "--project-root", "--all-hashes", "--active-only", "--delta", "--flat", "--with-state", "--json", "--json-min"},
	},
	{
		Name:  "session smoke",
		Flags: []string{"--project-root", "--levels", "--prompt-style", "--matrix-file", "--chaos", "--model", "--poll-interval", "--max-polls", "--keep-sessions", "--report-min", "--json"},
	},
	{
		Name:  "session preflight",
		Flags: []string{"--project-root", "--agent", "--model", "--auto-model", "--auto-model-candidates", "--json"},
	},
	{
		Name:  "session list",
		Flags: []string{"--all-sockets", "--project-only", "--active-only", "--with-next-action", "--stale", "--prune-preview", "--project-root", "--json", "--json-min"},
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
		Name:  "skills sync",
		Flags: []string{"--from", "--path", "--repo-root", "--json"},
	},
	{
		Name:  "skills doctor",
		Flags: []string{"--repo-root", "--deep", "--explain-drift", "--json"},
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

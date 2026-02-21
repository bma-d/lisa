package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type doctorCheck struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Error     string `json:"error,omitempty"`
}

func doctorJSONPayload(allOK bool, results []doctorCheck) map[string]any {
	return map[string]any{
		"ok":      allOK,
		"checks":  results,
		"version": BuildVersion,
		"commit":  BuildCommit,
		"date":    BuildDate,
	}
}

func doctorReady(results []doctorCheck) bool {
	tmuxOK := false
	agentOK := false
	for _, r := range results {
		if r.Name == "tmux" && r.Available {
			tmuxOK = true
		}
		if (r.Name == "claude" || r.Name == "codex") && r.Available {
			agentOK = true
		}
	}
	return tmuxOK && agentOK
}

func cmdDoctor(args []string) int {
	jsonOut := hasJSONFlag(args)
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			return showHelp("doctor")
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", arg)
		}
	}

	results := []doctorCheck{}
	for _, bin := range []string{"tmux", "claude", "codex"} {
		path, err := exec.LookPath(bin)
		if err != nil {
			results = append(results, doctorCheck{Name: bin, Available: false, Error: err.Error()})
			continue
		}
		results = append(results, doctorCheck{Name: bin, Available: true, Path: path})
	}

	allOK := doctorReady(results)

	if jsonOut {
		payload := doctorJSONPayload(allOK, results)
		if !allOK {
			payload["errorCode"] = "doctor_missing_prerequisites"
		}
		writeJSON(payload)
		return boolExit(allOK)
	}

	for _, r := range results {
		if r.Available {
			fmt.Printf("ok      %-7s %s\n", r.Name, r.Path)
		} else {
			fmt.Printf("missing %-7s %s\n", r.Name, r.Error)
		}
	}
	if allOK {
		fmt.Println("doctor: ready")
		return 0
	}
	fmt.Println("doctor: missing prerequisites")
	return 1
}

func cmdAgent(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lisa agent <subcommand>")
		return 1
	}
	if args[0] == "--help" || args[0] == "-h" {
		return showHelp("agent")
	}
	if args[0] == "help" {
		if len(args) > 1 {
			return showHelp("agent " + args[1])
		}
		return showHelp("agent")
	}
	switch args[0] {
	case "build-cmd":
		return cmdAgentBuildCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown agent subcommand: %s\n", args[0])
		return 1
	}
}

func cmdAgentBuildCmd(args []string) int {
	agent := "claude"
	mode := "interactive"
	prompt := ""
	agentArgs := ""
	skipPermissions := true
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("agent build-cmd")
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agent = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			mode = args[i+1]
			i++
		case "--prompt":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --prompt")
			}
			prompt = args[i+1]
			i++
		case "--agent-args":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent-args")
			}
			agentArgs = args[i+1]
			i++
		case "--no-dangerously-skip-permissions":
			skipPermissions = false
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}
	if shouldAutoEnableNestedCodexBypass(agent, mode, prompt, agentArgs) {
		agentArgs = strings.TrimSpace(agentArgs + " --dangerously-bypass-approvals-and-sandbox")
	}

	cmd, err := buildAgentCommandWithOptions(agent, mode, prompt, agentArgs, skipPermissions)
	if err != nil {
		return commandError(jsonOut, "agent_command_build_failed", err.Error())
	}

	agent, _ = parseAgent(agent)
	mode, _ = parseMode(mode)

	if jsonOut {
		writeJSON(map[string]any{
			"agent":   agent,
			"mode":    mode,
			"prompt":  prompt,
			"command": cmd,
		})
		return 0
	}

	fmt.Println(cmd)
	return 0
}

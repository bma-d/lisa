package app

import (
	"fmt"
	"os"
	"os/exec"
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
	jsonOut := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
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
		writeJSON(doctorJSONPayload(allOK, results))
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
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 >= len(args) {
				return flagValueError("--agent")
			}
			agent = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return flagValueError("--mode")
			}
			mode = args[i+1]
			i++
		case "--prompt":
			if i+1 >= len(args) {
				return flagValueError("--prompt")
			}
			prompt = args[i+1]
			i++
		case "--agent-args":
			if i+1 >= len(args) {
				return flagValueError("--agent-args")
			}
			agentArgs = args[i+1]
			i++
		case "--json":
			jsonOut = true
		default:
			return unknownFlagError(args[i])
		}
	}

	cmd, err := buildAgentCommand(agent, mode, prompt, agentArgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
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

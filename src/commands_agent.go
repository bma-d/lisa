package app

import (
	"fmt"
	"os"
	"os/exec"
)

func cmdDoctor(args []string) int {
	jsonOut := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
		}
	}

	type check struct {
		Name      string `json:"name"`
		Available bool   `json:"available"`
		Path      string `json:"path,omitempty"`
		Error     string `json:"error,omitempty"`
	}
	results := []check{}
	for _, bin := range []string{"tmux", "claude", "codex"} {
		path, err := exec.LookPath(bin)
		if err != nil {
			results = append(results, check{Name: bin, Available: false, Error: err.Error()})
			continue
		}
		results = append(results, check{Name: bin, Available: true, Path: path})
	}

	allOK := true
	for _, r := range results {
		if !r.Available {
			allOK = false
		}
	}

	if jsonOut {
		payload := map[string]any{
			"ok":      allOK,
			"checks":  results,
			"version": "1.0.0",
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

	if jsonOut {
		writeJSON(map[string]any{
			"agent":   normalizeAgent(agent),
			"mode":    normalizeMode(mode),
			"prompt":  prompt,
			"command": cmd,
		})
		return 0
	}

	fmt.Println(cmd)
	return 0
}

package app

import (
	"errors"
	"fmt"
	"strings"
)

func buildAgentCommand(agent, mode, prompt, agentArgs string) (string, error) {
	agent = normalizeAgent(agent)
	mode = normalizeMode(mode)

	switch mode {
	case "interactive":
		base := "claude"
		if agent == "codex" {
			base = "codex"
		}
		var parts []string
		parts = append(parts, base)
		if strings.TrimSpace(agentArgs) != "" {
			parts = append(parts, strings.TrimSpace(agentArgs))
		}
		if strings.TrimSpace(prompt) != "" {
			parts = append(parts, shellQuote(prompt))
		}
		return strings.Join(parts, " "), nil

	case "exec":
		if strings.TrimSpace(prompt) == "" {
			return "", errors.New("exec mode requires --prompt (or provide --command)")
		}
		if agent == "codex" {
			base := fmt.Sprintf("codex exec %s --full-auto", shellQuote(prompt))
			if strings.TrimSpace(agentArgs) != "" {
				base += " " + strings.TrimSpace(agentArgs)
			}
			return base, nil
		}
		base := fmt.Sprintf("claude -p %s", shellQuote(prompt))
		if strings.TrimSpace(agentArgs) != "" {
			base += " " + strings.TrimSpace(agentArgs)
		}
		return base, nil
	}

	return "", fmt.Errorf("invalid mode: %s", mode)
}

func wrapExecCommand(command string) string {
	return fmt.Sprintf("{ %s; __lisa_ec=$?; printf '\\n%s%%d\\n' \"$__lisa_ec\"; }", command, execDonePrefix)
}

func normalizeAgent(agent string) string {
	a := strings.ToLower(strings.TrimSpace(agent))
	if a == "codex" {
		return "codex"
	}
	return "claude"
}

func normalizeMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "exec" || m == "execution" || m == "non-interactive" {
		return "exec"
	}
	return "interactive"
}

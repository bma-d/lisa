package app

import (
	"errors"
	"fmt"
	"strings"
)

func buildAgentCommand(agent, mode, prompt, agentArgs string) (string, error) {
	var err error
	agent, err = parseAgent(agent)
	if err != nil {
		return "", err
	}
	mode, err = parseMode(mode)
	if err != nil {
		return "", err
	}

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
	return fmt.Sprintf("{ __lisa_had_errexit=0; case $- in *e*) __lisa_had_errexit=1;; esac; set +e; %s; __lisa_ec=$?; printf '\\n%s%%d\\n' \"$__lisa_ec\"; if [ \"$__lisa_had_errexit\" -eq 1 ]; then set -e; fi; }", command, execDonePrefix)
}

func parseAgent(agent string) (string, error) {
	a := strings.ToLower(strings.TrimSpace(agent))
	switch a {
	case "claude", "":
		return "claude", nil
	case "codex":
		return "codex", nil
	default:
		return "", fmt.Errorf("invalid --agent: %s (expected claude|codex)", agent)
	}
}

func parseMode(mode string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "interactive", "":
		return "interactive", nil
	case "exec", "execution", "non-interactive":
		return "exec", nil
	default:
		return "", fmt.Errorf("invalid --mode: %s (expected interactive|exec)", mode)
	}
}

func normalizeAgent(agent string) string {
	a, err := parseAgent(agent)
	if err != nil {
		return "claude"
	}
	return a
}

func normalizeMode(mode string) string {
	m, err := parseMode(mode)
	if err != nil {
		return "interactive"
	}
	return m
}

package app

import (
	"fmt"
	"strings"
)

func parseModel(model string) (string, error) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", nil
	}
	if strings.ContainsAny(trimmed, "\r\n\t") {
		return "", fmt.Errorf("invalid --model: control characters are not allowed")
	}
	switch strings.ToLower(trimmed) {
	case "codex-spark":
		return "gpt-5.3-codex-spark", nil
	default:
		return trimmed, nil
	}
}

func applyModelToAgentArgs(agent, agentArgs, model string) (string, error) {
	trimmedArgs := strings.TrimSpace(agentArgs)
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" {
		return trimmedArgs, nil
	}
	if agent != "codex" {
		return "", fmt.Errorf("invalid --model: only supported when --agent is codex")
	}
	if hasFlagToken(trimmedArgs, "--model") {
		return "", fmt.Errorf("invalid model configuration: --model cannot be combined with --agent-args that already include --model")
	}
	if trimmedArgs == "" {
		return fmt.Sprintf("--model %s", shellQuote(trimmedModel)), nil
	}
	return fmt.Sprintf("%s --model %s", trimmedArgs, shellQuote(trimmedModel)), nil
}

package app

import "strings"

type sessionPreflightModelCheck struct {
	Agent     string `json:"agent"`
	Model     string `json:"model"`
	OK        bool   `json:"ok"`
	Detail    string `json:"detail,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

func runSessionPreflightModelCheck(agent, model string) sessionPreflightModelCheck {
	result := sessionPreflightModelCheck{
		Agent: agent,
		Model: model,
		OK:    false,
	}

	if normalizeAgent(agent) != "codex" {
		result.ErrorCode = "preflight_model_agent_unsupported"
		result.Detail = "model probing currently supports --agent codex only"
		return result
	}

	out, err := runCmd(
		"codex",
		"exec",
		"Reply exactly: LISA_PREFLIGHT_MODEL_OK",
		"--full-auto",
		"--skip-git-repo-check",
		"--model",
		model,
	)
	lower := strings.ToLower(out)
	if err == nil {
		if strings.Contains(out, "LISA_PREFLIGHT_MODEL_OK") {
			result.OK = true
			result.Detail = "model probe succeeded"
			return result
		}
		result.OK = true
		result.Detail = "model probe command completed"
		return result
	}

	result.Detail = summarizeModelProbeDetail(out, err.Error())
	switch {
	case strings.Contains(lower, "not supported") && strings.Contains(lower, "model"):
		result.ErrorCode = "preflight_model_not_supported"
	case strings.Contains(lower, "model metadata") && strings.Contains(lower, "not found"):
		result.ErrorCode = "preflight_model_metadata_missing"
	case strings.Contains(lower, "authentication") || strings.Contains(lower, "login") || strings.Contains(lower, "unauthorized"):
		result.ErrorCode = "preflight_model_auth_failed"
	default:
		result.ErrorCode = "preflight_model_probe_failed"
	}
	return result
}

func summarizeModelProbeDetail(out, errMsg string) string {
	parts := []string{}
	for _, candidate := range []string{strings.TrimSpace(out), strings.TrimSpace(errMsg)} {
		if candidate == "" {
			continue
		}
		line := strings.Split(candidate, "\n")[0]
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts = append(parts, line)
		if len(parts) == 2 {
			break
		}
	}
	if len(parts) == 0 {
		return "model probe failed"
	}
	detail := strings.Join(parts, " | ")
	if len(detail) > 260 {
		return detail[:257] + "..."
	}
	return detail
}

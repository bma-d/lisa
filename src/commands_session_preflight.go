package app

import (
	"fmt"
	"strings"
	"time"
)

type sessionPreflightContractCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type sessionPreflightAutoModelAttempt struct {
	Model     string `json:"model"`
	OK        bool   `json:"ok"`
	Detail    string `json:"detail,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

type sessionPreflightAutoModelResult struct {
	Enabled    bool                               `json:"enabled"`
	Candidates []string                           `json:"candidates"`
	Selected   string                             `json:"selected,omitempty"`
	Attempts   []sessionPreflightAutoModelAttempt `json:"attempts"`
}

var sessionPreflightModelCheckFn = runSessionPreflightModelCheck

func cmdSessionPreflight(args []string) int {
	projectRoot := getPWD()
	agent := ""
	model := ""
	autoModel := false
	autoModelCandidates := ""
	jsonOut := hasJSONFlag(args)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session preflight")
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agent = args[i+1]
			i++
		case "--model":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --model")
			}
			model = args[i+1]
			i++
		case "--auto-model":
			autoModel = true
		case "--auto-model-candidates":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --auto-model-candidates")
			}
			autoModelCandidates = args[i+1]
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	projectRoot = canonicalProjectRoot(projectRoot)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	var err error
	agent = strings.TrimSpace(agent)
	model = strings.TrimSpace(model)
	if model != "" && agent == "" {
		agent = "codex"
	}
	if autoModel && strings.TrimSpace(model) != "" {
		return commandError(jsonOut, "model_auto_model_conflict", "cannot combine --model with --auto-model")
	}
	if autoModel && agent == "" {
		agent = "codex"
	}
	if agent != "" {
		agent, err = parseAgent(agent)
		if err != nil {
			return commandError(jsonOut, "invalid_agent", err.Error())
		}
	}
	if model != "" {
		model, err = parseModel(model)
		if err != nil {
			return commandError(jsonOut, "invalid_model", err.Error())
		}
		if _, err := applyModelToAgentArgs(agent, "", model); err != nil {
			return commandError(jsonOut, "invalid_model_configuration", err.Error())
		}
	}

	envChecks := collectDoctorChecks()
	envReady := doctorReady(envChecks)
	contractChecks := runSessionPreflightContractChecks()
	autoModelResult := sessionPreflightAutoModelResult{}
	var modelCheck *sessionPreflightModelCheck
	if autoModel {
		autoModelResult.Enabled = true
		autoModelResult.Candidates = preflightAutoModelCandidates(autoModelCandidates)
		for _, candidate := range autoModelResult.Candidates {
			parsedModel, parseErr := parseModel(candidate)
			if parseErr != nil {
				autoModelResult.Attempts = append(autoModelResult.Attempts, sessionPreflightAutoModelAttempt{
					Model:     candidate,
					OK:        false,
					Detail:    parseErr.Error(),
					ErrorCode: "invalid_model",
				})
				continue
			}
			probe := sessionPreflightModelCheckFn(agent, parsedModel)
			autoModelResult.Attempts = append(autoModelResult.Attempts, sessionPreflightAutoModelAttempt{
				Model:     parsedModel,
				OK:        probe.OK,
				Detail:    probe.Detail,
				ErrorCode: probe.ErrorCode,
			})
			if probe.OK {
				autoModelResult.Selected = parsedModel
				model = parsedModel
				modelCheck = &probe
				break
			}
			probeCopy := probe
			modelCheck = &probeCopy
		}
		if modelCheck == nil {
			modelCheck = &sessionPreflightModelCheck{
				Agent:     agent,
				Model:     "",
				OK:        false,
				Detail:    "no auto-model candidates evaluated",
				ErrorCode: "preflight_auto_model_empty",
			}
		}
	}
	if model != "" && !autoModel {
		probe := sessionPreflightModelCheckFn(agent, model)
		modelCheck = &probe
	}

	contractsReady := true
	for _, check := range contractChecks {
		if !check.OK {
			contractsReady = false
			break
		}
	}
	modelReady := true
	if modelCheck != nil {
		modelReady = modelCheck.OK
	}

	ok := envReady && contractsReady && modelReady
	if jsonOut {
		payload := map[string]any{
			"ok":          ok,
			"projectRoot": projectRoot,
			"environment": map[string]any{"ok": envReady, "checks": envChecks},
			"contracts":   contractChecks,
			"version":     BuildVersion,
			"commit":      BuildCommit,
			"date":        BuildDate,
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
		}
		if modelCheck != nil {
			payload["modelCheck"] = modelCheck
		}
		if autoModel {
			payload["autoModel"] = autoModelResult
		}
		if code := preflightErrorCode(ok); code != "" {
			payload["errorCode"] = code
		}
		writeJSON(payload)
		return boolExit(ok)
	}

	state := "ready"
	if !ok {
		state = "not_ready"
	}
	fmt.Printf("session preflight: %s\n", state)
	fmt.Printf("project_root: %s\n", projectRoot)
	fmt.Println("environment:")
	for _, c := range envChecks {
		line := "missing"
		detail := c.Error
		if c.Available {
			line = "ok"
			detail = c.Path
		}
		fmt.Printf("- %s %s %s\n", line, c.Name, strings.TrimSpace(detail))
	}
	fmt.Println("contracts:")
	for _, c := range contractChecks {
		line := "ok"
		if !c.OK {
			line = "fail"
		}
		if c.Detail != "" {
			fmt.Printf("- %s %s (%s)\n", line, c.Name, c.Detail)
			continue
		}
		fmt.Printf("- %s %s\n", line, c.Name)
	}
	if modelCheck != nil {
		line := "ok"
		if !modelCheck.OK {
			line = "fail"
		}
		fmt.Printf("model_check: %s agent=%s model=%s detail=%s\n", line, modelCheck.Agent, modelCheck.Model, modelCheck.Detail)
	}
	if autoModel {
		fmt.Printf("auto_model_selected: %s\n", strings.TrimSpace(autoModelResult.Selected))
		for _, attempt := range autoModelResult.Attempts {
			state := "ok"
			if !attempt.OK {
				state = "fail"
			}
			fmt.Printf("auto_model_attempt: %s model=%s detail=%s\n", state, attempt.Model, attempt.Detail)
		}
	}
	return boolExit(ok)
}

func preflightAutoModelCandidates(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"gpt-5.3-codex", "gpt-5-codex"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		model := strings.TrimSpace(part)
		if model == "" {
			continue
		}
		if seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, model)
	}
	if len(out) == 0 {
		return []string{"gpt-5.3-codex", "gpt-5-codex"}
	}
	return out
}

func preflightErrorCode(ok bool) string {
	if ok {
		return ""
	}
	return "session_preflight_failed"
}

func runSessionPreflightContractChecks() []sessionPreflightContractCheck {
	checks := []sessionPreflightContractCheck{}
	add := func(name string, ok bool, detail string) {
		checks = append(checks, sessionPreflightContractCheck{Name: name, OK: ok, Detail: strings.TrimSpace(detail)})
	}

	modeExecution, err := parseMode("execution")
	add("mode_alias_execution", err == nil && modeExecution == "exec", fmt.Sprintf("mode=%s err=%v", modeExecution, err))

	modeNonInteractive, err := parseMode("non-interactive")
	add("mode_alias_non_interactive", err == nil && modeNonInteractive == "exec", fmt.Sprintf("mode=%s err=%v", modeNonInteractive, err))

	_, err = parseMonitorExpect("marker")
	add("monitor_expect_marker_parse", err == nil, fmt.Sprintf("err=%v", err))

	err = validateMonitorExpectationConfig("marker", "")
	add("monitor_expect_marker_guard", err != nil, fmt.Sprintf("err=%v", err))

	err = validateMonitorExpectationConfig("marker", "DONE_MARKER")
	add("monitor_expect_marker_until_marker", err == nil, fmt.Sprintf("err=%v", err))

	offset, ok := parseCaptureOffset("42")
	add("capture_delta_offset_parse", ok && offset == 42, fmt.Sprintf("offset=%d ok=%t", offset, ok))

	_, err = parseCaptureTimestamp("@0")
	add("capture_delta_unix_timestamp_parse", err == nil, fmt.Sprintf("err=%v", err))

	_, err = parseCaptureTimestamp("2026-02-21T00:00:00Z")
	add("capture_delta_rfc3339_parse", err == nil, fmt.Sprintf("err=%v", err))

	autoDetection, autoArgs, err := applyNestedPolicyToAgentArgs("codex", "exec", "Use ./lisa for child orchestration.", "", "auto", "auto")
	autoCmd := ""
	if err == nil {
		autoCmd, err = buildAgentCommandWithOptions("codex", "exec", "Use ./lisa for child orchestration.", autoArgs, true)
	}
	autoBypass := err == nil &&
		autoDetection.AutoBypass &&
		strings.Contains(autoCmd, "--dangerously-bypass-approvals-and-sandbox") &&
		!strings.Contains(autoCmd, "--full-auto")
	add("nested_prompt_auto_bypass", autoBypass, fmt.Sprintf("reason=%s err=%v", autoDetection.Reason, err))

	neutralDetection, neutralArgs, err := applyNestedPolicyToAgentArgs("codex", "exec", "No nesting requested here.", "", "auto", "auto")
	neutralCmd := ""
	if err == nil {
		neutralCmd, err = buildAgentCommandWithOptions("codex", "exec", "No nesting requested here.", neutralArgs, true)
	}
	neutralFullAuto := err == nil &&
		!neutralDetection.AutoBypass &&
		strings.Contains(neutralCmd, "--full-auto") &&
		!strings.Contains(neutralCmd, "--dangerously-bypass-approvals-and-sandbox")
	add("nested_prompt_neutral_full_auto", neutralFullAuto, fmt.Sprintf("reason=%s err=%v", neutralDetection.Reason, err))

	quotedDetection, _, err := applyNestedPolicyToAgentArgs("codex", "exec", "The string './lisa' appears in docs only.", "", "auto", "auto")
	add("nested_prompt_quote_guard", err == nil && !quotedDetection.AutoBypass, fmt.Sprintf("reason=%s err=%v", quotedDetection.Reason, err))

	intentDetection, intentArgs, err := applyNestedPolicyToAgentArgs("codex", "exec", "No nesting requested here.", "", "auto", "nested")
	intentCmd := ""
	if err == nil {
		intentCmd, err = buildAgentCommandWithOptions("codex", "exec", "No nesting requested here.", intentArgs, true)
	}
	intentBypass := err == nil &&
		intentDetection.AutoBypass &&
		strings.Contains(intentCmd, "--dangerously-bypass-approvals-and-sandbox")
	add("nested_intent_nested_bypass", intentBypass, fmt.Sprintf("reason=%s err=%v", intentDetection.Reason, err))

	return checks
}

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

func cmdSessionPreflight(args []string) int {
	projectRoot := getPWD()
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
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	projectRoot = canonicalProjectRoot(projectRoot)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	envChecks := collectDoctorChecks()
	envReady := doctorReady(envChecks)
	contractChecks := runSessionPreflightContractChecks()

	contractsReady := true
	for _, check := range contractChecks {
		if !check.OK {
			contractsReady = false
			break
		}
	}

	ok := envReady && contractsReady
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
	return boolExit(ok)
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

	autoDetection, autoArgs, err := applyNestedPolicyToAgentArgs("codex", "exec", "Use ./lisa for child orchestration.", "", "auto")
	autoCmd := ""
	if err == nil {
		autoCmd, err = buildAgentCommandWithOptions("codex", "exec", "Use ./lisa for child orchestration.", autoArgs, true)
	}
	autoBypass := err == nil &&
		autoDetection.AutoBypass &&
		strings.Contains(autoCmd, "--dangerously-bypass-approvals-and-sandbox") &&
		!strings.Contains(autoCmd, "--full-auto")
	add("nested_prompt_auto_bypass", autoBypass, fmt.Sprintf("reason=%s err=%v", autoDetection.Reason, err))

	neutralDetection, neutralArgs, err := applyNestedPolicyToAgentArgs("codex", "exec", "No nesting requested here.", "", "auto")
	neutralCmd := ""
	if err == nil {
		neutralCmd, err = buildAgentCommandWithOptions("codex", "exec", "No nesting requested here.", neutralArgs, true)
	}
	neutralFullAuto := err == nil &&
		!neutralDetection.AutoBypass &&
		strings.Contains(neutralCmd, "--full-auto") &&
		!strings.Contains(neutralCmd, "--dangerously-bypass-approvals-and-sandbox")
	add("nested_prompt_neutral_full_auto", neutralFullAuto, fmt.Sprintf("reason=%s err=%v", neutralDetection.Reason, err))

	return checks
}

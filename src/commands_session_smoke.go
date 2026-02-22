package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var runLisaSubcommandFn = runLisaSubcommand
var osExecutableFn = os.Executable

type smokePromptProbe struct {
	Style          string               `json:"style"`
	Prompt         string               `json:"prompt"`
	ExpectedBypass bool                 `json:"expectedBypass"`
	Detection      nestedCodexDetection `json:"detection"`
	Command        string               `json:"command,omitempty"`
}

type sessionSmokeSummary struct {
	OK             bool               `json:"ok"`
	ProjectRoot    string             `json:"projectRoot"`
	Levels         int                `json:"levels"`
	Model          string             `json:"model,omitempty"`
	PromptStyle    string             `json:"promptStyle,omitempty"`
	PromptProbe    *smokePromptProbe  `json:"promptProbe,omitempty"`
	PromptMatrix   []smokeMatrixProbe `json:"promptMatrix,omitempty"`
	WorkDir        string             `json:"workDir"`
	KeepSessions   bool               `json:"keepSessions"`
	ReportMin      bool               `json:"reportMin,omitempty"`
	Sessions       []string           `json:"sessions"`
	Markers        []string           `json:"markers"`
	MissingMarkers []string           `json:"missingMarkers,omitempty"`
	Monitor        monitorResult      `json:"monitor"`
	Tree           *sessionTreeResult `json:"tree,omitempty"`
	Error          string             `json:"error,omitempty"`
	ErrorCode      string             `json:"errorCode,omitempty"`
	CleanupErrors  []string           `json:"cleanupErrors,omitempty"`
}

type smokeMatrixProbe struct {
	ExpectedMode string               `json:"expectedMode"`
	ActualMode   string               `json:"actualMode"`
	Prompt       string               `json:"prompt"`
	Pass         bool                 `json:"pass"`
	Detection    nestedCodexDetection `json:"detection"`
	Command      string               `json:"command,omitempty"`
}

func cmdSessionSmoke(args []string) int {
	projectRoot := getPWD()
	levels := 3
	promptStyle := "none"
	matrixFile := ""
	model := ""
	maxPolls := 180
	pollInterval := 1
	keepSessions := false
	reportMin := false
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session smoke")
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--levels":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --levels")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return commandError(jsonOut, "invalid_levels", "invalid --levels")
			}
			levels = n
			i++
		case "--prompt-style":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --prompt-style")
			}
			promptStyle = args[i+1]
			i++
		case "--matrix-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --matrix-file")
			}
			matrixFile = args[i+1]
			i++
		case "--model":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --model")
			}
			model = args[i+1]
			i++
		case "--max-polls":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-polls")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return commandError(jsonOut, "invalid_max_polls", "invalid --max-polls")
			}
			maxPolls = n
			i++
		case "--poll-interval":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --poll-interval")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return commandError(jsonOut, "invalid_poll_interval", "invalid --poll-interval")
			}
			pollInterval = n
			i++
		case "--keep-sessions":
			keepSessions = true
		case "--report-min":
			reportMin = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	if levels > 4 {
		return commandError(jsonOut, "invalid_levels_max", "invalid --levels: max supported is 4")
	}
	var err error
	promptStyle, err = parseSmokePromptStyle(promptStyle)
	if err != nil {
		return commandError(jsonOut, "invalid_prompt_style", err.Error())
	}
	model, err = parseModel(model)
	if err != nil {
		return commandError(jsonOut, "invalid_model", err.Error())
	}

	projectRoot = canonicalProjectRoot(projectRoot)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	if _, err := exec.LookPath("bash"); err != nil {
		return commandErrorf(jsonOut, "missing_bash", "error: required command not found: bash (%v)", err)
	}

	binPath, err := osExecutableFn()
	if err != nil {
		return commandErrorf(jsonOut, "binary_path_resolve_failed", "failed to resolve lisa binary path: %v", err)
	}
	binPath = strings.TrimSpace(binPath)
	if binPath == "" {
		return commandError(jsonOut, "binary_path_empty", "failed to resolve lisa binary path")
	}

	runID := fmt.Sprintf("%s-%d", time.Now().Format("20060102_150405"), os.Getpid())
	workDir := filepath.Join(os.TempDir(), "lisa-smoke-"+runID)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return commandErrorf(jsonOut, "smoke_workdir_create_failed", "failed to create smoke workdir: %v", err)
	}

	sessions := make([]string, levels)
	markers := make([]string, levels)
	scripts := make([]string, levels)
	for i := 0; i < levels; i++ {
		level := i + 1
		sessions[i] = fmt.Sprintf("lisa-smoke-l%d-%s", level, runID)
		markers[i] = fmt.Sprintf("LISA_SMOKE_L%d_DONE=1", level)
		scripts[i] = filepath.Join(workDir, fmt.Sprintf("l%d.sh", level))
	}

	summary := sessionSmokeSummary{
		OK:           false,
		ProjectRoot:  projectRoot,
		Levels:       levels,
		Model:        model,
		PromptStyle:  promptStyle,
		WorkDir:      workDir,
		KeepSessions: keepSessions,
		ReportMin:    reportMin,
		Sessions:     sessions,
		Markers:      markers,
	}

	if !keepSessions {
		defer func() {
			summary.CleanupErrors = append(summary.CleanupErrors, cleanupSmokeSessions(binPath, projectRoot, sessions)...)
		}()
	}

	if promptStyle != "none" {
		probe, probeErr := runSmokePromptStyleProbe(binPath, projectRoot, promptStyle, model)
		if probeErr != nil {
			return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_prompt_style_probe_failed", probeErr.Error())
		}
		summary.PromptProbe = probe
	}
	if strings.TrimSpace(matrixFile) != "" {
		matrixProbes, matrixErr := runSmokePromptMatrixProbe(binPath, projectRoot, matrixFile, model)
		summary.PromptMatrix = matrixProbes
		if matrixErr != nil {
			return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_prompt_matrix_assertion_failed", matrixErr.Error())
		}
	}

	for idx := levels - 1; idx >= 0; idx-- {
		lines := []string{
			"#!/usr/bin/env bash",
			"set -euo pipefail",
			"BIN=" + shellQuote(binPath),
			"ROOT=" + shellQuote(projectRoot),
		}

		if idx < levels-1 {
			child := sessions[idx+1]
			childScript := scripts[idx+1]
			spawnLine := fmt.Sprintf(`"$BIN" session spawn --agent codex --mode interactive --project-root "$ROOT" --session %s --command %s --json`,
				shellQuote(child),
				shellQuote("/bin/bash "+childScript),
			)
			if model != "" {
				spawnLine = strings.TrimSuffix(spawnLine, " --json") + " --model " + shellQuote(model) + " --json"
			}
			lines = append(lines, spawnLine,
				fmt.Sprintf(`"$BIN" session monitor --session %s --project-root "$ROOT" --poll-interval %d --max-polls %d --expect terminal --json`,
					shellQuote(child), pollInterval, maxPolls),
				fmt.Sprintf(`"$BIN" session capture --session %s --project-root "$ROOT" --raw --lines %d`,
					shellQuote(child), 120+idx*80),
			)
		}

		lines = append(lines,
			"echo "+markers[idx],
			"echo LISA_SMOKE_SESSION="+sessions[idx],
		)

		body := strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(scripts[idx], []byte(body), 0o700); err != nil {
			return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_script_write_failed", fmt.Sprintf("failed to write smoke script %s: %v", scripts[idx], err))
		}
	}

	rootSession := sessions[0]
	rootSpawnArgs := []string{
		"session", "spawn",
		"--agent", "codex",
		"--mode", "interactive",
		"--project-root", projectRoot,
		"--session", rootSession,
		"--command", "/bin/bash " + scripts[0],
	}
	if model != "" {
		rootSpawnArgs = append(rootSpawnArgs, "--model", model)
	}
	rootSpawnArgs = append(rootSpawnArgs, "--json")
	if _, stderr, err := runLisaSubcommandFn(binPath, rootSpawnArgs...); err != nil {
		return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_spawn_failed", formatSmokeSubcommandError("failed to spawn L1 smoke session", err, stderr))
	}

	monitorOut, monitorErr, err := runLisaSubcommandFn(binPath,
		"session", "monitor",
		"--session", rootSession,
		"--project-root", projectRoot,
		"--poll-interval", strconv.Itoa(pollInterval),
		"--max-polls", strconv.Itoa(maxPolls),
		"--expect", "terminal",
		"--json",
	)
	if err != nil {
		return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_monitor_failed", formatSmokeSubcommandError("failed to monitor L1 smoke session", err, monitorErr))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(monitorOut)), &summary.Monitor); err != nil {
		return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_monitor_parse_failed", fmt.Sprintf("failed to parse monitor output: %v", err))
	}
	if summary.Monitor.FinalState != "completed" {
		return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_unexpected_final_state", fmt.Sprintf("unexpected smoke final state: %s", summary.Monitor.FinalState))
	}

	captureOut, captureErr, err := runLisaSubcommandFn(binPath,
		"session", "capture",
		"--session", rootSession,
		"--project-root", projectRoot,
		"--raw",
		"--lines", strconv.Itoa(220+levels*120),
		"--json",
	)
	if err != nil {
		return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_capture_failed", formatSmokeSubcommandError("failed to capture smoke output", err, captureErr))
	}
	var capturePayload struct {
		Capture string `json:"capture"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(captureOut)), &capturePayload); err != nil {
		return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_capture_parse_failed", fmt.Sprintf("failed to parse capture output: %v", err))
	}
	for _, marker := range markers {
		if !strings.Contains(capturePayload.Capture, marker) {
			summary.MissingMarkers = append(summary.MissingMarkers, marker)
		}
	}
	if len(summary.MissingMarkers) > 0 {
		return emitSmokeFailure(jsonOut, reportMin, &summary, "smoke_marker_assertion_failed", "smoke marker assertions failed")
	}

	treeOut, _, err := runLisaSubcommandFn(binPath,
		"session", "tree",
		"--session", rootSession,
		"--project-root", projectRoot,
		"--json",
	)
	if err == nil {
		tree := sessionTreeResult{}
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(treeOut)), &tree); jsonErr == nil {
			summary.Tree = &tree
		}
	}

	summary.OK = true
	if jsonOut {
		writeSmokeSummaryJSON(summary, reportMin)
		return 0
	}
	fmt.Printf("PASS: nested smoke %d-level\n", levels)
	for i, session := range sessions {
		fmt.Printf("L%d=%s\n", i+1, session)
	}
	fmt.Printf("Artifacts: %s\n", workDir)
	return 0
}

func emitSmokeFailure(jsonOut bool, reportMin bool, summary *sessionSmokeSummary, errorCode, message string) int {
	summary.OK = false
	summary.ErrorCode = errorCode
	summary.Error = message
	if jsonOut {
		writeSmokeSummaryJSON(*summary, reportMin)
		return 1
	}
	fmt.Fprintln(os.Stderr, message)
	return 1
}

func writeSmokeSummaryJSON(summary sessionSmokeSummary, reportMin bool) {
	if !reportMin {
		writeJSON(summary)
		return
	}
	payload := map[string]any{
		"ok":         summary.OK,
		"errorCode":  summary.ErrorCode,
		"error":      summary.Error,
		"levels":     summary.Levels,
		"model":      summary.Model,
		"projectRoot": summary.ProjectRoot,
	}
	if summary.Monitor.FinalState != "" {
		payload["finalState"] = summary.Monitor.FinalState
		payload["exitReason"] = summary.Monitor.ExitReason
	}
	if len(summary.MissingMarkers) > 0 {
		payload["missingMarkers"] = summary.MissingMarkers
	}
	if len(summary.CleanupErrors) > 0 {
		payload["cleanupErrorCount"] = len(summary.CleanupErrors)
	}
	if len(summary.PromptMatrix) > 0 {
		failed := make([]map[string]any, 0)
		for _, probe := range summary.PromptMatrix {
			if probe.Pass {
				continue
			}
			failed = append(failed, map[string]any{
				"prompt":       probe.Prompt,
				"expectedMode": probe.ExpectedMode,
				"actualMode":   probe.ActualMode,
			})
		}
		if len(failed) > 0 {
			payload["failedMatrix"] = failed
		}
	}
	writeJSON(payload)
}

func cleanupSmokeSessions(binPath, projectRoot string, sessions []string) []string {
	errs := []string{}
	for i := len(sessions) - 1; i >= 0; i-- {
		session := sessions[i]
		if _, stderr, err := runLisaSubcommandFn(binPath,
			"session", "kill",
			"--session", session,
			"--project-root", projectRoot,
		); err != nil {
			errs = append(errs, formatSmokeSubcommandError(fmt.Sprintf("%s cleanup", session), err, stderr))
		}
	}
	return errs
}

func runLisaSubcommand(binPath string, args ...string) (string, string, error) {
	cmd := exec.Command(binPath, args...)
	cmd.Env = os.Environ()
	out, errOut := cmd.Output()
	if errOut != nil {
		stderrText := ""
		if exitErr, ok := errOut.(*exec.ExitError); ok {
			stderrText = strings.TrimSpace(string(exitErr.Stderr))
		}
		return strings.TrimSpace(string(out)), stderrText, errOut
	}
	return strings.TrimSpace(string(out)), "", nil
}

func parseSmokePromptStyle(style string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "", "none":
		return "none", nil
	case "dot-slash", "spawn", "nested", "neutral":
		return strings.ToLower(strings.TrimSpace(style)), nil
	default:
		return "", fmt.Errorf("invalid --prompt-style: %s (expected none|dot-slash|spawn|nested|neutral)", style)
	}
}

type smokePromptMatrixCase struct {
	ExpectedMode string
	Prompt       string
}

func parseSmokePromptMatrixFile(path string) ([]smokePromptMatrixCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cases := []smokePromptMatrixCase{}
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid matrix line %d: expected mode|prompt", lineNo)
		}
		mode := strings.ToLower(strings.TrimSpace(parts[0]))
		prompt := strings.TrimSpace(parts[1])
		if prompt == "" {
			return nil, fmt.Errorf("invalid matrix line %d: empty prompt", lineNo)
		}
		switch mode {
		case "bypass", "full-auto", "any":
		default:
			return nil, fmt.Errorf("invalid matrix line %d: mode must be bypass|full-auto|any", lineNo)
		}
		cases = append(cases, smokePromptMatrixCase{
			ExpectedMode: mode,
			Prompt:       prompt,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("matrix file has no probe rows")
	}
	return cases, nil
}

func runSmokePromptMatrixProbe(binPath, projectRoot, matrixFile string, modelOptional ...string) ([]smokeMatrixProbe, error) {
	model := ""
	if len(modelOptional) > 0 {
		model = modelOptional[0]
	}
	cases, err := parseSmokePromptMatrixFile(matrixFile)
	if err != nil {
		return nil, err
	}
	probes := make([]smokeMatrixProbe, 0, len(cases))
	for _, matrixCase := range cases {
		args := []string{
			"session", "spawn",
			"--agent", "codex",
			"--mode", "exec",
			"--project-root", projectRoot,
			"--prompt", matrixCase.Prompt,
			"--dry-run",
			"--detect-nested",
		}
		if strings.TrimSpace(model) != "" {
			args = append(args, "--model", model)
		}
		args = append(args, "--json")
		raw, stderrText, runErr := runLisaSubcommandFn(binPath, args...)
		if runErr != nil {
			return probes, fmt.Errorf("matrix probe failed: %s", formatSmokeSubcommandError("session spawn dry-run", runErr, stderrText))
		}
		var payload struct {
			Command         string               `json:"command"`
			NestedDetection nestedCodexDetection `json:"nestedDetection"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
			return probes, fmt.Errorf("failed parsing matrix probe output: %w", err)
		}

		actualMode := "other"
		switch {
		case strings.Contains(payload.Command, "--dangerously-bypass-approvals-and-sandbox"):
			actualMode = "bypass"
		case strings.Contains(payload.Command, "--full-auto"):
			actualMode = "full-auto"
		}
		pass := matrixCase.ExpectedMode == "any" || matrixCase.ExpectedMode == actualMode
		probe := smokeMatrixProbe{
			ExpectedMode: matrixCase.ExpectedMode,
			ActualMode:   actualMode,
			Prompt:       matrixCase.Prompt,
			Pass:         pass,
			Detection:    payload.NestedDetection,
			Command:      payload.Command,
		}
		probes = append(probes, probe)
		if !pass {
			return probes, fmt.Errorf("matrix mismatch for prompt %q: expected %s got %s (reason=%s)", matrixCase.Prompt, matrixCase.ExpectedMode, actualMode, payload.NestedDetection.Reason)
		}
	}
	return probes, nil
}

func smokePromptForStyle(style string) (string, bool) {
	switch style {
	case "dot-slash":
		return "Use ./lisa for all child orchestration.", true
	case "spawn":
		return "Run lisa session spawn inside the spawned agent.", true
	case "nested":
		return "Build a nested lisa chain and report markers.", true
	case "neutral":
		return "No nesting requested here.", false
	default:
		return "", false
	}
}

func runSmokePromptStyleProbe(binPath, projectRoot, style string, modelOptional ...string) (*smokePromptProbe, error) {
	model := ""
	if len(modelOptional) > 0 {
		model = modelOptional[0]
	}
	prompt, expectedBypass := smokePromptForStyle(style)
	if prompt == "" {
		return nil, nil
	}
	args := []string{
		"session", "spawn",
		"--agent", "codex",
		"--mode", "exec",
		"--project-root", projectRoot,
		"--prompt", prompt,
		"--dry-run",
		"--detect-nested",
	}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "--json")
	raw, stderrText, err := runLisaSubcommandFn(binPath, args...)
	if err != nil {
		return nil, fmt.Errorf("prompt probe failed: %s", formatSmokeSubcommandError("session spawn dry-run", err, stderrText))
	}
	var payload struct {
		Command         string               `json:"command"`
		NestedDetection nestedCodexDetection `json:"nestedDetection"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse prompt probe output: %w", err)
	}
	probe := &smokePromptProbe{
		Style:          style,
		Prompt:         prompt,
		ExpectedBypass: expectedBypass,
		Detection:      payload.NestedDetection,
		Command:        payload.Command,
	}
	if payload.NestedDetection.AutoBypass != expectedBypass {
		return probe, fmt.Errorf("prompt style %q expected autoBypass=%t, got %t (%s)", style, expectedBypass, payload.NestedDetection.AutoBypass, payload.NestedDetection.Reason)
	}
	return probe, nil
}

func formatSmokeSubcommandError(prefix string, err error, stderrText string) string {
	if strings.TrimSpace(stderrText) == "" {
		return fmt.Sprintf("%s: %v", prefix, err)
	}
	return fmt.Sprintf("%s: %v (stderr: %s)", prefix, err, stderrText)
}

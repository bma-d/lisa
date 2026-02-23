package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type anomalyFinding struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	Count          int    `json:"count"`
	Message        string `json:"message"`
	Recommendation string `json:"recommendation"`
}

func cmdSessionAnomaly(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	events := 80
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session anomaly")
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = strings.TrimSpace(args[i+1])
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			projectRootExplicit = true
			i++
		case "--events":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --events")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--events")
			if err != nil {
				return commandError(jsonOut, "invalid_events", err.Error())
			}
			events = n
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}
	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--session is required")
	}

	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	status, err := computeSessionStatusFn(session, projectRoot, "auto", "auto", false, 0)
	if err != nil {
		return commandError(jsonOut, "status_compute_failed", err.Error())
	}
	status = normalizeStatusForSessionStatusOutput(status)
	tail, _ := readSessionEventTailFn(projectRoot, session, events)
	findings := detectSessionAnomalies(status, tail.Events)

	payload := map[string]any{
		"session":      session,
		"status":       status.Status,
		"sessionState": status.SessionState,
		"events":       len(tail.Events),
		"dropped":      tail.DroppedLines,
		"anomalies":    findings,
		"ok":           len(findings) == 0,
		"nextAction":   nextActionForState(status.SessionState),
	}
	if status.SessionState == "not_found" {
		payload["errorCode"] = "session_not_found"
	}

	if jsonOut {
		writeJSON(payload)
		if status.SessionState == "not_found" || len(findings) > 0 {
			return 1
		}
		return 0
	}
	if len(findings) == 0 {
		fmt.Println("ok")
		if status.SessionState == "not_found" {
			return 1
		}
		return 0
	}
	for _, finding := range findings {
		fmt.Printf("%s,%s,%d,%s\n", finding.Code, finding.Severity, finding.Count, finding.Message)
	}
	return 1
}

func detectSessionAnomalies(status sessionStatus, events []sessionEvent) []anomalyFinding {
	findings := make([]anomalyFinding, 0)
	if strings.TrimSpace(status.SessionState) == "not_found" {
		findings = append(findings, anomalyFinding{
			Code:           "session_not_found",
			Severity:       "high",
			Count:          1,
			Message:        "session is not present in tmux",
			Recommendation: "spawn or restore the session before monitoring",
		})
		return findings
	}

	if len(events) == 0 {
		if status.SessionState == "in_progress" || status.SessionState == "degraded" {
			findings = append(findings, anomalyFinding{
				Code:           "no_events_observed",
				Severity:       "medium",
				Count:          1,
				Message:        "active state with empty event tail",
				Recommendation: "run session explain to verify event logging",
			})
		}
		return findings
	}

	reasonCounts := map[string]int{}
	stateCounts := map[string]int{}
	maxRunReason := ""
	maxRun := 0
	curReason := ""
	curRun := 0
	expectedMismatch := 0
	degradedTail := 0
	for idx, event := range events {
		reason := strings.TrimSpace(event.Reason)
		state := strings.TrimSpace(event.State)
		reasonCounts[reason]++
		stateCounts[state]++
		if reason != "" && reason == curReason {
			curRun++
		} else {
			curReason = reason
			curRun = 1
		}
		if curRun > maxRun {
			maxRun = curRun
			maxRunReason = curReason
		}
		if strings.Contains(reason, "expected_") || strings.Contains(reason, "expectation") {
			expectedMismatch++
		}
		if idx >= len(events)-10 && state == "degraded" {
			degradedTail++
		}
	}

	if maxRun >= 6 && (status.SessionState == "in_progress" || status.SessionState == "degraded") {
		findings = append(findings, anomalyFinding{
			Code:           "reason_loop",
			Severity:       "high",
			Count:          maxRun,
			Message:        fmt.Sprintf("repeated reason %q across %d consecutive events", maxRunReason, maxRun),
			Recommendation: "run session explain --events 40 and evaluate whether monitor expectations are too strict",
		})
	}
	if degradedTail >= 3 {
		findings = append(findings, anomalyFinding{
			Code:           "degraded_retries",
			Severity:       "medium",
			Count:          degradedTail,
			Message:        "recent event tail shows repeated degraded state",
			Recommendation: "check tmux/socket health and rerun preflight",
		})
	}
	if expectedMismatch >= 2 {
		findings = append(findings, anomalyFinding{
			Code:           "expectation_churn",
			Severity:       "medium",
			Count:          expectedMismatch,
			Message:        "multiple monitor expectation mismatch events detected",
			Recommendation: "align --expect with stop condition or use until-state/jsonpath gates",
		})
	}
	if status.SessionState == "stuck" {
		findings = append(findings, anomalyFinding{
			Code:           "terminal_stuck",
			Severity:       "high",
			Count:          1,
			Message:        "session resolved to stuck",
			Recommendation: "inspect with session explain then restart with session spawn",
		})
	}
	if status.SessionState == "crashed" {
		findings = append(findings, anomalyFinding{
			Code:           "terminal_crashed",
			Severity:       "high",
			Count:          1,
			Message:        "session resolved to crashed",
			Recommendation: "capture raw output and rerun using safer command path",
		})
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity == findings[j].Severity {
			return findings[i].Code < findings[j].Code
		}
		return anomalySeverityRank(findings[i].Severity) > anomalySeverityRank(findings[j].Severity)
	})
	return findings
}

func anomalySeverityRank(value string) int {
	switch value {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func cmdSessionBudgetEnforce(args []string) int {
	from := ""
	maxTokens := 0
	maxSeconds := 0
	maxSteps := 0
	obsTokens := -1
	obsSeconds := -1
	obsSteps := -1
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session budget-enforce")
		case "--from":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --from")
			}
			from = strings.TrimSpace(args[i+1])
			i++
		case "--max-tokens":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-tokens")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--max-tokens")
			if err != nil {
				return commandError(jsonOut, "invalid_max_tokens", err.Error())
			}
			maxTokens = n
			i++
		case "--max-seconds":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-seconds")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--max-seconds")
			if err != nil {
				return commandError(jsonOut, "invalid_max_seconds", err.Error())
			}
			maxSeconds = n
			i++
		case "--max-steps":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-steps")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--max-steps")
			if err != nil {
				return commandError(jsonOut, "invalid_max_steps", err.Error())
			}
			maxSteps = n
			i++
		case "--tokens":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --tokens")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--tokens")
			if err != nil {
				return commandError(jsonOut, "invalid_tokens", err.Error())
			}
			obsTokens = n
			i++
		case "--seconds":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --seconds")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--seconds")
			if err != nil {
				return commandError(jsonOut, "invalid_seconds", err.Error())
			}
			obsSeconds = n
			i++
		case "--steps":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --steps")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--steps")
			if err != nil {
				return commandError(jsonOut, "invalid_steps", err.Error())
			}
			obsSteps = n
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	if maxTokens == 0 && maxSeconds == 0 && maxSteps == 0 {
		return commandError(jsonOut, "missing_budget_limits", "at least one max limit is required")
	}

	observed := map[string]int{"tokens": obsTokens, "seconds": obsSeconds, "steps": obsSteps}
	if from != "" {
		parsed, err := loadAnyJSONMap(from)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_from", "failed loading --from: %v", err)
		}
		extractObservedBudgets(parsed, observed)
	}
	if observed["tokens"] < 0 {
		observed["tokens"] = 0
	}
	if observed["seconds"] < 0 {
		observed["seconds"] = 0
	}
	if observed["steps"] < 0 {
		observed["steps"] = 0
	}

	violations := make([]map[string]any, 0)
	if maxTokens > 0 && observed["tokens"] > maxTokens {
		violations = append(violations, map[string]any{"metric": "tokens", "observed": observed["tokens"], "limit": maxTokens})
	}
	if maxSeconds > 0 && observed["seconds"] > maxSeconds {
		violations = append(violations, map[string]any{"metric": "seconds", "observed": observed["seconds"], "limit": maxSeconds})
	}
	if maxSteps > 0 && observed["steps"] > maxSteps {
		violations = append(violations, map[string]any{"metric": "steps", "observed": observed["steps"], "limit": maxSteps})
	}

	ok := len(violations) == 0
	payload := map[string]any{
		"ok": ok,
		"observed": map[string]int{
			"tokens":  observed["tokens"],
			"seconds": observed["seconds"],
			"steps":   observed["steps"],
		},
		"limits": map[string]int{
			"maxTokens":  maxTokens,
			"maxSeconds": maxSeconds,
			"maxSteps":   maxSteps,
		},
		"violations": violations,
	}
	if !ok {
		payload["errorCode"] = "budget_limit_exceeded"
	}
	if jsonOut {
		writeJSON(payload)
		return boolExit(ok)
	}
	if ok {
		fmt.Println("ok")
		return 0
	}
	for _, violation := range violations {
		fmt.Printf("violation=%s observed=%v limit=%v\n", violation["metric"], violation["observed"], violation["limit"])
	}
	return 1
}

func extractObservedBudgets(payload map[string]any, observed map[string]int) {
	if value, ok := numberFromAny(payload["totalTokens"]); ok {
		observed["tokens"] = maxInt(observed["tokens"], value)
	}
	if value, ok := numberFromAny(payload["tokens"]); ok {
		observed["tokens"] = maxInt(observed["tokens"], value)
	}
	if value, ok := numberFromAny(payload["totalSeconds"]); ok {
		observed["seconds"] = maxInt(observed["seconds"], value)
	}
	if value, ok := numberFromAny(payload["seconds"]); ok {
		observed["seconds"] = maxInt(observed["seconds"], value)
	}
	if value, ok := numberFromAny(payload["totalSteps"]); ok {
		observed["steps"] = maxInt(observed["steps"], value)
	}
	if stepsRaw, ok := payload["steps"].([]any); ok {
		observed["steps"] = maxInt(observed["steps"], len(stepsRaw))
	}
	if costRaw, ok := payload["costEstimate"].(map[string]any); ok {
		if value, ok := numberFromAny(costRaw["totalTokens"]); ok {
			observed["tokens"] = maxInt(observed["tokens"], value)
		}
		if value, ok := numberFromAny(costRaw["totalSeconds"]); ok {
			observed["seconds"] = maxInt(observed["seconds"], value)
		}
		if stepsRaw, ok := costRaw["steps"].([]any); ok {
			observed["steps"] = maxInt(observed["steps"], len(stepsRaw))
		}
	}
}

func numberFromAny(raw any) (int, bool) {
	switch typed := raw.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func cmdSessionReplay(args []string) int {
	fromCheckpoint := ""
	projectRoot := ""
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session replay")
		case "--from-checkpoint":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --from-checkpoint")
			}
			fromCheckpoint = strings.TrimSpace(args[i+1])
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = strings.TrimSpace(args[i+1])
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}
	if fromCheckpoint == "" {
		return commandError(jsonOut, "missing_required_flag", "--from-checkpoint is required")
	}
	checkpointPath, err := expandAndCleanPath(fromCheckpoint)
	if err != nil {
		return commandErrorf(jsonOut, "invalid_checkpoint_path", "invalid --from-checkpoint: %v", err)
	}
	bundle, err := loadCheckpointBundle(checkpointPath)
	if err != nil {
		return commandErrorf(jsonOut, "checkpoint_read_failed", "failed reading checkpoint: %v", err)
	}
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = strings.TrimSpace(bundle.ProjectRoot)
	}
	if root == "" {
		root = getPWD()
	}
	root = canonicalProjectRoot(root)

	steps := replayStepsFromCheckpoint(bundle, root)
	payload := map[string]any{
		"session":         bundle.Session,
		"projectRoot":     root,
		"checkpoint":      checkpointPath,
		"nextAction":      bundle.NextAction,
		"sessionState":    bundle.SessionState,
		"deterministicId": fmt.Sprintf("%s|%s|%s|%d", bundle.Session, bundle.SessionState, bundle.NextAction, bundle.NextOffset),
		"steps":           steps,
		"ok":              true,
	}
	if jsonOut {
		writeJSON(payload)
		return 0
	}
	for _, step := range steps {
		fmt.Println(step["command"])
	}
	return 0
}

func replayStepsFromCheckpoint(bundle sessionCheckpointBundle, projectRoot string) []map[string]any {
	session := strings.TrimSpace(bundle.Session)
	nextAction := strings.TrimSpace(bundle.NextAction)
	steps := []map[string]any{
		{
			"id":      "status",
			"command": "./lisa session status --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --json-min",
			"reason":  "replay baseline status",
		},
	}
	cmd := ""
	switch nextAction {
	case "session send":
		cmd = "./lisa session send --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --text " + shellQuote("Continue from checkpoint context.") + " --enter --json-min"
	case "session monitor":
		cmd = "./lisa session monitor --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --expect terminal --json-min"
	case "session explain":
		cmd = "./lisa session explain --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --events 30 --json-min"
	case "session spawn":
		cmd = "./lisa session spawn --agent codex --mode interactive --project-root " + shellQuote(projectRoot) + " --prompt " + shellQuote("Resume from checkpoint replay and continue.") + " --model gpt-5.3-codex-spark --json"
	default:
		delta := bundle.NextOffset
		if delta < 0 {
			delta = 0
		}
		cmd = "./lisa session capture --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --raw --delta-from " + strconv.Itoa(delta) + " --json-min"
	}
	steps = append(steps, map[string]any{
		"id":      "next",
		"command": cmd,
		"reason":  "checkpoint nextAction replay",
	})
	return steps
}

func loadAnyJSONMap(from string) (map[string]any, error) {
	source := strings.TrimSpace(from)
	if source == "" {
		return nil, fmt.Errorf("empty source")
	}
	var raw []byte
	var err error
	switch source {
	case "-":
		raw, err = io.ReadAll(os.Stdin)
	default:
		path, resolveErr := expandAndCleanPath(source)
		if resolveErr != nil {
			return nil, resolveErr
		}
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

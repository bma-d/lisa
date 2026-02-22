package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type sessionHandoffItem struct {
	At     string `json:"at,omitempty"`
	Type   string `json:"type,omitempty"`
	State  string `json:"state,omitempty"`
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type sessionTreeDeltaState struct {
	Rows map[string]string `json:"rows"`
}

type contextPackStrategyConfig struct {
	Name        string
	Events      int
	Lines       int
	TokenBudget int
}

func cmdSessionHandoff(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	events := 8
	deltaFrom := -1
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session handoff")
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
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agentHint = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			modeHint = args[i+1]
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
		case "--delta-from":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --delta-from")
			}
			offset, parseErr := parseNonNegativeIntFlag(args[i+1], "--delta-from")
			if parseErr != nil {
				return commandError(jsonOut, "invalid_delta_from", parseErr.Error())
			}
			deltaFrom = offset
			i++
		case "--json":
			jsonOut = true
		case "--json-min":
			jsonMin = true
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

	var err error
	agentHint, err = parseAgentHint(agentHint)
	if err != nil {
		return commandError(jsonOut, "invalid_agent_hint", err.Error())
	}
	modeHint, err = parseModeHint(modeHint)
	if err != nil {
		return commandError(jsonOut, "invalid_mode_hint", err.Error())
	}

	status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, false, 0)
	if err != nil {
		return commandError(jsonOut, "status_compute_failed", err.Error())
	}
	status = normalizeStatusForSessionStatusOutput(status)
	items := make([]sessionHandoffItem, 0)
	droppedRecent := 0
	nextDeltaOffset := -1
	if deltaFrom >= 0 {
		deltaItems, deltaDropped, deltaNext, deltaErr := readSessionHandoffDelta(projectRoot, session, deltaFrom, events)
		if deltaErr == nil {
			items = deltaItems
			droppedRecent = deltaDropped
			nextDeltaOffset = deltaNext
		}
	} else {
		tail, _ := readSessionEventTailFn(projectRoot, session, events)
		droppedRecent = tail.DroppedLines
		items = make([]sessionHandoffItem, 0, len(tail.Events))
		for _, event := range tail.Events {
			items = append(items, sessionHandoffItem{
				At:     event.At,
				Type:   event.Type,
				State:  event.State,
				Status: event.Status,
				Reason: event.Reason,
			})
		}
	}
	nextOffset := computeSessionCaptureNextOffset(session)
	nextAction := nextActionForState(status.SessionState)
	summary := fmt.Sprintf("state=%s reason=%s next=%s", status.SessionState, status.ClassificationReason, nextAction)

	if jsonOut {
		payload := map[string]any{
			"session":      session,
			"status":       status.Status,
			"sessionState": status.SessionState,
			"reason":       status.ClassificationReason,
			"nextAction":   nextAction,
			"nextOffset":   nextOffset,
			"summary":      summary,
		}
		if !jsonMin {
			payload["projectRoot"] = projectRoot
			payload["recent"] = items
			payload["droppedRecent"] = droppedRecent
		}
		if deltaFrom >= 0 {
			payload["deltaFrom"] = deltaFrom
			payload["nextDeltaOffset"] = nextDeltaOffset
			payload["deltaCount"] = len(items)
			if jsonMin {
				payload["recent"] = items
			}
		}
		if status.SessionState == "not_found" {
			payload["errorCode"] = "session_not_found"
		}
		writeJSON(payload)
		if status.SessionState == "not_found" {
			return 1
		}
		return 0
	}

	fmt.Println(summary)
	if len(items) > 0 {
		fmt.Println("recent:")
		for _, item := range items {
			fmt.Printf("- %s %s %s %s\n", item.At, item.Type, item.State, item.Reason)
		}
	}
	if deltaFrom >= 0 {
		fmt.Printf("delta_from=%d next_delta_offset=%d\n", deltaFrom, nextDeltaOffset)
	}
	if status.SessionState == "not_found" {
		return 1
	}
	return 0
}

func cmdSessionContextPack(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	events := 8
	lines := 120
	tokenBudget := 700
	strategy := "balanced"
	eventsSet := false
	linesSet := false
	tokenBudgetSet := false
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session context-pack")
		case "--for", "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for %s", args[i])
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
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agentHint = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			modeHint = args[i+1]
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
			eventsSet = true
			i++
		case "--lines":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --lines")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--lines")
			if err != nil {
				return commandError(jsonOut, "invalid_lines", err.Error())
			}
			lines = n
			linesSet = true
			i++
		case "--token-budget":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --token-budget")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--token-budget")
			if err != nil {
				return commandError(jsonOut, "invalid_token_budget", err.Error())
			}
			tokenBudget = n
			tokenBudgetSet = true
			i++
		case "--strategy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --strategy")
			}
			strategy = args[i+1]
			i++
		case "--json":
			jsonOut = true
		case "--json-min":
			jsonMin = true
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--for is required")
	}
	strategyConfig, err := parseContextPackStrategy(strategy)
	if err != nil {
		return commandError(jsonOut, "invalid_strategy", err.Error())
	}
	if !eventsSet {
		events = strategyConfig.Events
	}
	if !linesSet {
		lines = strategyConfig.Lines
	}
	if !tokenBudgetSet {
		tokenBudget = strategyConfig.TokenBudget
	}

	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	agentHint, err = parseAgentHint(agentHint)
	if err != nil {
		return commandError(jsonOut, "invalid_agent_hint", err.Error())
	}
	modeHint, err = parseModeHint(modeHint)
	if err != nil {
		return commandError(jsonOut, "invalid_mode_hint", err.Error())
	}

	status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, false, 0)
	if err != nil {
		return commandError(jsonOut, "status_compute_failed", err.Error())
	}
	status = normalizeStatusForSessionStatusOutput(status)
	tail, _ := readSessionEventTailFn(projectRoot, session, events)
	recent := make([]string, 0, len(tail.Events))
	for _, event := range tail.Events {
		recent = append(recent, fmt.Sprintf("%s %s/%s %s", event.At, event.State, event.Status, event.Reason))
	}

	captureTail := ""
	if tmuxHasSessionFn(session) {
		if capture, captureErr := tmuxCapturePaneFn(session, lines); captureErr == nil {
			captureTail = strings.Join(trimLines(filterCaptureNoise(capture)), "\n")
		}
	}
	if captureTail == "" {
		captureTail = "(no live capture)"
	}

	packRaw := buildContextPackRaw(strategyConfig.Name, session, status, recent, captureTail)
	pack, truncated := truncateToTokenBudget(packRaw, tokenBudget)
	nextOffset := computeSessionCaptureNextOffset(session)

	if jsonOut {
		payload := map[string]any{
			"session":      session,
			"sessionState": status.SessionState,
			"status":       status.Status,
			"reason":       status.ClassificationReason,
			"nextAction":   nextActionForState(status.SessionState),
			"nextOffset":   nextOffset,
			"strategy":     strategyConfig.Name,
			"tokenBudget":  tokenBudget,
			"truncated":    truncated,
			"pack":         pack,
		}
		if !jsonMin {
			payload["projectRoot"] = projectRoot
			payload["events"] = len(recent)
			payload["droppedRecent"] = tail.DroppedLines
		}
		if status.SessionState == "not_found" {
			payload["errorCode"] = "session_not_found"
		}
		writeJSON(payload)
		if status.SessionState == "not_found" {
			return 1
		}
		return 0
	}

	fmt.Println(pack)
	if status.SessionState == "not_found" {
		return 1
	}
	return 0
}

func cmdSessionRoute(args []string) int {
	goal := "analysis"
	agent := "codex"
	projectRoot := getPWD()
	prompt := ""
	model := ""
	emitRunbook := false
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session route")
		case "--goal":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --goal")
			}
			goal = args[i+1]
			i++
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agent = args[i+1]
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--prompt":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --prompt")
			}
			prompt = args[i+1]
			i++
		case "--model":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --model")
			}
			model = args[i+1]
			i++
		case "--emit-runbook":
			emitRunbook = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	var err error
	goal, err = parseSessionRouteGoal(goal)
	if err != nil {
		return commandError(jsonOut, "invalid_goal", err.Error())
	}
	agent, err = parseAgent(agent)
	if err != nil {
		return commandError(jsonOut, "invalid_agent", err.Error())
	}
	projectRoot = canonicalProjectRoot(projectRoot)

	mode, nestedPolicy, nestingIntent, defaultPrompt, defaultModel := sessionRouteDefaults(goal)
	if strings.TrimSpace(prompt) == "" {
		prompt = defaultPrompt
	}
	if strings.TrimSpace(model) == "" {
		model = defaultModel
	}
	model, err = parseModel(model)
	if err != nil {
		return commandError(jsonOut, "invalid_model", err.Error())
	}

	agentArgs := ""
	agentArgs, err = applyModelToAgentArgs(agent, agentArgs, model)
	if err != nil {
		return commandError(jsonOut, "invalid_model_configuration", err.Error())
	}
	detection, effectiveArgs, err := applyNestedPolicyToAgentArgs(agent, mode, prompt, agentArgs, nestedPolicy, nestingIntent)
	if err != nil {
		return commandError(jsonOut, "invalid_nested_policy_combination", err.Error())
	}
	command, err := buildAgentCommandWithOptions(agent, mode, prompt, effectiveArgs, true)
	if err != nil {
		return commandError(jsonOut, "build_command_failed", err.Error())
	}

	rationale := []string{
		fmt.Sprintf("goal=%s", goal),
		fmt.Sprintf("mode=%s", mode),
		fmt.Sprintf("nested_policy=%s", nestedPolicy),
		fmt.Sprintf("nesting_intent=%s", nestingIntent),
	}
	if model != "" {
		rationale = append(rationale, "model="+model)
	}
	if detection.Reason != "" {
		rationale = append(rationale, "nested_reason="+detection.Reason)
	}
	monitorHint := "session monitor --expect terminal --json"
	if mode == "interactive" {
		monitorHint = "session monitor --stop-on-waiting true --json"
	}

	payload := map[string]any{
		"goal":            goal,
		"projectRoot":     projectRoot,
		"agent":           agent,
		"mode":            mode,
		"nestedPolicy":    nestedPolicy,
		"nestingIntent":   nestingIntent,
		"prompt":          prompt,
		"model":           model,
		"command":         command,
		"monitorHint":     monitorHint,
		"nestedDetection": detection,
		"rationale":       rationale,
	}
	if emitRunbook {
		payload["runbook"] = buildRouteRunbook(projectRoot, agent, mode, nestedPolicy, nestingIntent, prompt, model)
	}
	if jsonOut {
		writeJSON(payload)
		return 0
	}
	fmt.Printf("%s\n%s\n", command, strings.Join(rationale, " | "))
	return 0
}

func cmdSessionGuard(args []string) int {
	sharedTmux := false
	commandText := ""
	projectRoot := canonicalProjectRoot(getPWD())
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session guard")
		case "--shared-tmux":
			sharedTmux = true
		case "--command":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --command")
			}
			commandText = args[i+1]
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = canonicalProjectRoot(args[i+1])
			i++
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	if !sharedTmux {
		return commandError(jsonOut, "missing_required_flag", "--shared-tmux is required")
	}

	defaultSessions := []string{}
	out, err := runCmd("tmux", "list-sessions", "-F", "#S")
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				defaultSessions = append(defaultSessions, line)
			}
		}
	}

	warnings := []string{}
	if len(defaultSessions) > 0 {
		warnings = append(warnings, "default tmux server has active sessions")
		warnings = append(warnings, "avoid cleanup --include-tmux-default without --dry-run")
		warnings = append(warnings, "avoid session kill-all without --project-only --project-root")
	}

	commandRisk := "low"
	lowerCommand := strings.ToLower(strings.TrimSpace(commandText))
	if lowerCommand != "" {
		if strings.Contains(lowerCommand, "cleanup") && strings.Contains(lowerCommand, "--include-tmux-default") {
			commandRisk = "high"
			warnings = append(warnings, "command targets tmux default server")
		}
		if strings.Contains(lowerCommand, "session kill-all") && !strings.Contains(lowerCommand, "--project-only") {
			commandRisk = "high"
			warnings = append(warnings, "kill-all without --project-only can impact unrelated sessions")
		}
	}

	safe := len(defaultSessions) == 0 && commandRisk != "high"
	if jsonOut {
		payload := map[string]any{
			"sharedTmux":          true,
			"projectRoot":         projectRoot,
			"defaultSessionCount": len(defaultSessions),
			"defaultSessions":     defaultSessions,
			"command":             commandText,
			"commandRisk":         commandRisk,
			"safe":                safe,
			"warnings":            warnings,
		}
		if !safe {
			payload["errorCode"] = "shared_tmux_risk_detected"
		}
		writeJSON(payload)
		return boolExit(safe)
	}

	fmt.Printf("safe=%t default_sessions=%d command_risk=%s\n", safe, len(defaultSessions), commandRisk)
	for _, warning := range warnings {
		fmt.Printf("- %s\n", warning)
	}
	return boolExit(safe)
}

func parseSessionRouteGoal(goal string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(goal)) {
	case "analysis":
		return "analysis", nil
	case "exec", "execution":
		return "exec", nil
	case "nested":
		return "nested", nil
	default:
		return "", fmt.Errorf("invalid --goal: %s (expected nested|analysis|exec)", goal)
	}
}

func sessionRouteDefaults(goal string) (mode, nestedPolicy, nestingIntent, prompt, model string) {
	switch goal {
	case "nested":
		return "exec", "force", "nested", "Create nested lisa workers and report markers.", "gpt-5.3-codex-spark"
	case "exec":
		return "exec", "off", "neutral", "Run the task and return concise final output.", "gpt-5.3-codex-spark"
	default:
		return "interactive", "off", "neutral", "Analyze current task and propose next concrete actions.", "gpt-5.3-codex-spark"
	}
}

func parseContextPackStrategy(raw string) (contextPackStrategyConfig, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "balanced":
		return contextPackStrategyConfig{Name: "balanced", Events: 8, Lines: 120, TokenBudget: 700}, nil
	case "terse":
		return contextPackStrategyConfig{Name: "terse", Events: 4, Lines: 60, TokenBudget: 400}, nil
	case "full":
		return contextPackStrategyConfig{Name: "full", Events: 20, Lines: 260, TokenBudget: 1400}, nil
	default:
		return contextPackStrategyConfig{}, fmt.Errorf("invalid --strategy: %s (expected terse|balanced|full)", raw)
	}
}

func buildContextPackRaw(strategy, session string, status sessionStatus, recent []string, captureTail string) string {
	nextAction := nextActionForState(status.SessionState)
	switch strategy {
	case "terse":
		lines := []string{
			"session=" + session,
			"state=" + status.SessionState,
			"status=" + status.Status,
			"next_action=" + nextAction,
		}
		if len(recent) > 0 {
			lines = append(lines, "recent:")
			for i := len(recent) - 1; i >= 0; i-- {
				lines = append(lines, recent[i])
				if len(lines) >= 8 {
					break
				}
			}
		}
		return strings.Join(lines, "\n")
	case "full":
		lines := []string{
			"session=" + session,
			"state=" + status.SessionState,
			"status=" + status.Status,
			"reason=" + status.ClassificationReason,
			"next_action=" + nextAction,
			fmt.Sprintf("todos=%d/%d", status.TodosDone, status.TodosTotal),
			fmt.Sprintf("wait_estimate=%d", status.WaitEstimate),
			fmt.Sprintf("output_age_seconds=%d", status.OutputAgeSeconds),
			fmt.Sprintf("heartbeat_age_seconds=%d", status.HeartbeatAge),
		}
		if strings.TrimSpace(status.ActiveTask) != "" {
			lines = append(lines, "active_task="+status.ActiveTask)
		}
		if len(recent) > 0 {
			lines = append(lines, "recent_events:")
			lines = append(lines, recent...)
		}
		lines = append(lines, "capture_tail:")
		lines = append(lines, captureTail)
		return strings.Join(lines, "\n")
	default:
		lines := []string{
			"session=" + session,
			"state=" + status.SessionState,
			"status=" + status.Status,
			"reason=" + status.ClassificationReason,
			"next_action=" + nextAction,
		}
		if len(recent) > 0 {
			lines = append(lines, "recent_events:")
			lines = append(lines, recent...)
		}
		lines = append(lines, "capture_tail:")
		lines = append(lines, captureTail)
		return strings.Join(lines, "\n")
	}
}

func readSessionHandoffDelta(projectRoot, session string, offset, limit int) ([]sessionHandoffItem, int, int, error) {
	eventsPath := sessionEventsFile(projectRoot, session)
	raw, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []sessionHandoffItem{}, 0, 0, nil
		}
		return nil, 0, 0, err
	}
	lines := trimLines(string(raw))
	all := make([]sessionHandoffItem, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event sessionEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		all = append(all, sessionHandoffItem{
			At:     event.At,
			Type:   event.Type,
			State:  event.State,
			Status: event.Status,
			Reason: event.Reason,
		})
	}
	total := len(all)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	delta := append([]sessionHandoffItem{}, all[offset:]...)
	dropped := 0
	if limit > 0 && len(delta) > limit {
		dropped = len(delta) - limit
		delta = delta[len(delta)-limit:]
	}
	return delta, dropped, total, nil
}

func parseNonNegativeIntFlag(raw, flag string) (int, error) {
	value := strings.TrimSpace(raw)
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid %s: expected non-negative integer", flag)
	}
	return n, nil
}

func buildRouteRunbook(projectRoot, agent, mode, nestedPolicy, nestingIntent, prompt, model string) map[string]any {
	spawn := fmt.Sprintf("./lisa session spawn --agent %s --mode %s --nested-policy %s --nesting-intent %s --project-root %s --prompt %s --json",
		agent, mode, nestedPolicy, nestingIntent, shellQuote(projectRoot), shellQuote(prompt))
	if model != "" && agent == "codex" {
		spawn = strings.TrimSuffix(spawn, " --json") + " --model " + shellQuote(model) + " --json"
	}
	monitor := "./lisa session monitor --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --expect terminal --json"
	if mode == "interactive" {
		monitor = "./lisa session monitor --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --stop-on-waiting true --json"
	}
	return map[string]any{
		"steps": []map[string]any{
			{
				"id":      "preflight",
				"command": "./lisa session preflight --agent " + agent + " --project-root " + shellQuote(projectRoot) + " --json",
			},
			{
				"id":      "spawn",
				"command": spawn,
				"note":    "extract SESSION from spawn JSON payload",
			},
			{
				"id":      "monitor",
				"command": monitor,
			},
			{
				"id":      "capture",
				"command": "./lisa session capture --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --raw --json-min",
			},
			{
				"id":      "handoff",
				"command": "./lisa session handoff --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --json-min",
			},
			{
				"id":      "cleanup",
				"command": "./lisa session kill --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --json",
			},
		},
	}
}

func nextActionForState(state string) string {
	switch strings.TrimSpace(state) {
	case "waiting_input":
		return "session send"
	case "in_progress", "degraded":
		return "session monitor"
	case "completed":
		return "session capture"
	case "crashed", "stuck":
		return "session explain"
	case "not_found":
		return "session spawn"
	default:
		return "session status"
	}
}

func truncateToTokenBudget(input string, tokenBudget int) (string, bool) {
	if tokenBudget <= 0 {
		return "", false
	}
	maxChars := tokenBudget * 4
	if len(input) <= maxChars {
		return input, false
	}
	if maxChars <= 3 {
		return input[:maxChars], true
	}
	return input[:maxChars-3] + "...", true
}

func parseMonitorUntilState(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "":
		return "", nil
	case "waiting_input", "completed", "crashed", "stuck", "not_found", "in_progress", "degraded":
		return value, nil
	default:
		return "", fmt.Errorf("invalid --until-state: %s (expected waiting_input|completed|crashed|stuck|not_found|in_progress|degraded)", raw)
	}
}

func loadCursorOffset(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor file offset: %q", value)
	}
	if n < 0 {
		return 0, nil
	}
	return n, nil
}

func writeCursorOffset(path string, offset int) error {
	if offset < 0 {
		offset = 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(strconv.Itoa(offset)+"\n"))
}

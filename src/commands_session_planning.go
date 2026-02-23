package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type sessionPackSnapshot struct {
	Session      string
	Status       string
	SessionState string
	Reason       string
	NextAction   string
	NextOffset   int
	Pack         string
	Truncated    bool
	Events       int
	Dropped      int
}

type sessionAggregateDeltaItem struct {
	Session      string `json:"session"`
	Status       string `json:"status"`
	SessionState string `json:"sessionState"`
	NextAction   string `json:"nextAction"`
	NextOffset   int    `json:"nextOffset"`
	Pack         string `json:"pack"`
}

type sessionAggregateDeltaCursor struct {
	UpdatedAt string                               `json:"updatedAt"`
	Items     map[string]sessionAggregateDeltaItem `json:"items"`
}

func cmdSessionNext(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	budget := 480
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session next")
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
		case "--budget":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --budget")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--budget")
			if err != nil {
				return commandError(jsonOut, "invalid_budget", err.Error())
			}
			budget = n
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

	resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, projectRootExplicit)
	if resolveErr != nil {
		return commandErrorf(jsonOut, "ambiguous_project_root", "%v", resolveErr)
	}
	projectRoot = resolvedRoot
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	status, err := computeSessionStatusFn(session, projectRoot, "auto", "auto", false, 0)
	if err != nil {
		return commandError(jsonOut, "status_compute_failed", err.Error())
	}
	status = normalizeStatusForSessionStatusOutput(status)

	nextAction, command, reason := recommendedSessionNext(status, session, projectRoot, budget)
	payload := map[string]any{
		"session":            session,
		"status":             status.Status,
		"sessionState":       status.SessionState,
		"nextAction":         nextAction,
		"recommendedCommand": command,
		"reason":             reason,
		"budget":             budget,
	}
	if status.SessionState == "not_found" {
		payload["errorCode"] = "session_not_found"
	}

	if jsonOut {
		writeJSON(payload)
		if status.SessionState == "not_found" {
			return 1
		}
		return 0
	}

	fmt.Printf("%s\n", command)
	if status.SessionState == "not_found" {
		return 1
	}
	return 0
}

func recommendedSessionNext(status sessionStatus, session, projectRoot string, budget int) (string, string, string) {
	state := strings.TrimSpace(status.SessionState)
	safeBudget := budget
	if safeBudget <= 0 {
		safeBudget = 320
	}

	switch state {
	case "waiting_input":
		return "session send",
			"./lisa session send --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --text " + shellQuote("Continue from current context and produce concise next step output.") + " --enter --json-min",
			"interactive session is ready for next instruction"
	case "in_progress", "degraded":
		if safeBudget <= 250 {
			return "session snapshot",
				"./lisa session snapshot --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --json-min",
				"low token budget favors one-shot lightweight status+capture"
		}
		if safeBudget <= 600 {
			return "session packet",
				"./lisa session packet --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --token-budget " + strconv.Itoa(safeBudget) + " --json-min",
				"medium budget favors summarized packet"
		}
		return "session context-pack",
			"./lisa session context-pack --for " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --strategy balanced --token-budget " + strconv.Itoa(safeBudget) + " --json-min",
			"higher budget can carry richer handoff context"
	case "completed":
		if safeBudget <= 250 {
			return "session capture",
				"./lisa session capture --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --raw --summary --summary-style terse --token-budget " + strconv.Itoa(safeBudget) + " --json",
				"completed session with low budget should return compact summary"
		}
		if safeBudget <= 600 {
			return "session packet",
				"./lisa session packet --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --token-budget " + strconv.Itoa(safeBudget) + " --json-min",
				"completed session can be transferred as compact packet"
		}
		return "session context-pack",
			"./lisa session context-pack --for " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --strategy full --token-budget " + strconv.Itoa(safeBudget) + " --json-min",
			"higher budget can preserve more completion context"
	case "crashed", "stuck":
		return "session explain",
			"./lisa session explain --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --events 40 --json-min",
			"terminal error state needs diagnostics before continuation"
	case "not_found":
		return "session spawn",
			"./lisa session spawn --agent codex --mode interactive --project-root " + shellQuote(projectRoot) + " --prompt " + shellQuote("Resume task from latest plan and continue.") + " --model gpt-5.3-codex-spark --json",
			"session metadata exists but tmux session is missing"
	default:
		return "session status",
			"./lisa session status --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --json-min",
			"fallback to explicit status probe"
	}
}

func cmdSessionAggregate(args []string) int {
	sessionsRaw := ""
	projectRoot := getPWD()
	strategy := "balanced"
	events := 8
	lines := 120
	tokenBudget := 900
	dedupe := false
	deltaJSON := false
	cursorFile := ""
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session aggregate")
		case "--sessions":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --sessions")
			}
			sessionsRaw = strings.TrimSpace(args[i+1])
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--strategy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --strategy")
			}
			strategy = args[i+1]
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
		case "--lines":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --lines")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--lines")
			if err != nil {
				return commandError(jsonOut, "invalid_lines", err.Error())
			}
			lines = n
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
			i++
		case "--dedupe":
			dedupe = true
		case "--delta-json":
			deltaJSON = true
			jsonOut = true
		case "--cursor-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --cursor-file")
			}
			cursorFile = strings.TrimSpace(args[i+1])
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

	strategyConfig, err := parseContextPackStrategy(strategy)
	if err != nil {
		return commandError(jsonOut, "invalid_strategy", err.Error())
	}
	projectRoot = canonicalProjectRoot(projectRoot)
	if cursorFile != "" && !deltaJSON {
		return commandError(jsonOut, "cursor_file_requires_delta_json", "--cursor-file requires --delta-json")
	}
	if deltaJSON {
		if cursorFile == "" {
			cursorFile = fmt.Sprintf("/tmp/.lisa-%s-session-aggregate-delta.json", projectHash(projectRoot))
		}
		cursorFile, err = expandAndCleanPath(cursorFile)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", err)
		}
	}
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	sessions := parseCommaValues(sessionsRaw)
	if len(sessions) == 0 {
		list, listErr := tmuxListSessionsFn(false, projectRoot)
		if listErr != nil {
			return commandErrorf(jsonOut, "session_list_failed", "failed to list sessions for aggregate: %v", listErr)
		}
		sessions = list
	}
	if len(sessions) == 0 {
		return commandError(jsonOut, "no_sessions", "no sessions available for aggregation")
	}
	sort.Strings(sessions)

	perBudget := tokenBudget / len(sessions)
	if perBudget < 120 {
		perBudget = 120
	}
	items := make([]map[string]any, 0, len(sessions))
	currentDelta := map[string]sessionAggregateDeltaItem{}
	combinedParts := make([]string, 0, len(sessions)*3)
	notFound := 0
	ambiguityWarnings := make([]map[string]any, 0)

	for _, session := range sessions {
		var ambiguityWarning map[string]any
		resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, false)
		if resolveErr != nil {
			if isSessionMetaAmbiguousError(resolveErr) {
				ambiguityWarning = map[string]any{
					"session":   session,
					"errorCode": "ambiguous_project_root",
					"message":   resolveErr.Error(),
					"fallback":  projectRoot,
				}
				ambiguityWarnings = append(ambiguityWarnings, ambiguityWarning)
			}
			resolvedRoot = projectRoot
		}
		snapshot, snapErr := buildSessionPackSnapshot(session, resolvedRoot, "auto", "auto", strategyConfig, events, lines, perBudget, nil)
		if snapErr != nil {
			errorItem := map[string]any{
				"session":   session,
				"error":     snapErr.Error(),
				"errorCode": "aggregate_snapshot_failed",
			}
			if ambiguityWarning != nil {
				errorItem["warning"] = ambiguityWarning
			}
			items = append(items, errorItem)
			continue
		}
		if snapshot.SessionState == "not_found" {
			notFound++
		}
		item := map[string]any{
			"session":      snapshot.Session,
			"status":       snapshot.Status,
			"sessionState": snapshot.SessionState,
			"nextAction":   snapshot.NextAction,
			"nextOffset":   snapshot.NextOffset,
			"truncated":    snapshot.Truncated,
			"pack":         snapshot.Pack,
		}
		if !jsonMin {
			item["reason"] = snapshot.Reason
			item["events"] = snapshot.Events
			item["droppedRecent"] = snapshot.Dropped
			item["projectRoot"] = resolvedRoot
		}
		if ambiguityWarning != nil {
			item["warning"] = ambiguityWarning
		}
		items = append(items, item)
		currentDelta[session] = sessionAggregateDeltaItem{
			Session:      snapshot.Session,
			Status:       snapshot.Status,
			SessionState: snapshot.SessionState,
			NextAction:   snapshot.NextAction,
			NextOffset:   snapshot.NextOffset,
			Pack:         snapshot.Pack,
		}

		combinedParts = append(combinedParts,
			"session="+snapshot.Session,
			snapshot.Pack,
			"---",
		)
	}

	combinedRaw := strings.Join(combinedParts, "\n")
	dedupeRemoved := 0
	if dedupe {
		combinedRaw, dedupeRemoved = dedupeSemanticPackLines(combinedRaw)
	}
	combinedPack, combinedTruncated := truncateToTokenBudget(combinedRaw, tokenBudget)
	deltaAdded := []sessionAggregateDeltaItem{}
	deltaRemoved := []sessionAggregateDeltaItem{}
	deltaChanged := []sessionAggregateDeltaItem{}
	if deltaJSON {
		prev, prevErr := loadSessionAggregateDeltaCursor(cursorFile)
		if prevErr != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", prevErr)
		}
		for name, nowItem := range currentDelta {
			prevItem, ok := prev.Items[name]
			if !ok {
				deltaAdded = append(deltaAdded, nowItem)
				continue
			}
			if !sessionAggregateDeltaItemsEqual(nowItem, prevItem) {
				deltaChanged = append(deltaChanged, nowItem)
			}
		}
		for name, prevItem := range prev.Items {
			if _, ok := currentDelta[name]; !ok {
				deltaRemoved = append(deltaRemoved, prevItem)
			}
		}
		sort.Slice(deltaAdded, func(i, j int) bool { return deltaAdded[i].Session < deltaAdded[j].Session })
		sort.Slice(deltaRemoved, func(i, j int) bool { return deltaRemoved[i].Session < deltaRemoved[j].Session })
		sort.Slice(deltaChanged, func(i, j int) bool { return deltaChanged[i].Session < deltaChanged[j].Session })
		if err := saveSessionAggregateDeltaCursor(cursorFile, sessionAggregateDeltaCursor{
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			Items:     currentDelta,
		}); err != nil {
			return commandErrorf(jsonOut, "cursor_file_write_failed", "failed writing --cursor-file: %v", err)
		}
	}
	payload := map[string]any{
		"sessionCount":  len(sessions),
		"items":         items,
		"combinedPack":  combinedPack,
		"truncated":     combinedTruncated,
		"tokenBudget":   tokenBudget,
		"strategy":      strategyConfig.Name,
		"dedupe":        dedupe,
		"dedupeRemoved": dedupeRemoved,
	}
	if notFound > 0 {
		payload["errorCode"] = "aggregate_partial_not_found"
		payload["notFoundCount"] = notFound
	}
	if deltaJSON {
		payload["delta"] = map[string]any{
			"added":   deltaAdded,
			"removed": deltaRemoved,
			"changed": deltaChanged,
			"count":   len(deltaAdded) + len(deltaRemoved) + len(deltaChanged),
		}
		payload["cursorFile"] = cursorFile
	}
	if !jsonMin {
		payload["projectRoot"] = projectRoot
		payload["sessions"] = sessions
	}
	if len(ambiguityWarnings) > 0 {
		payload["warnings"] = ambiguityWarnings
	}

	if jsonOut {
		writeJSON(payload)
		if notFound > 0 {
			return 1
		}
		return 0
	}
	for _, warning := range ambiguityWarnings {
		fmt.Fprintf(os.Stderr, "warning: session=%s %s\n", mapStringValue(warning, "session"), mapStringValue(warning, "message"))
	}
	if deltaJSON {
		fmt.Printf("delta added=%d removed=%d changed=%d cursor=%s\n", len(deltaAdded), len(deltaRemoved), len(deltaChanged), cursorFile)
		if notFound > 0 {
			return 1
		}
		return 0
	}
	fmt.Println(combinedPack)
	if notFound > 0 {
		return 1
	}
	return 0
}

func dedupeSemanticPackLines(raw string) (string, int) {
	lines := strings.Split(raw, "\n")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(lines))
	removed := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "---" {
			out = append(out, line)
			continue
		}
		key := strings.ToLower(trimmed)
		// Preserve per-session boundaries even when similar wording repeats.
		if strings.HasPrefix(key, "session=") {
			out = append(out, line)
			continue
		}
		if _, ok := seen[key]; ok {
			removed++
			continue
		}
		seen[key] = struct{}{}
		out = append(out, line)
	}
	return strings.Join(out, "\n"), removed
}

func cmdSessionPromptLint(args []string) int {
	agent := "codex"
	mode := "exec"
	nestedPolicy := "auto"
	nestingIntent := "auto"
	prompt := ""
	model := ""
	projectRoot := getPWD()
	markersRaw := ""
	budget := 320
	strict := false
	rewrite := false
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session prompt-lint")
		case "--agent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --agent")
			}
			agent = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			mode = args[i+1]
			i++
		case "--nested-policy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nested-policy")
			}
			nestedPolicy = args[i+1]
			i++
		case "--nesting-intent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nesting-intent")
			}
			nestingIntent = args[i+1]
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
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--markers":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --markers")
			}
			markersRaw = args[i+1]
			i++
		case "--budget":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --budget")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--budget")
			if err != nil {
				return commandError(jsonOut, "invalid_budget", err.Error())
			}
			budget = n
			i++
		case "--strict":
			strict = true
		case "--rewrite":
			rewrite = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	if strings.TrimSpace(prompt) == "" {
		return commandError(jsonOut, "missing_required_flag", "--prompt is required")
	}
	var err error
	agent, err = parseAgent(agent)
	if err != nil {
		return commandError(jsonOut, "invalid_agent", err.Error())
	}
	mode, err = parseMode(mode)
	if err != nil {
		return commandError(jsonOut, "invalid_mode", err.Error())
	}
	nestedPolicy, err = parseNestedPolicy(nestedPolicy)
	if err != nil {
		return commandError(jsonOut, "invalid_nested_policy", err.Error())
	}
	nestingIntent, err = parseNestingIntent(nestingIntent)
	if err != nil {
		return commandError(jsonOut, "invalid_nesting_intent", err.Error())
	}
	model, err = parseModel(model)
	if err != nil {
		return commandError(jsonOut, "invalid_model", err.Error())
	}
	projectRoot = canonicalProjectRoot(projectRoot)

	agentArgs := ""
	agentArgs, err = applyModelToAgentArgs(agent, agentArgs, model)
	if err != nil {
		return commandError(jsonOut, "invalid_model_configuration", err.Error())
	}
	detection, effectiveArgs, err := applyNestedPolicyToAgentArgs(agent, mode, prompt, agentArgs, nestedPolicy, nestingIntent)
	if err != nil {
		return commandError(jsonOut, "invalid_nested_policy_combination", err.Error())
	}
	tokenEstimate := estimatePromptTokens(prompt)
	warnings := make([]map[string]any, 0)
	score := 100

	if tokenEstimate > budget {
		warnings = append(warnings, map[string]any{
			"code":     "prompt_over_budget",
			"severity": "medium",
			"message":  fmt.Sprintf("estimated prompt tokens %d exceed budget %d", tokenEstimate, budget),
		})
		score -= 18
	}

	markers := parseCommaValues(markersRaw)
	collisionMarkers := make([]string, 0)
	for _, marker := range markers {
		if strings.Contains(prompt, marker) {
			collisionMarkers = append(collisionMarkers, marker)
		}
	}
	if len(collisionMarkers) > 0 {
		warnings = append(warnings, map[string]any{
			"code":     "prompt_contains_marker",
			"severity": "high",
			"message":  "prompt includes marker strings that may trigger early monitor success",
			"markers":  collisionMarkers,
		})
		score -= 30
	}

	lowerPrompt := strings.ToLower(prompt)
	if agent == "codex" && mode == "exec" && strings.Contains(lowerPrompt, "nested") && !detection.AutoBypass {
		warnings = append(warnings, map[string]any{
			"code":     "nested_hint_missing",
			"severity": "medium",
			"message":  "nested intent detected but bypass hint did not trigger; include './lisa' or 'lisa session spawn'",
		})
		score -= 15
	}
	if detection.AutoBypass && detection.HasFullAutoArg {
		warnings = append(warnings, map[string]any{
			"code":     "bypass_full_auto_conflict",
			"severity": "high",
			"message":  "nested bypass should not be combined with --full-auto",
		})
		score -= 25
	}
	if score < 0 {
		score = 0
	}
	highSeverityWarnings := 0
	for _, warning := range warnings {
		if strings.EqualFold(fmt.Sprintf("%v", warning["severity"]), "high") {
			highSeverityWarnings++
		}
	}
	strictFailed := strict && highSeverityWarnings > 0

	rewrites := []string{}
	if rewrite {
		rewrites = nestedRewriteSuggestions(prompt, detection)
	}
	recommendedPrompt := ""
	if len(rewrites) > 0 {
		recommendedPrompt = rewrites[0]
	}
	payload := map[string]any{
		"agent":              agent,
		"mode":               mode,
		"projectRoot":        projectRoot,
		"budget":             budget,
		"prompt":             prompt,
		"tokenEstimate":      tokenEstimate,
		"nestedDetection":    detection,
		"effectiveAgentArgs": effectiveArgs,
		"warnings":           warnings,
		"score":              score,
		"strict":             strict,
		"highSeverityCount":  highSeverityWarnings,
	}
	if rewrite {
		payload["rewrites"] = rewrites
		payload["recommendedPrompt"] = recommendedPrompt
	}
	if strictFailed {
		payload["errorCode"] = "prompt_lint_strict_failed"
	}
	if model != "" {
		payload["model"] = model
	}
	if command, buildErr := buildAgentCommandWithOptions(agent, mode, prompt, effectiveArgs, true); buildErr == nil {
		payload["command"] = command
	}
	if jsonOut {
		writeJSON(payload)
		if strictFailed {
			return 1
		}
		return 0
	}
	fmt.Printf("score=%d warnings=%d\n", score, len(warnings))
	if rewrite && recommendedPrompt != "" {
		fmt.Println(recommendedPrompt)
	}
	if strictFailed {
		fmt.Fprintln(os.Stderr, "strict lint failed: high-severity warnings detected")
		return 1
	}
	return 0
}

func cmdSessionDiffPack(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	strategy := "balanced"
	events := 8
	lines := 120
	tokenBudget := 700
	cursorFile := ""
	redactRaw := ""
	semanticOnly := false
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session diff-pack")
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
		case "--strategy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --strategy")
			}
			strategy = args[i+1]
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
		case "--lines":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --lines")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--lines")
			if err != nil {
				return commandError(jsonOut, "invalid_lines", err.Error())
			}
			lines = n
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
			i++
		case "--cursor-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --cursor-file")
			}
			cursorFile = strings.TrimSpace(args[i+1])
			i++
		case "--redact":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --redact")
			}
			redactRaw = strings.TrimSpace(args[i+1])
			i++
		case "--semantic-only":
			semanticOnly = true
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
	strategyConfig, err := parseContextPackStrategy(strategy)
	if err != nil {
		return commandError(jsonOut, "invalid_strategy", err.Error())
	}
	redactRules, err := parseRedactionRules(redactRaw)
	if err != nil {
		return commandError(jsonOut, "invalid_redact_rules", err.Error())
	}

	resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, projectRootExplicit)
	if resolveErr != nil {
		return commandErrorf(jsonOut, "ambiguous_project_root", "%v", resolveErr)
	}
	projectRoot = resolvedRoot
	if cursorFile == "" {
		cursorFile = fmt.Sprintf("/tmp/.lisa-%s-session-%s-diff-pack.txt", projectHash(projectRoot), sessionArtifactID(session))
	}
	cursorFile, err = expandAndCleanPath(cursorFile)
	if err != nil {
		return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", err)
	}

	snapshot, err := buildSessionPackSnapshot(session, projectRoot, "auto", "auto", strategyConfig, events, lines, tokenBudget, redactRules)
	if err != nil {
		return commandErrorf(jsonOut, "diff_pack_build_failed", "failed to build context pack: %v", err)
	}

	previous := ""
	if raw, readErr := os.ReadFile(cursorFile); readErr == nil {
		previous = string(raw)
	}
	added, removed := diffPackLines(previous, snapshot.Pack)
	changed := strings.TrimSpace(previous) != strings.TrimSpace(snapshot.Pack)
	semanticCursorFile := semanticCursorPath(cursorFile)
	semanticLines := []string{}
	if semanticOnly {
		currentSemantic := extractSemanticLines(snapshot.Pack)
		baselineSemantic, loadErr := loadSemanticCursor(semanticCursorFile)
		if loadErr != nil {
			return commandErrorf(jsonOut, "semantic_cursor_read_failed", "failed reading semantic cursor: %v", loadErr)
		}
		added, removed = diffPackLines(strings.Join(baselineSemantic, "\n"), strings.Join(currentSemantic, "\n"))
		changed = strings.Join(baselineSemantic, "\n") != strings.Join(currentSemantic, "\n")
		semanticLines = currentSemantic
	}

	if err := os.MkdirAll(filepath.Dir(cursorFile), 0o700); err != nil {
		return commandErrorf(jsonOut, "cursor_file_write_failed", "failed creating cursor dir: %v", err)
	}
	if err := writeFileAtomic(cursorFile, []byte(snapshot.Pack)); err != nil {
		return commandErrorf(jsonOut, "cursor_file_write_failed", "failed writing --cursor-file: %v", err)
	}
	if semanticOnly {
		if err := saveSemanticCursor(semanticCursorFile, semanticLines); err != nil {
			return commandErrorf(jsonOut, "semantic_cursor_write_failed", "failed writing semantic cursor: %v", err)
		}
	}

	payload := map[string]any{
		"session":      session,
		"status":       snapshot.Status,
		"sessionState": snapshot.SessionState,
		"nextAction":   snapshot.NextAction,
		"nextOffset":   snapshot.NextOffset,
		"changed":      changed,
		"addedLines":   added,
		"removedLines": removed,
		"cursorFile":   cursorFile,
		"semanticOnly": semanticOnly,
		"strategy":     strategyConfig.Name,
		"tokenBudget":  tokenBudget,
		"truncated":    snapshot.Truncated,
	}
	if semanticOnly {
		payload["semanticCursorFile"] = semanticCursorFile
		payload["semanticLineCount"] = len(semanticLines)
	}
	if !jsonMin {
		payload["pack"] = snapshot.Pack
		payload["reason"] = snapshot.Reason
		payload["events"] = snapshot.Events
		payload["droppedRecent"] = snapshot.Dropped
		payload["projectRoot"] = projectRoot
		if semanticOnly {
			payload["semanticLines"] = semanticLines
		}
	}
	if snapshot.SessionState == "not_found" {
		payload["errorCode"] = "session_not_found"
	}
	if jsonOut {
		writeJSON(payload)
		if snapshot.SessionState == "not_found" {
			return 1
		}
		return 0
	}
	fmt.Printf("changed=%t added=%d removed=%d cursor=%s\n", changed, len(added), len(removed), cursorFile)
	if snapshot.SessionState == "not_found" {
		return 1
	}
	return 0
}

func buildSessionPackSnapshot(session, projectRoot, agentHint, modeHint string, strategyConfig contextPackStrategyConfig, events, lines, tokenBudget int, redactRules []string) (sessionPackSnapshot, error) {
	restore := withProjectRuntimeEnv(projectRoot)
	defer restore()
	status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, false, 0)
	if err != nil {
		return sessionPackSnapshot{}, err
	}
	status = normalizeStatusForSessionStatusOutput(status)
	if strings.TrimSpace(status.Session) == "" {
		status.Session = session
	}

	tail, _ := readSessionEventTailFn(projectRoot, session, events)
	recent := make([]string, 0, len(tail.Events))
	for _, event := range tail.Events {
		recent = append(recent, fmt.Sprintf("%s %s/%s %s", event.At, event.State, event.Status, event.Reason))
	}

	captureTail := "(no live capture)"
	restoreCapture := withProjectRuntimeEnv(projectRoot)
	active := tmuxHasSessionFn(session)
	restoreCapture()
	if active {
		restoreCapture = withProjectRuntimeEnv(projectRoot)
		capture, captureErr := tmuxCapturePaneFn(session, lines)
		restoreCapture()
		if captureErr == nil {
			captureTail = strings.Join(trimLines(filterCaptureNoise(capture)), "\n")
		}
	}

	raw := buildContextPackRaw(strategyConfig.Name, session, status, recent, captureTail)
	pack, truncated := truncateToTokenBudget(raw, tokenBudget)
	pack = applyRedactionRules(pack, redactRules)

	return sessionPackSnapshot{
		Session:      session,
		Status:       status.Status,
		SessionState: status.SessionState,
		Reason:       status.ClassificationReason,
		NextAction:   nextActionForState(status.SessionState),
		NextOffset:   computeSessionCaptureNextOffset(session),
		Pack:         pack,
		Truncated:    truncated,
		Events:       len(recent),
		Dropped:      tail.DroppedLines,
	}, nil
}

func diffPackLines(before, after string) ([]string, []string) {
	beforeLines := trimLines(before)
	afterLines := trimLines(after)
	beforeSet := map[string]struct{}{}
	afterSet := map[string]struct{}{}
	for _, line := range beforeLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		beforeSet[line] = struct{}{}
	}
	for _, line := range afterLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		afterSet[line] = struct{}{}
	}

	added := make([]string, 0)
	for line := range afterSet {
		if _, ok := beforeSet[line]; !ok {
			added = append(added, line)
		}
	}
	removed := make([]string, 0)
	for line := range beforeSet {
		if _, ok := afterSet[line]; !ok {
			removed = append(removed, line)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func parseCommaValues(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func loadSessionAggregateDeltaCursor(path string) (sessionAggregateDeltaCursor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return sessionAggregateDeltaCursor{Items: map[string]sessionAggregateDeltaItem{}}, nil
		}
		return sessionAggregateDeltaCursor{}, err
	}
	cursor := sessionAggregateDeltaCursor{}
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return sessionAggregateDeltaCursor{}, err
	}
	if cursor.Items == nil {
		cursor.Items = map[string]sessionAggregateDeltaItem{}
	}
	return cursor, nil
}

func saveSessionAggregateDeltaCursor(path string, cursor sessionAggregateDeltaCursor) error {
	if cursor.Items == nil {
		cursor.Items = map[string]sessionAggregateDeltaItem{}
	}
	data, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

func sessionAggregateDeltaItemsEqual(a, b sessionAggregateDeltaItem) bool {
	return strings.TrimSpace(a.Session) == strings.TrimSpace(b.Session) &&
		strings.TrimSpace(a.Status) == strings.TrimSpace(b.Status) &&
		strings.TrimSpace(a.SessionState) == strings.TrimSpace(b.SessionState) &&
		strings.TrimSpace(a.NextAction) == strings.TrimSpace(b.NextAction) &&
		a.NextOffset == b.NextOffset &&
		strings.TrimSpace(a.Pack) == strings.TrimSpace(b.Pack)
}

func estimatePromptTokens(prompt string) int {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return 0
	}
	return (len(trimmed) / 4) + 1
}

package app

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

type sessionAutopilotStep struct {
	OK       bool           `json:"ok"`
	ExitCode int            `json:"exitCode,omitempty"`
	Output   map[string]any `json:"output,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type sessionAutopilotSummary struct {
	OK          bool                 `json:"ok"`
	Goal        string               `json:"goal"`
	Agent       string               `json:"agent"`
	Mode        string               `json:"mode"`
	ProjectRoot string               `json:"projectRoot"`
	Session     string               `json:"session,omitempty"`
	KillAfter   bool                 `json:"killAfter"`
	Spawn       sessionAutopilotStep `json:"spawn"`
	Monitor     sessionAutopilotStep `json:"monitor"`
	Capture     sessionAutopilotStep `json:"capture"`
	Handoff     sessionAutopilotStep `json:"handoff"`
	Cleanup     sessionAutopilotStep `json:"cleanup,omitempty"`
	ErrorCode   string               `json:"errorCode,omitempty"`
	Error       string               `json:"error,omitempty"`
	FailedStep  string               `json:"failedStep,omitempty"`
	ResumedFrom string               `json:"resumedFrom,omitempty"`
	ResumeStep  string               `json:"resumeStep,omitempty"`
}

type handoffInputPayload struct {
	Session      string               `json:"session"`
	Status       string               `json:"status"`
	SessionState string               `json:"sessionState"`
	Reason       string               `json:"reason"`
	NextAction   string               `json:"nextAction"`
	NextOffset   int                  `json:"nextOffset"`
	Recent       []sessionHandoffItem `json:"recent"`
	CaptureTail  string               `json:"captureTail"`
}

type sessionHandoffRisk struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type sessionHandoffQuestion struct {
	Code     string `json:"code"`
	Question string `json:"question"`
}

type routeStateInput struct {
	Session      string `json:"session"`
	Status       string `json:"status"`
	SessionState string `json:"sessionState"`
	Reason       string `json:"reason"`
	NextAction   string `json:"nextAction"`
}

func cmdSessionHandoff(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	events := 8
	deltaFrom := -1
	cursorFile := ""
	compressMode := "none"
	schemaVersion := "v1"
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
		case "--cursor-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --cursor-file")
			}
			cursorFile = strings.TrimSpace(args[i+1])
			i++
		case "--compress":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --compress")
			}
			compressMode = strings.TrimSpace(args[i+1])
			i++
		case "--schema":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --schema")
			}
			schemaVersion = strings.ToLower(strings.TrimSpace(args[i+1]))
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
	switch schemaVersion {
	case "", "1", "v1":
		schemaVersion = "v1"
	case "2", "v2":
		schemaVersion = "v2"
	case "3", "v3":
		schemaVersion = "v3"
	default:
		return commandErrorf(jsonOut, "invalid_schema", "invalid --schema: %s (expected v1|v2|v3)", schemaVersion)
	}
	var err error
	compressMode, err = parseHandoffCompressMode(compressMode)
	if err != nil {
		return commandError(jsonOut, "invalid_compress_mode", err.Error())
	}
	if cursorFile != "" {
		cursorFileResolved, resolveErr := expandAndCleanPath(cursorFile)
		if resolveErr != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", resolveErr)
		}
		cursorFile = cursorFileResolved
		if deltaFrom < 0 {
			cursorOffset, cursorErr := loadCursorOffset(cursorFile)
			if cursorErr != nil {
				return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", cursorErr)
			}
			deltaFrom = cursorOffset
		}
	}

	resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, projectRootExplicit)
	if resolveErr != nil {
		return commandErrorf(jsonOut, "ambiguous_project_root", "%v", resolveErr)
	}
	projectRoot = resolvedRoot
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
	meta, _ := loadSessionMeta(projectRoot, session)
	laneName := strings.TrimSpace(meta.Lane)
	laneRecord := sessionLaneRecord{}
	laneFound := false
	if laneName != "" {
		if resolved, found, laneErr := loadLaneRecord(projectRoot, laneName); laneErr == nil && found {
			laneRecord = resolved
			laneFound = true
			if schemaVersion == "v1" && laneContractRequiresHandoffSchemaV2(laneRecord.Contract) {
				message := fmt.Sprintf("handoff_schema_v2_required: lane %q contract %q requires --schema v2", laneName, strings.TrimSpace(laneRecord.Contract))
				return commandError(jsonOut, "handoff_schema_v2_required", message)
			}
		}
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
		if deltaErr != nil {
			return commandErrorf(jsonOut, "handoff_delta_read_failed", "failed to read handoff delta: %v", deltaErr)
		}
		items = deltaItems
		droppedRecent = deltaDropped
		nextDeltaOffset = deltaNext
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
	objective := objectivePayloadFromMeta(meta)
	memoryPayload, hasMemory := loadSessionMemoryCompact(projectRoot, session, 8)
	risks := deriveHandoffRisks(status, items)
	openQuestions := deriveHandoffQuestions(status, items)
	lanePayload := map[string]any(nil)
	if laneName != "" {
		lanePayload = map[string]any{"name": laneName}
		if laneFound {
			if strings.TrimSpace(laneRecord.Contract) != "" {
				lanePayload["contract"] = laneRecord.Contract
			}
			if laneRecord.Budget > 0 {
				lanePayload["budget"] = laneRecord.Budget
			}
			if strings.TrimSpace(laneRecord.Goal) != "" {
				lanePayload["goal"] = laneRecord.Goal
			}
		}
	}
	if cursorFile != "" && nextDeltaOffset >= 0 {
		if writeErr := writeCursorOffset(cursorFile, nextDeltaOffset); writeErr != nil {
			return commandErrorf(jsonOut, "cursor_file_write_failed", "failed writing --cursor-file: %v", writeErr)
		}
	}

	if jsonOut {
		payload := map[string]any{
			"session":      session,
			"status":       status.Status,
			"sessionState": status.SessionState,
			"schema":       schemaVersion,
			"reason":       status.ClassificationReason,
			"nextAction":   nextAction,
			"nextOffset":   nextOffset,
			"summary":      summary,
		}
		if objective != nil {
			payload["objective"] = objective
		}
		if lanePayload != nil {
			payload["lane"] = lanePayload
		}
		if hasMemory {
			payload["memory"] = memoryPayload
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
		if cursorFile != "" && !jsonMin {
			payload["cursorFile"] = cursorFile
		}
		if status.SessionState == "not_found" {
			payload["errorCode"] = "session_not_found"
		}
		if schemaVersion == "v2" || schemaVersion == "v3" {
			payload["state"] = map[string]any{
				"status":       status.Status,
				"sessionState": status.SessionState,
				"reason":       status.ClassificationReason,
				"summary":      summary,
			}
			payload["nextAction"] = map[string]any{
				"name":    nextAction,
				"command": recommendedCommandForAction(nextAction, session, projectRoot),
			}
			payload["risks"] = risks
			payload["openQuestions"] = openQuestions
			if schemaVersion == "v3" {
				payload["state"] = map[string]any{
					"id":           deterministicHandoffID(session, status.SessionState, "state", 0),
					"status":       status.Status,
					"sessionState": status.SessionState,
					"reason":       status.ClassificationReason,
					"summary":      summary,
				}
				payload["nextAction"] = map[string]any{
					"id":      deterministicHandoffID(session, status.SessionState, "nextAction:"+nextAction, nextOffset),
					"name":    nextAction,
					"command": recommendedCommandForAction(nextAction, session, projectRoot),
				}
				payload["risks"] = handoffRisksV3(session, status.SessionState, risks)
				payload["openQuestions"] = handoffQuestionsV3(session, status.SessionState, openQuestions)
			}
		}
		if compressMode != "none" {
			compressInput := map[string]any{
				"session":      payload["session"],
				"status":       payload["status"],
				"sessionState": payload["sessionState"],
				"reason":       payload["reason"],
				"nextAction":   nextAction,
				"nextOffset":   payload["nextOffset"],
				"summary":      payload["summary"],
			}
			if recent, ok := payload["recent"]; ok {
				compressInput["recent"] = recent
			}
			if deltaFrom >= 0 {
				compressInput["deltaFrom"] = payload["deltaFrom"]
				compressInput["nextDeltaOffset"] = payload["nextDeltaOffset"]
				compressInput["deltaCount"] = payload["deltaCount"]
			}
			encoded, uncompressed, compressed, compressErr := compressJSONPayloadStdlib(compressInput)
			if compressErr != nil {
				return commandErrorf(jsonOut, "handoff_compress_failed", "failed to compress handoff payload: %v", compressErr)
			}
			payload["compression"] = compressMode
			payload["encoding"] = "base64-gzip"
			payload["compressedPayload"] = encoded
			payload["uncompressedBytes"] = uncompressed
			payload["compressedBytes"] = compressed
			// Prefer compressed transfer packet when explicitly requested.
			delete(payload, "recent")
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

func parseHandoffCompressMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none":
		return "none", nil
	case "zstd":
		// stdlib-only build: expose zstd contract but use gzip transport backend.
		return "zstd", nil
	default:
		return "", fmt.Errorf("invalid --compress: %s (expected none|zstd)", raw)
	}
}

func laneContractRequiresHandoffSchemaV2(contract string) bool {
	normalized := strings.ToLower(strings.TrimSpace(contract))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "handoff_v2_required") ||
		strings.Contains(normalized, "handoff_schema_v2_required")
}

func compressJSONPayloadStdlib(payload map[string]any) (string, int, int, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", 0, 0, err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return "", 0, 0, err
	}
	if err := zw.Close(); err != nil {
		return "", 0, 0, err
	}
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return encoded, len(raw), buf.Len(), nil
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
	fromHandoff := ""
	redactRaw := ""
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
		case "--from-handoff":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --from-handoff")
			}
			fromHandoff = strings.TrimSpace(args[i+1])
			i++
		case "--redact":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --redact")
			}
			redactRaw = strings.TrimSpace(args[i+1])
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
	if !eventsSet {
		events = strategyConfig.Events
	}
	if !linesSet {
		lines = strategyConfig.Lines
	}
	if !tokenBudgetSet {
		tokenBudget = strategyConfig.TokenBudget
	}
	var handoffInput *handoffInputPayload
	if fromHandoff != "" {
		handoffInput, err = loadHandoffInputPayload(fromHandoff)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_from_handoff", "failed to load --from-handoff: %v", err)
		}
		handoffSession := strings.TrimSpace(handoffInput.Session)
		if session == "" {
			session = handoffSession
		} else if handoffSession != "" && session != handoffSession {
			return commandErrorf(jsonOut, "from_handoff_session_mismatch", "session mismatch: --for=%s --from-handoff session=%s", session, handoffSession)
		}
	}
	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--for is required")
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

	status := sessionStatus{Session: session}
	recent := []string{}
	captureTail := ""
	droppedRecent := 0
	nextOffset := 0
	if handoffInput != nil {
		status.Status = normalizeMonitorFinalStatus(handoffInput.SessionState, handoffInput.Status)
		status.SessionState = strings.TrimSpace(handoffInput.SessionState)
		status.ClassificationReason = strings.TrimSpace(handoffInput.Reason)
		if status.Status == "" {
			status.Status = strings.TrimSpace(handoffInput.Status)
		}
		if status.Status == "" {
			status.Status = status.SessionState
		}
		if status.SessionState == "" {
			status.SessionState = "in_progress"
		}
		if status.ClassificationReason == "" {
			status.ClassificationReason = "from_handoff"
		}
		for _, item := range handoffInput.Recent {
			recent = append(recent, fmt.Sprintf("%s %s/%s %s", item.At, item.State, item.Status, item.Reason))
		}
		captureTail = strings.TrimSpace(handoffInput.CaptureTail)
		if captureTail == "" {
			captureTail = "(from handoff: capture unavailable)"
		}
		nextOffset = handoffInput.NextOffset
	} else {
		statusComputed, statusErr := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, false, 0)
		if statusErr != nil {
			return commandError(jsonOut, "status_compute_failed", statusErr.Error())
		}
		status = normalizeStatusForSessionStatusOutput(statusComputed)
		tail, _ := readSessionEventTailFn(projectRoot, session, events)
		droppedRecent = tail.DroppedLines
		recent = make([]string, 0, len(tail.Events))
		for _, event := range tail.Events {
			recent = append(recent, fmt.Sprintf("%s %s/%s %s", event.At, event.State, event.Status, event.Reason))
		}

		if tmuxHasSessionFn(session) {
			if capture, captureErr := tmuxCapturePaneFn(session, lines); captureErr == nil {
				captureTail = strings.Join(trimLines(filterCaptureNoise(capture)), "\n")
			}
		}
		if captureTail == "" {
			captureTail = "(no live capture)"
		}
		nextOffset = computeSessionCaptureNextOffset(session)
	}

	meta, _ := loadSessionMeta(projectRoot, session)
	objective := objectivePayloadFromMeta(meta)
	memoryPayload, hasMemory := loadSessionMemoryCompact(projectRoot, session, 8)
	packRaw := buildContextPackRaw(strategyConfig.Name, session, status, recent, captureTail)
	if objective != nil {
		packRaw = strings.TrimSpace(packRaw) + "\nobjective:\n" + objectiveSummaryLine(objective)
	}
	if hasMemory {
		if lines, ok := memoryPayload["lines"].([]string); ok && len(lines) > 0 {
			packRaw = strings.TrimSpace(packRaw) + "\nmemory:\n" + strings.Join(lines, "\n")
		}
	}
	pack, truncated := truncateToTokenBudget(packRaw, tokenBudget)
	pack = applyRedactionRules(pack, redactRules)

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
		if objective != nil {
			payload["objective"] = objective
		}
		if strings.TrimSpace(meta.Lane) != "" {
			payload["lane"] = meta.Lane
		}
		if hasMemory {
			payload["memory"] = memoryPayload
		}
		if !jsonMin {
			payload["projectRoot"] = projectRoot
			payload["events"] = len(recent)
			payload["droppedRecent"] = droppedRecent
			if fromHandoff != "" {
				payload["fromHandoff"] = fromHandoff
			}
			if len(redactRules) > 0 {
				payload["redactRules"] = redactRules
			}
		} else if len(redactRules) > 0 {
			payload["redactRules"] = redactRules
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
	lane := ""
	projectRoot := getPWD()
	prompt := ""
	model := ""
	profile := ""
	budget := 0
	emitRunbook := false
	queue := false
	queueSessionsRaw := ""
	queueLimit := 0
	concurrency := 1
	topologyRaw := ""
	laneModeOverride := ""
	laneNestedPolicyOverride := ""
	laneNestingIntentOverride := ""
	costEstimate := false
	fromState := ""
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
		case "--lane":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --lane")
			}
			lane = strings.ToLower(strings.TrimSpace(args[i+1]))
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
		case "--profile":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --profile")
			}
			profile = strings.TrimSpace(args[i+1])
			i++
		case "--budget":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --budget")
			}
			n, parseErr := parsePositiveIntFlag(args[i+1], "--budget")
			if parseErr != nil {
				return commandError(jsonOut, "invalid_budget", parseErr.Error())
			}
			budget = n
			i++
		case "--emit-runbook":
			emitRunbook = true
		case "--queue":
			queue = true
		case "--sessions":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --sessions")
			}
			queueSessionsRaw = strings.TrimSpace(args[i+1])
			i++
		case "--queue-limit":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --queue-limit")
			}
			n, parseErr := parsePositiveIntFlag(args[i+1], "--queue-limit")
			if parseErr != nil {
				return commandError(jsonOut, "invalid_queue_limit", parseErr.Error())
			}
			queueLimit = n
			i++
		case "--concurrency":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --concurrency")
			}
			n, parseErr := parsePositiveIntFlag(args[i+1], "--concurrency")
			if parseErr != nil {
				return commandError(jsonOut, "invalid_concurrency", parseErr.Error())
			}
			concurrency = n
			i++
		case "--topology":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --topology")
			}
			topologyRaw = strings.TrimSpace(args[i+1])
			i++
		case "--cost-estimate":
			costEstimate = true
		case "--from-state":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --from-state")
			}
			fromState = strings.TrimSpace(args[i+1])
			i++
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
	if lane != "" {
		laneRecord, found, laneErr := loadLaneRecord(projectRoot, lane)
		if laneErr != nil {
			return commandErrorf(jsonOut, "lane_load_failed", "failed loading lane %q: %v", lane, laneErr)
		}
		if !found {
			return commandErrorf(jsonOut, "lane_not_found", "lane not found: %s", lane)
		}
		if strings.TrimSpace(laneRecord.Goal) != "" {
			goal = laneRecord.Goal
		}
		if strings.TrimSpace(laneRecord.Agent) != "" {
			agent = laneRecord.Agent
		}
		if strings.TrimSpace(laneRecord.Prompt) != "" && strings.TrimSpace(prompt) == "" {
			prompt = laneRecord.Prompt
		}
		if strings.TrimSpace(laneRecord.Model) != "" && strings.TrimSpace(model) == "" {
			model = laneRecord.Model
		}
		if laneRecord.Budget > 0 && budget <= 0 {
			budget = laneRecord.Budget
		}
		if strings.TrimSpace(laneRecord.Topology) != "" && strings.TrimSpace(topologyRaw) == "" {
			topologyRaw = laneRecord.Topology
		}
		if strings.TrimSpace(laneRecord.Mode) != "" {
			laneModeOverride = laneRecord.Mode
		}
		if strings.TrimSpace(laneRecord.NestedPolicy) != "" {
			laneNestedPolicyOverride = laneRecord.NestedPolicy
		}
		if strings.TrimSpace(laneRecord.NestingIntent) != "" {
			laneNestingIntentOverride = laneRecord.NestingIntent
		}
	}
	if strings.TrimSpace(profile) != "" {
		presetAgent, presetModel, presetErr := parseRouteProfile(profile)
		if presetErr != nil {
			return commandError(jsonOut, "invalid_profile", presetErr.Error())
		}
		agent = presetAgent
		if strings.TrimSpace(model) == "" {
			model = presetModel
		}
	}
	goal, err = parseSessionRouteGoal(goal)
	if err != nil {
		return commandError(jsonOut, "invalid_goal", err.Error())
	}
	agent, err = parseAgent(agent)
	if err != nil {
		return commandError(jsonOut, "invalid_agent", err.Error())
	}

	mode, nestedPolicy, nestingIntent, defaultPrompt, defaultModel := sessionRouteDefaults(goal)
	if laneModeOverride != "" {
		parsedMode, modeErr := parseMode(laneModeOverride)
		if modeErr != nil {
			return commandErrorf(jsonOut, "invalid_lane_mode", "invalid lane mode: %v", modeErr)
		}
		mode = parsedMode
	}
	if laneNestedPolicyOverride != "" {
		parsedNestedPolicy, nestedErr := parseNestedPolicy(laneNestedPolicyOverride)
		if nestedErr != nil {
			return commandErrorf(jsonOut, "invalid_lane_nested_policy", "invalid lane nested policy: %v", nestedErr)
		}
		nestedPolicy = parsedNestedPolicy
	}
	if laneNestingIntentOverride != "" {
		parsedNestingIntent, nestingErr := parseNestingIntent(laneNestingIntentOverride)
		if nestingErr != nil {
			return commandErrorf(jsonOut, "invalid_lane_nesting_intent", "invalid lane nesting intent: %v", nestingErr)
		}
		nestingIntent = parsedNestingIntent
	}
	topologyRoles, err := parseTopologyRoles(topologyRaw)
	if err != nil {
		return commandError(jsonOut, "invalid_topology", err.Error())
	}
	var parsedFromState *routeStateInput
	if fromState != "" {
		parsedFromState, err = loadRouteStateInput(fromState)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_from_state", "failed to load --from-state: %v", err)
		}
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = defaultPrompt
		if parsedFromState != nil {
			prompt = routePromptFromState(*parsedFromState, defaultPrompt)
		}
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
	if budget > 0 {
		rationale = append(rationale, fmt.Sprintf("budget=%d", budget))
	}
	if len(topologyRoles) > 0 {
		rationale = append(rationale, "topology="+strings.Join(topologyRoles, ","))
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
		"profile":         profile,
		"command":         command,
		"monitorHint":     monitorHint,
		"nestedDetection": detection,
		"rationale":       rationale,
	}
	if lane != "" {
		payload["lane"] = lane
	}
	if budget > 0 {
		payload["budget"] = budget
	}
	payload["concurrency"] = concurrency
	if parsedFromState != nil {
		payload["fromState"] = parsedFromState
	}
	if emitRunbook {
		payload["runbook"] = buildRouteRunbook(projectRoot, agent, mode, nestedPolicy, nestingIntent, prompt, model, budget)
	}
	routeQueue := make([]map[string]any, 0)
	if queue {
		queueItems, queueErr := buildRouteQueue(projectRoot, queueSessionsRaw, queueLimit, budget, concurrency)
		if queueErr != nil {
			return commandErrorf(jsonOut, "queue_build_failed", "failed to build queue: %v", queueErr)
		}
		routeQueue = queueItems
		payload["queue"] = queueItems
		payload["queueCount"] = len(queueItems)
		payload["dispatchPlan"] = buildRouteDispatchPlan(queueItems, concurrency)
	}
	if len(topologyRoles) > 0 {
		payload["topology"] = buildTopologyGraph(topologyRoles)
	}
	if costEstimate {
		payload["costEstimate"] = estimateRouteCost(goal, mode, budget, topologyRoles)
	}
	if jsonOut {
		writeJSON(payload)
		return 0
	}
	for _, item := range routeQueue {
		if mapStringValue(item, "sessionState") != "ambiguous_project_root" {
			continue
		}
		fmt.Fprintf(os.Stderr, "warning: session=%s %s\n", mapStringValue(item, "session"), mapStringValue(item, "reason"))
	}
	fmt.Printf("%s\n%s\n", command, strings.Join(rationale, " | "))
	return 0
}

func parseRouteProfile(raw string) (agent string, model string, err error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "codex-spark":
		return "codex", "gpt-5.3-codex-spark", nil
	case "claude":
		return "claude", "", nil
	default:
		return "", "", fmt.Errorf("invalid --profile: %s (expected codex-spark|claude)", raw)
	}
}

func cmdSessionGuard(args []string) int {
	sharedTmux := false
	enforce := false
	adviceOnly := false
	machinePolicy := "strict"
	commandText := ""
	policyFile := ""
	projectRoot := canonicalProjectRoot(getPWD())
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session guard")
		case "--shared-tmux":
			sharedTmux = true
		case "--enforce":
			enforce = true
		case "--advice-only":
			adviceOnly = true
		case "--machine-policy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --machine-policy")
			}
			machinePolicy = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--command":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --command")
			}
			commandText = args[i+1]
			i++
		case "--policy-file":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --policy-file")
			}
			policyFile = strings.TrimSpace(args[i+1])
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
	switch machinePolicy {
	case "strict", "warn", "off":
	default:
		return commandErrorf(jsonOut, "invalid_machine_policy", "invalid --machine-policy: %s (expected strict|warn|off)", machinePolicy)
	}
	var policy *sessionGuardPolicy
	if policyFile != "" {
		resolvedPolicy, resolveErr := expandAndCleanPath(policyFile)
		if resolveErr != nil {
			return commandErrorf(jsonOut, "invalid_policy_file", "invalid --policy-file: %v", resolveErr)
		}
		policyFile = resolvedPolicy
		loaded, loadErr := loadSessionGuardPolicy(policyFile)
		if loadErr != nil {
			return commandErrorf(jsonOut, "policy_file_read_failed", "failed reading --policy-file: %v", loadErr)
		}
		policy = loaded
		if strings.TrimSpace(policy.MachinePolicy) != "" {
			machinePolicy = strings.ToLower(strings.TrimSpace(policy.MachinePolicy))
			switch machinePolicy {
			case "strict", "warn", "off":
			default:
				return commandErrorf(jsonOut, "invalid_policy_machine_policy", "invalid policy machinePolicy: %s (expected strict|warn|off)", policy.MachinePolicy)
			}
		}
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
	riskReasons := []string{}
	lowerCommand := strings.ToLower(strings.TrimSpace(commandText))
	if lowerCommand != "" {
		if strings.Contains(lowerCommand, "cleanup") && strings.Contains(lowerCommand, "--include-tmux-default") {
			commandRisk = "high"
			warnings = append(warnings, "command targets tmux default server")
			riskReasons = append(riskReasons, "cleanup_include_tmux_default")
		}
		if strings.Contains(lowerCommand, "session kill-all") && !strings.Contains(lowerCommand, "--project-only") {
			commandRisk = "high"
			warnings = append(warnings, "kill-all without --project-only can impact unrelated sessions")
			riskReasons = append(riskReasons, "kill_all_without_project_only")
		}
		if strings.Contains(lowerCommand, "cleanup") && !strings.Contains(lowerCommand, "--dry-run") {
			if commandRisk == "low" {
				commandRisk = "medium"
			}
			warnings = append(warnings, "cleanup without --dry-run mutates runtime artifacts")
			riskReasons = append(riskReasons, "cleanup_without_dry_run")
		}
		if strings.Contains(lowerCommand, "session kill") && !strings.Contains(lowerCommand, "--project-root") {
			if commandRisk == "low" {
				commandRisk = "medium"
			}
			warnings = append(warnings, "session kill without --project-root may target wrong project hash")
			riskReasons = append(riskReasons, "kill_without_project_root")
		}
		if policy != nil {
			policyWarnings, policyReasons, policyRisk := evaluateGuardPolicy(*policy, lowerCommand)
			if len(policyWarnings) > 0 {
				warnings = append(warnings, policyWarnings...)
			}
			if len(policyReasons) > 0 {
				riskReasons = append(riskReasons, policyReasons...)
			}
			commandRisk = mergeGuardRisk(commandRisk, policyRisk)
		}
	}

	safe := len(defaultSessions) == 0 && commandRisk != "high"
	if machinePolicy == "strict" || enforce {
		safe = len(defaultSessions) == 0 && commandRisk == "low"
	}
	remediation := []string{}
	if !safe {
		remediation = append(remediation, "use --project-only and --project-root for destructive session commands")
		if strings.Contains(lowerCommand, "cleanup") {
			remediation = append(remediation, "run cleanup with --dry-run before executing mutations")
		}
		if len(defaultSessions) > 0 {
			remediation = append(remediation, "avoid touching default tmux server while shared sessions are active")
		}
	}
	if jsonOut {
		payload := map[string]any{
			"sharedTmux":          true,
			"projectRoot":         projectRoot,
			"defaultSessionCount": len(defaultSessions),
			"defaultSessions":     defaultSessions,
			"command":             commandText,
			"commandRisk":         commandRisk,
			"enforce":             enforce,
			"adviceOnly":          adviceOnly,
			"machinePolicy":       machinePolicy,
			"safe":                safe,
			"warnings":            warnings,
		}
		if policyFile != "" {
			payload["policyFile"] = policyFile
		}
		if len(riskReasons) > 0 {
			payload["riskReasons"] = riskReasons
		}
		if len(remediation) > 0 {
			payload["remediation"] = remediation
		}
		if !safe && !adviceOnly && machinePolicy == "strict" {
			if enforce {
				payload["errorCode"] = "shared_tmux_guard_enforced"
			} else {
				payload["errorCode"] = "shared_tmux_risk_detected"
			}
		}
		writeJSON(payload)
		if adviceOnly {
			return 0
		}
		if machinePolicy == "warn" || machinePolicy == "off" {
			return 0
		}
		return boolExit(safe)
	}

	fmt.Printf("safe=%t enforce=%t advice_only=%t machine_policy=%s default_sessions=%d command_risk=%s\n", safe, enforce, adviceOnly, machinePolicy, len(defaultSessions), commandRisk)
	for _, warning := range warnings {
		fmt.Printf("- %s\n", warning)
	}
	for _, line := range remediation {
		fmt.Printf("- remediation: %s\n", line)
	}
	if adviceOnly {
		return 0
	}
	if machinePolicy == "warn" || machinePolicy == "off" {
		return 0
	}
	return boolExit(safe)
}

type sessionGuardPolicy struct {
	MachinePolicy                  string   `json:"machinePolicy"`
	AllowedCommands                []string `json:"allowedCommands"`
	DeniedCommands                 []string `json:"deniedCommands"`
	RequireProjectRoot             bool     `json:"requireProjectRoot"`
	RequireProjectOnlyForKillAll   bool     `json:"requireProjectOnlyForKillAll"`
	AllowCleanupIncludeTmuxDefault bool     `json:"allowCleanupIncludeTmuxDefault"`
}

func loadSessionGuardPolicy(path string) (*sessionGuardPolicy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	policy := sessionGuardPolicy{}
	if err := json.Unmarshal(raw, &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

func evaluateGuardPolicy(policy sessionGuardPolicy, commandText string) (warnings []string, reasons []string, risk string) {
	risk = "low"
	command := strings.ToLower(strings.TrimSpace(commandText))
	if command == "" {
		return warnings, reasons, risk
	}

	if len(policy.AllowedCommands) > 0 {
		matched := false
		for _, entry := range policy.AllowedCommands {
			entry = strings.ToLower(strings.TrimSpace(entry))
			if entry == "" {
				continue
			}
			if strings.Contains(command, entry) {
				matched = true
				break
			}
		}
		if !matched {
			warnings = append(warnings, "policy denies command outside allowedCommands")
			reasons = append(reasons, "policy_not_allowed")
			risk = "high"
		}
	}

	for _, denied := range policy.DeniedCommands {
		denied = strings.ToLower(strings.TrimSpace(denied))
		if denied == "" {
			continue
		}
		if strings.Contains(command, denied) {
			warnings = append(warnings, "policy denies command token: "+denied)
			reasons = append(reasons, "policy_denied_command")
			risk = "high"
			break
		}
	}

	if policy.RequireProjectRoot && strings.Contains(command, "session ") && !strings.Contains(command, "--project-root") {
		warnings = append(warnings, "policy requires --project-root on session commands")
		reasons = append(reasons, "policy_requires_project_root")
		risk = mergeGuardRisk(risk, "medium")
	}
	if policy.RequireProjectOnlyForKillAll && strings.Contains(command, "session kill-all") && !strings.Contains(command, "--project-only") {
		warnings = append(warnings, "policy requires --project-only for kill-all")
		reasons = append(reasons, "policy_requires_project_only_for_kill_all")
		risk = mergeGuardRisk(risk, "high")
	}
	if !policy.AllowCleanupIncludeTmuxDefault && strings.Contains(command, "cleanup") && strings.Contains(command, "--include-tmux-default") {
		warnings = append(warnings, "policy disallows cleanup --include-tmux-default")
		reasons = append(reasons, "policy_disallow_cleanup_include_tmux_default")
		risk = mergeGuardRisk(risk, "high")
	}
	return warnings, reasons, risk
}

func mergeGuardRisk(base, candidate string) string {
	rank := func(value string) int {
		switch value {
		case "high":
			return 3
		case "medium":
			return 2
		default:
			return 1
		}
	}
	if rank(candidate) > rank(base) {
		return candidate
	}
	return base
}

func cmdSessionAutopilot(args []string) int {
	goal := "analysis"
	agent := "codex"
	lane := ""
	modeOverride := ""
	nestedPolicyOverride := ""
	nestingIntentOverride := ""
	projectRoot := getPWD()
	session := ""
	prompt := ""
	model := ""
	pollInterval := defaultPollIntervalSeconds
	maxPolls := defaultMaxPolls
	captureLines := 220
	summary := false
	summaryStyle := "ops"
	tokenBudget := 320
	killAfter := false
	killAfterExplicit := false
	resumeFrom := ""
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session autopilot")
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
		case "--lane":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --lane")
			}
			lane = strings.ToLower(strings.TrimSpace(args[i+1]))
			i++
		case "--mode":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --mode")
			}
			modeOverride = args[i+1]
			i++
		case "--nested-policy":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nested-policy")
			}
			nestedPolicyOverride = args[i+1]
			i++
		case "--nesting-intent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --nesting-intent")
			}
			nestingIntentOverride = args[i+1]
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = strings.TrimSpace(args[i+1])
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
		case "--poll-interval":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --poll-interval")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--poll-interval")
			if err != nil {
				return commandError(jsonOut, "invalid_poll_interval", err.Error())
			}
			pollInterval = n
			i++
		case "--max-polls":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --max-polls")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--max-polls")
			if err != nil {
				return commandError(jsonOut, "invalid_max_polls", err.Error())
			}
			maxPolls = n
			i++
		case "--capture-lines":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --capture-lines")
			}
			n, err := parsePositiveIntFlag(args[i+1], "--capture-lines")
			if err != nil {
				return commandError(jsonOut, "invalid_capture_lines", err.Error())
			}
			captureLines = n
			i++
		case "--summary":
			summary = true
		case "--summary-style":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --summary-style")
			}
			summaryStyle = args[i+1]
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
		case "--kill-after":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --kill-after")
			}
			parsed, err := parseBoolFlag(args[i+1])
			if err != nil {
				return commandErrorf(jsonOut, "invalid_kill_after", "invalid --kill-after: %s (expected true|false)", args[i+1])
			}
			killAfter = parsed
			killAfterExplicit = true
			i++
		case "--resume-from":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --resume-from")
			}
			resumeFrom = strings.TrimSpace(args[i+1])
			i++
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
	if lane != "" {
		laneRecord, found, laneErr := loadLaneRecord(projectRoot, lane)
		if laneErr != nil {
			return commandErrorf(jsonOut, "lane_load_failed", "failed loading lane %q: %v", lane, laneErr)
		}
		if !found {
			return commandErrorf(jsonOut, "lane_not_found", "lane not found: %s", lane)
		}
		if strings.TrimSpace(laneRecord.Goal) != "" {
			goal = laneRecord.Goal
		}
		if strings.TrimSpace(laneRecord.Agent) != "" {
			agent = laneRecord.Agent
		}
		if modeOverride == "" && strings.TrimSpace(laneRecord.Mode) != "" {
			modeOverride = laneRecord.Mode
		}
		if nestedPolicyOverride == "" && strings.TrimSpace(laneRecord.NestedPolicy) != "" {
			nestedPolicyOverride = laneRecord.NestedPolicy
		}
		if nestingIntentOverride == "" && strings.TrimSpace(laneRecord.NestingIntent) != "" {
			nestingIntentOverride = laneRecord.NestingIntent
		}
		if strings.TrimSpace(prompt) == "" && strings.TrimSpace(laneRecord.Prompt) != "" {
			prompt = laneRecord.Prompt
		}
		if strings.TrimSpace(model) == "" && strings.TrimSpace(laneRecord.Model) != "" {
			model = laneRecord.Model
		}
		if tokenBudget == 320 && laneRecord.Budget > 0 {
			tokenBudget = laneRecord.Budget
		}
	}
	goal, err = parseSessionRouteGoal(goal)
	if err != nil {
		return commandError(jsonOut, "invalid_goal", err.Error())
	}
	agent, err = parseAgent(agent)
	if err != nil {
		return commandError(jsonOut, "invalid_agent", err.Error())
	}
	if session != "" && !strings.HasPrefix(session, "lisa-") {
		return commandError(jsonOut, "invalid_session_name", `invalid --session: must start with "lisa-"`)
	}
	mode, nestedPolicy, nestingIntent, defaultPrompt, defaultModel := sessionRouteDefaults(goal)
	if strings.TrimSpace(modeOverride) != "" {
		mode, err = parseMode(modeOverride)
		if err != nil {
			return commandError(jsonOut, "invalid_mode", err.Error())
		}
	}
	if strings.TrimSpace(nestedPolicyOverride) != "" {
		nestedPolicy, err = parseNestedPolicy(nestedPolicyOverride)
		if err != nil {
			return commandError(jsonOut, "invalid_nested_policy", err.Error())
		}
	}
	if strings.TrimSpace(nestingIntentOverride) != "" {
		nestingIntent, err = parseNestingIntent(nestingIntentOverride)
		if err != nil {
			return commandError(jsonOut, "invalid_nesting_intent", err.Error())
		}
	}
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
	summaryStyle, err = parseCaptureSummaryStyle(summaryStyle)
	if err != nil {
		return commandError(jsonOut, "invalid_summary_style", err.Error())
	}
	if !summary && summaryStyle != "ops" {
		return commandError(jsonOut, "summary_style_requires_summary", "--summary-style requires --summary")
	}
	var resumePayload *sessionAutopilotSummary
	resumeStep := ""
	if resumeFrom != "" {
		resumePayload, err = loadAutopilotSummaryInput(resumeFrom)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_resume_from", "failed to load --resume-from: %v", err)
		}
		if strings.TrimSpace(modeOverride) == "" && strings.TrimSpace(resumePayload.Mode) != "" {
			mode = strings.TrimSpace(resumePayload.Mode)
		}
		if strings.TrimSpace(goal) == "analysis" && strings.TrimSpace(resumePayload.Goal) != "" {
			goal = strings.TrimSpace(resumePayload.Goal)
		}
		if session == "" {
			session = strings.TrimSpace(resumePayload.Session)
		}
		if session == "" && !resumePayload.Spawn.OK {
			// spawn failed previously; resume can still proceed by spawning a fresh session.
			session = ""
		}
		if !killAfterExplicit {
			killAfter = resumePayload.KillAfter
		}
		resumeStep = autopilotFirstFailedStep(*resumePayload)
	}

	binPath, err := osExecutableFn()
	if err != nil {
		return commandErrorf(jsonOut, "binary_path_resolve_failed", "failed to resolve lisa binary path: %v", err)
	}
	binPath = strings.TrimSpace(binPath)
	if binPath == "" {
		return commandError(jsonOut, "binary_path_empty", "failed to resolve lisa binary path")
	}

	summaryPayload := sessionAutopilotSummary{
		OK:          false,
		Goal:        goal,
		Agent:       agent,
		Mode:        mode,
		ProjectRoot: projectRoot,
		KillAfter:   killAfter,
	}
	if resumePayload != nil {
		summaryPayload.ResumedFrom = resumeFrom
		summaryPayload.ResumeStep = resumeStep
	}

	runStep := func(stepName string, stepArgs []string) (map[string]any, int, string) {
		stdout, stderrText, runErr := runLisaSubcommandFn(binPath, stepArgs...)
		if runErr != nil {
			exitCode := commandExitCode(runErr)
			msg := strings.TrimSpace(stderrText)
			if msg == "" {
				msg = runErr.Error()
			}
			return nil, exitCode, msg
		}
		payload := map[string]any{}
		if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); decodeErr != nil {
			return nil, 1, fmt.Sprintf("%s output parse failed: %v", stepName, decodeErr)
		}
		return payload, 0, ""
	}

	spawnedSession := ""
	startStep := "spawn"
	if resumePayload != nil {
		startStep = resumeStep
		summaryPayload.Spawn = resumePayload.Spawn
		summaryPayload.Monitor = resumePayload.Monitor
		summaryPayload.Capture = resumePayload.Capture
		summaryPayload.Handoff = resumePayload.Handoff
		summaryPayload.Cleanup = resumePayload.Cleanup
		if strings.TrimSpace(resumePayload.Session) != "" {
			spawnedSession = strings.TrimSpace(resumePayload.Session)
			summaryPayload.Session = spawnedSession
		}
		if startStep == "" {
			summaryPayload.OK = true
			if jsonOut {
				writeJSON(summaryPayload)
			} else {
				fmt.Printf("session=%s ok=true (resume no-op)\n", summaryPayload.Session)
			}
			return 0
		}
	}
	if startStep == "spawn" {
		spawnArgs := []string{
			"session", "spawn",
			"--agent", agent,
			"--mode", mode,
			"--nested-policy", nestedPolicy,
			"--nesting-intent", nestingIntent,
			"--project-root", projectRoot,
			"--prompt", prompt,
			"--json",
		}
		if session != "" {
			spawnArgs = append(spawnArgs, "--session", session)
		}
		if model != "" && agent == "codex" {
			spawnArgs = append(spawnArgs, "--model", model)
		}
		spawnOutput, spawnCode, spawnErr := runStep("spawn", spawnArgs)
		summaryPayload.Spawn = sessionAutopilotStep{
			OK:       spawnCode == 0,
			ExitCode: spawnCode,
			Output:   spawnOutput,
			Error:    spawnErr,
		}
		if spawnCode != 0 || spawnOutput == nil {
			summaryPayload.ErrorCode = "autopilot_spawn_failed"
			summaryPayload.Error = spawnErr
			summaryPayload.FailedStep = "spawn"
			if jsonOut {
				writeJSON(summaryPayload)
			} else {
				fmt.Fprintf(os.Stderr, "autopilot spawn failed: %s\n", spawnErr)
			}
			if spawnCode == 0 {
				return 1
			}
			return spawnCode
		}
		spawnedSession = strings.TrimSpace(fmt.Sprintf("%v", spawnOutput["session"]))
		if spawnedSession == "" {
			summaryPayload.ErrorCode = "autopilot_spawn_parse_failed"
			summaryPayload.Error = "spawn payload missing session field"
			summaryPayload.FailedStep = "spawn"
			if jsonOut {
				writeJSON(summaryPayload)
			} else {
				fmt.Fprintln(os.Stderr, "autopilot spawn failed: missing session in spawn output")
			}
			return 1
		}
		summaryPayload.Session = spawnedSession
	}
	if strings.TrimSpace(spawnedSession) == "" {
		summaryPayload.ErrorCode = "autopilot_resume_missing_session"
		summaryPayload.Error = "resume payload missing session"
		summaryPayload.FailedStep = startStep
		if jsonOut {
			writeJSON(summaryPayload)
		} else {
			fmt.Fprintln(os.Stderr, "autopilot resume failed: missing session")
		}
		return 1
	}

	stepOrder := []string{"spawn", "monitor", "capture", "handoff", "cleanup"}
	stepIndex := map[string]int{}
	for idx, step := range stepOrder {
		stepIndex[step] = idx
	}
	shouldRunStep := func(step string) bool {
		if startStep == "" {
			return true
		}
		return stepIndex[step] >= stepIndex[startStep]
	}

	monitorCode := summaryPayload.Monitor.ExitCode
	monitorErr := summaryPayload.Monitor.Error
	if shouldRunStep("monitor") {
		monitorArgs := []string{
			"session", "monitor",
			"--session", spawnedSession,
			"--project-root", projectRoot,
			"--poll-interval", strconv.Itoa(pollInterval),
			"--max-polls", strconv.Itoa(maxPolls),
			"--json",
		}
		if mode == "interactive" {
			monitorArgs = append(monitorArgs, "--stop-on-waiting", "true")
		} else {
			monitorArgs = append(monitorArgs, "--expect", "terminal")
		}
		monitorOutput, code, errText := runStep("monitor", monitorArgs)
		monitorCode = code
		monitorErr = errText
		summaryPayload.Monitor = sessionAutopilotStep{
			OK:       code == 0,
			ExitCode: code,
			Output:   monitorOutput,
			Error:    errText,
		}
	}

	captureCode := summaryPayload.Capture.ExitCode
	captureErr := summaryPayload.Capture.Error
	if shouldRunStep("capture") {
		captureArgs := []string{
			"session", "capture",
			"--session", spawnedSession,
			"--project-root", projectRoot,
			"--raw",
			"--lines", strconv.Itoa(captureLines),
			"--json",
		}
		if summary {
			captureArgs = append(captureArgs, "--summary", "--summary-style", summaryStyle, "--token-budget", strconv.Itoa(tokenBudget))
		}
		captureOutput, code, errText := runStep("capture", captureArgs)
		captureCode = code
		captureErr = errText
		summaryPayload.Capture = sessionAutopilotStep{
			OK:       code == 0,
			ExitCode: code,
			Output:   captureOutput,
			Error:    errText,
		}
	}

	handoffCode := summaryPayload.Handoff.ExitCode
	handoffErr := summaryPayload.Handoff.Error
	if shouldRunStep("handoff") {
		handoffArgs := []string{
			"session", "handoff",
			"--session", spawnedSession,
			"--project-root", projectRoot,
			"--json",
		}
		handoffOutput, code, errText := runStep("handoff", handoffArgs)
		handoffCode = code
		handoffErr = errText
		summaryPayload.Handoff = sessionAutopilotStep{
			OK:       code == 0,
			ExitCode: code,
			Output:   handoffOutput,
			Error:    errText,
		}
	}

	finalCode := 0
	if monitorCode != 0 {
		summaryPayload.ErrorCode = "autopilot_monitor_failed"
		summaryPayload.Error = monitorErr
		summaryPayload.FailedStep = "monitor"
		finalCode = monitorCode
	} else if captureCode != 0 {
		summaryPayload.ErrorCode = "autopilot_capture_failed"
		summaryPayload.Error = captureErr
		summaryPayload.FailedStep = "capture"
		finalCode = captureCode
	} else if handoffCode != 0 {
		summaryPayload.ErrorCode = "autopilot_handoff_failed"
		summaryPayload.Error = handoffErr
		summaryPayload.FailedStep = "handoff"
		finalCode = handoffCode
	}

	if killAfter && shouldRunStep("cleanup") {
		cleanupOutput, cleanupCode, cleanupErr := runStep("cleanup", []string{
			"session", "kill",
			"--session", spawnedSession,
			"--project-root", projectRoot,
			"--json",
		})
		summaryPayload.Cleanup = sessionAutopilotStep{
			OK:       cleanupCode == 0,
			ExitCode: cleanupCode,
			Output:   cleanupOutput,
			Error:    cleanupErr,
		}
		if finalCode == 0 && cleanupCode != 0 {
			summaryPayload.ErrorCode = "autopilot_cleanup_failed"
			summaryPayload.Error = cleanupErr
			summaryPayload.FailedStep = "cleanup"
			finalCode = cleanupCode
		}
	}

	summaryPayload.OK = finalCode == 0
	if jsonOut {
		writeJSON(summaryPayload)
	} else {
		fmt.Printf("session=%s ok=%t\n", summaryPayload.Session, summaryPayload.OK)
		if summaryPayload.Error != "" {
			fmt.Fprintf(os.Stderr, "autopilot failed at %s: %s\n", summaryPayload.FailedStep, summaryPayload.Error)
		}
	}
	return finalCode
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
	lockTimeout := getIntEnv("LISA_EVENT_LOCK_TIMEOUT_MS", defaultEventLockTimeoutMS)
	lockPath := eventsPath + ".lock"

	all := make([]sessionHandoffItem, 0)
	err := withSharedFileLock(lockPath, lockTimeout, func() error {
		raw, readErr := os.ReadFile(eventsPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return nil
			}
			return readErr
		}
		lines := trimLines(string(raw))
		all = make([]sessionHandoffItem, 0, len(lines))
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
		return nil
	})
	if err != nil {
		return nil, 0, 0, err
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

func buildRouteRunbook(projectRoot, agent, mode, nestedPolicy, nestingIntent, prompt, model string, budget int) map[string]any {
	spawn := fmt.Sprintf("./lisa session spawn --agent %s --mode %s --nested-policy %s --nesting-intent %s --project-root %s --prompt %s --json",
		agent, mode, nestedPolicy, nestingIntent, shellQuote(projectRoot), shellQuote(prompt))
	if model != "" && agent == "codex" {
		spawn = strings.TrimSuffix(spawn, " --json") + " --model " + shellQuote(model) + " --json"
	}
	monitor := "./lisa session monitor --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --expect terminal --json"
	if mode == "interactive" {
		monitor = "./lisa session monitor --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --stop-on-waiting true --json"
	}
	capture := "./lisa session capture --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --raw --json-min"
	contextPack := ""
	if budget > 0 {
		capture = "./lisa session capture --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --raw --summary --summary-style ops --token-budget " + strconv.Itoa(budget) + " --json"
		contextPack = "./lisa session context-pack --for \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --strategy balanced --token-budget " + strconv.Itoa(budget) + " --json-min"
	}
	steps := []map[string]any{
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
			"command": capture,
		},
		{
			"id":      "handoff",
			"command": "./lisa session handoff --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --json-min",
		},
	}
	if contextPack != "" {
		steps = append(steps, map[string]any{
			"id":      "context-pack",
			"command": contextPack,
		})
	}
	steps = append(steps, map[string]any{
		"id":      "cleanup",
		"command": "./lisa session kill --session \"$SESSION\" --project-root " + shellQuote(projectRoot) + " --json",
	})
	return map[string]any{
		"steps": steps,
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

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	type exitCoder interface {
		ExitCode() int
	}
	if ec, ok := err.(exitCoder); ok {
		code := ec.ExitCode()
		if code >= 0 {
			return code
		}
	}
	return 1
}

func autopilotFirstFailedStep(summary sessionAutopilotSummary) string {
	step := strings.TrimSpace(summary.FailedStep)
	switch step {
	case "spawn", "monitor", "capture", "handoff", "cleanup":
		return step
	}
	if !summary.Spawn.OK {
		return "spawn"
	}
	if !summary.Monitor.OK {
		return "monitor"
	}
	if !summary.Capture.OK {
		return "capture"
	}
	if !summary.Handoff.OK {
		return "handoff"
	}
	if summary.KillAfter && !summary.Cleanup.OK {
		return "cleanup"
	}
	if summary.OK {
		return ""
	}
	return "monitor"
}

func loadAutopilotSummaryInput(from string) (*sessionAutopilotSummary, error) {
	source := strings.TrimSpace(from)
	if source == "" {
		return nil, fmt.Errorf("empty resume source")
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
	summary := sessionAutopilotSummary{}
	if err := json.Unmarshal(raw, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func loadHandoffInputPayload(from string) (*handoffInputPayload, error) {
	source := strings.TrimSpace(from)
	if source == "" {
		return nil, fmt.Errorf("empty handoff source")
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
	payload := handoffInputPayload{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	payload.Session = strings.TrimSpace(payload.Session)
	payload.Status = strings.TrimSpace(payload.Status)
	payload.SessionState = strings.TrimSpace(payload.SessionState)
	payload.Reason = strings.TrimSpace(payload.Reason)
	payload.NextAction = strings.TrimSpace(payload.NextAction)
	payload.CaptureTail = strings.TrimSpace(payload.CaptureTail)
	return &payload, nil
}

func routePromptFromState(input routeStateInput, fallback string) string {
	session := strings.TrimSpace(input.Session)
	state := strings.TrimSpace(input.SessionState)
	reason := strings.TrimSpace(input.Reason)
	next := strings.TrimSpace(input.NextAction)
	if session == "" && state == "" && reason == "" && next == "" {
		return fallback
	}
	lines := []string{"Continue orchestration from existing state context."}
	if session != "" {
		lines = append(lines, "Session: "+session)
	}
	if state != "" {
		lines = append(lines, "State: "+state)
	}
	if reason != "" {
		lines = append(lines, "Reason: "+reason)
	}
	if next != "" {
		lines = append(lines, "Recommended next action: "+next)
	}
	lines = append(lines, "Return concrete next steps.")
	return strings.Join(lines, "\n")
}

func loadRouteStateInput(from string) (*routeStateInput, error) {
	source := strings.TrimSpace(from)
	if source == "" {
		return nil, fmt.Errorf("empty from-state source")
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

	// Accept both handoff/status payload shapes.
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	input := routeStateInput{
		Session:      mapStringValue(generic, "session"),
		Status:       mapStringValue(generic, "status"),
		SessionState: mapStringValue(generic, "sessionState"),
		Reason:       mapStringValue(generic, "reason"),
		NextAction:   mapStringValue(generic, "nextAction"),
	}
	if input.Reason == "" {
		input.Reason = mapStringValue(generic, "classificationReason")
	}
	if input.Status == "" && input.SessionState != "" {
		input.Status = input.SessionState
	}
	if input.Session == "" && input.SessionState == "" && input.Reason == "" && input.NextAction == "" {
		return nil, fmt.Errorf("from-state payload missing session/state fields")
	}
	return &input, nil
}

func mapStringValue(m map[string]any, key string) string {
	value, ok := m[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func objectiveSummaryLine(objective map[string]any) string {
	if objective == nil {
		return ""
	}
	parts := []string{}
	if id := strings.TrimSpace(mapStringValue(objective, "id")); id != "" {
		parts = append(parts, "id="+id)
	}
	if goal := strings.TrimSpace(mapStringValue(objective, "goal")); goal != "" {
		parts = append(parts, "goal="+goal)
	}
	if acceptance := strings.TrimSpace(mapStringValue(objective, "acceptance")); acceptance != "" {
		parts = append(parts, "acceptance="+acceptance)
	}
	if budget := strings.TrimSpace(mapStringValue(objective, "budget")); budget != "" && budget != "0" {
		parts = append(parts, "budget="+budget)
	}
	return strings.Join(parts, " | ")
}

func recommendedCommandForAction(action, session, projectRoot string) string {
	switch action {
	case "session send":
		return "./lisa session send --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --text " + shellQuote("Continue from objective and latest state.") + " --enter --json-min"
	case "session monitor":
		return "./lisa session monitor --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --json-min"
	case "session capture":
		return "./lisa session capture --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --raw --summary --summary-style ops --json"
	case "session explain":
		return "./lisa session explain --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --events 40 --json-min"
	case "session spawn":
		return "./lisa session spawn --agent codex --mode interactive --project-root " + shellQuote(projectRoot) + " --json"
	default:
		return "./lisa session status --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --json-min"
	}
}

func deriveHandoffRisks(status sessionStatus, items []sessionHandoffItem) []sessionHandoffRisk {
	risks := []sessionHandoffRisk{}
	switch status.SessionState {
	case "crashed", "stuck":
		risks = append(risks, sessionHandoffRisk{
			Level:   "high",
			Code:    "terminal_failure",
			Message: "session reached terminal failure state",
		})
	case "degraded":
		risks = append(risks, sessionHandoffRisk{
			Level:   "medium",
			Code:    "degraded_loop",
			Message: "session is degraded; monitor and recovery may be required",
		})
	}
	if len(items) == 0 {
		risks = append(risks, sessionHandoffRisk{
			Level:   "low",
			Code:    "no_recent_events",
			Message: "handoff has no recent event history",
		})
	}
	return risks
}

func deriveHandoffQuestions(status sessionStatus, items []sessionHandoffItem) []sessionHandoffQuestion {
	questions := []sessionHandoffQuestion{}
	if status.SessionState == "waiting_input" {
		questions = append(questions, sessionHandoffQuestion{
			Code:     "next_instruction",
			Question: "What exact instruction should be sent next?",
		})
	}
	if len(items) == 0 {
		questions = append(questions, sessionHandoffQuestion{
			Code:     "context_gap",
			Question: "Should events be increased to include more execution history?",
		})
	}
	return questions
}

func deterministicHandoffID(session, state, kind string, ordinal int) string {
	base := strings.TrimSpace(session) + "|" + strings.TrimSpace(state) + "|" + strings.TrimSpace(kind) + "|" + strconv.Itoa(ordinal)
	sum := sha1Hex(base)
	return "hid-" + sum[:12]
}

func handoffRisksV3(session, state string, risks []sessionHandoffRisk) []map[string]any {
	out := make([]map[string]any, 0, len(risks))
	for idx, risk := range risks {
		level := strings.TrimSpace(risk.Level)
		if level == "" {
			level = "low"
		}
		code := strings.TrimSpace(risk.Code)
		if code == "" {
			code = "unknown_risk"
		}
		out = append(out, map[string]any{
			"id":      deterministicHandoffID(session, state, "risk:"+code, idx),
			"level":   level,
			"code":    code,
			"message": strings.TrimSpace(risk.Message),
		})
	}
	return out
}

func handoffQuestionsV3(session, state string, questions []sessionHandoffQuestion) []map[string]any {
	out := make([]map[string]any, 0, len(questions))
	for idx, question := range questions {
		code := strings.TrimSpace(question.Code)
		if code == "" {
			code = "unknown_question"
		}
		out = append(out, map[string]any{
			"id":       deterministicHandoffID(session, state, "question:"+code, idx),
			"code":     code,
			"question": strings.TrimSpace(question.Question),
		})
	}
	return out
}

func buildRouteQueue(projectRoot, sessionsRaw string, limit int, budget int, concurrency int) ([]map[string]any, error) {
	sessions := parseCommaValues(sessionsRaw)
	projectRoot = canonicalProjectRoot(projectRoot)
	enumeratedSessions := false
	if len(sessions) == 0 {
		restore := withProjectRuntimeEnv(projectRoot)
		list, err := tmuxListSessionsFn(true, projectRoot)
		restore()
		if err != nil {
			return nil, err
		}
		sessions = list
		enumeratedSessions = true
	}
	if len(sessions) == 0 {
		return []map[string]any{}, nil
	}
	type queueItem struct {
		Session      string
		Status       sessionStatus
		NextAction   string
		Command      string
		Reason       string
		Priority     int
		PriorityType string
	}
	items := make([]queueItem, 0, len(sessions))
	for _, session := range sessions {
		resolvedRoot, resolveErr := resolveSessionProjectRootChecked(session, projectRoot, false)
		if resolveErr != nil {
			if isSessionMetaAmbiguousError(resolveErr) {
				items = append(items, queueItem{
					Session: session,
					Status: sessionStatus{
						Session:              session,
						Status:               "unknown",
						SessionState:         "ambiguous_project_root",
						ClassificationReason: "metadata_ambiguous",
					},
					NextAction:   "provide_project_root",
					Command:      "./lisa session status --session " + shellQuote(session) + " --project-root " + shellQuote(projectRoot) + " --json",
					Reason:       resolveErr.Error(),
					Priority:     98,
					PriorityType: "high",
				})
				continue
			}
			resolvedRoot = projectRoot
		}
		if enumeratedSessions && canonicalProjectRoot(resolvedRoot) != projectRoot {
			continue
		}
		restore := withProjectRuntimeEnv(resolvedRoot)
		status, err := computeSessionStatusFn(session, resolvedRoot, "auto", "auto", false, 0)
		restore()
		if err != nil {
			continue
		}
		status = normalizeStatusForSessionStatusOutput(status)
		nextAction, command, reason := recommendedSessionNext(status, session, resolvedRoot, budget)
		priority, priorityType := computeSessionPriority(status)
		items = append(items, queueItem{
			Session:      session,
			Status:       status,
			NextAction:   nextAction,
			Command:      command,
			Reason:       reason,
			Priority:     priority,
			PriorityType: priorityType,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			return items[i].Session < items[j].Session
		}
		return items[i].Priority > items[j].Priority
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]map[string]any, 0, len(items))
	for idx, item := range items {
		dispatchWave := 1
		dispatchSlot := 1
		if concurrency > 0 {
			dispatchWave = (idx / concurrency) + 1
			dispatchSlot = (idx % concurrency) + 1
		}
		out = append(out, map[string]any{
			"position":      idx + 1,
			"dispatchWave":  dispatchWave,
			"dispatchSlot":  dispatchSlot,
			"session":       item.Session,
			"status":        item.Status.Status,
			"sessionState":  item.Status.SessionState,
			"nextAction":    item.NextAction,
			"command":       item.Command,
			"reason":        item.Reason,
			"priorityScore": item.Priority,
			"priorityLabel": item.PriorityType,
		})
	}
	return out, nil
}

func buildRouteDispatchPlan(queueItems []map[string]any, concurrency int) []map[string]any {
	if concurrency <= 0 {
		concurrency = 1
	}
	waves := map[int][]string{}
	maxWave := 0
	for _, item := range queueItems {
		wave := 1
		if parsed, ok := numberFromAny(item["dispatchWave"]); ok && parsed > 0 {
			wave = parsed
		}
		if wave > maxWave {
			maxWave = wave
		}
		waves[wave] = append(waves[wave], mapStringValue(item, "session"))
	}
	out := make([]map[string]any, 0, maxWave)
	for wave := 1; wave <= maxWave; wave++ {
		sessions := waves[wave]
		if len(sessions) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"wave":        wave,
			"concurrency": concurrency,
			"sessions":    sessions,
		})
	}
	return out
}

func sha1Hex(input string) string {
	sum := sha1.Sum([]byte(input))
	return hex.EncodeToString(sum[:])
}

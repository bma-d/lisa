package app

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

var computeSessionStatusFn = computeSessionStatus
var monitorWaitingTurnCompleteFn = monitorWaitingTurnComplete
var captureSessionTranscriptFn = captureSessionTranscript
var shouldUseTranscriptCaptureFn = shouldUseTranscriptCapture

type waitingTurnCompleteResult struct {
	Ready        bool
	InputAtNanos int64
	FileAge      int
}

type monitorResultMin struct {
	Session    string `json:"session"`
	FinalState string `json:"finalState"`
	ExitReason string `json:"exitReason"`
	Polls      int    `json:"polls"`
	NextOffset int    `json:"nextOffset,omitempty"`
}

type monitorStreamPollMin struct {
	Type         string `json:"type"`
	Poll         int    `json:"poll"`
	Session      string `json:"session"`
	Status       string `json:"status"`
	SessionState string `json:"sessionState"`
	TodosDone    int    `json:"todosDone"`
	TodosTotal   int    `json:"todosTotal"`
	WaitEstimate int    `json:"waitEstimate"`
}

type sessionStatusMin struct {
	Session      string `json:"session"`
	Status       string `json:"status"`
	SessionState string `json:"sessionState"`
	TodosDone    int    `json:"todosDone"`
	TodosTotal   int    `json:"todosTotal"`
	WaitEstimate int    `json:"waitEstimate"`
}

func normalizeStatusForSessionStatusOutput(status sessionStatus) sessionStatus {
	normalized := status
	switch normalized.SessionState {
	case "completed", "crashed", "stuck", "not_found":
		normalized.Status = normalized.SessionState
	}
	return normalized
}

func writeSessionStatusJSON(status sessionStatus, errorCode string, jsonMin bool) {
	if jsonMin {
		minPayload := sessionStatusMin{
			Session:      status.Session,
			Status:       status.Status,
			SessionState: status.SessionState,
			TodosDone:    status.TodosDone,
			TodosTotal:   status.TodosTotal,
			WaitEstimate: status.WaitEstimate,
		}
		payload := map[string]any{
			"session":      minPayload.Session,
			"status":       minPayload.Status,
			"sessionState": minPayload.SessionState,
			"todosDone":    minPayload.TodosDone,
			"todosTotal":   minPayload.TodosTotal,
			"waitEstimate": minPayload.WaitEstimate,
		}
		if errorCode != "" {
			payload["errorCode"] = errorCode
		}
		writeJSON(payload)
		return
	}

	payload := map[string]any{
		"session":               status.Session,
		"agent":                 status.Agent,
		"mode":                  status.Mode,
		"status":                status.Status,
		"todosDone":             status.TodosDone,
		"todosTotal":            status.TodosTotal,
		"activeTask":            status.ActiveTask,
		"waitEstimate":          status.WaitEstimate,
		"sessionState":          status.SessionState,
		"paneStatus":            status.PaneStatus,
		"paneCommand":           status.PaneCommand,
		"agentPid":              status.AgentPID,
		"agentCpu":              status.AgentCPU,
		"outputAgeSeconds":      status.OutputAgeSeconds,
		"outputFreshSeconds":    status.OutputFreshSeconds,
		"heartbeatAgeSeconds":   status.HeartbeatAge,
		"heartbeatFreshSeconds": status.HeartbeatFreshSecs,
		"classificationReason":  status.ClassificationReason,
		"signals":               status.Signals,
	}
	if status.OutputFile != "" {
		payload["outputFile"] = status.OutputFile
	}
	if errorCode != "" {
		payload["errorCode"] = errorCode
	}
	writeJSON(payload)
}

func writeMonitorJSON(result monitorResult, jsonMin bool, errorCode string) {
	if jsonMin {
		minPayload := monitorResultMin{
			Session:    result.Session,
			FinalState: result.FinalState,
			ExitReason: result.ExitReason,
			Polls:      result.Polls,
			NextOffset: result.NextOffset,
		}
		payload := map[string]any{
			"session":    minPayload.Session,
			"finalState": minPayload.FinalState,
			"exitReason": minPayload.ExitReason,
			"polls":      minPayload.Polls,
		}
		if minPayload.NextOffset > 0 {
			payload["nextOffset"] = minPayload.NextOffset
		}
		if errorCode != "" {
			payload["errorCode"] = errorCode
		}
		writeJSON(payload)
		return
	}
	payload := map[string]any{
		"finalState":  result.FinalState,
		"session":     result.Session,
		"todosDone":   result.TodosDone,
		"todosTotal":  result.TodosTotal,
		"outputFile":  result.OutputFile,
		"nextOffset":  result.NextOffset,
		"exitReason":  result.ExitReason,
		"polls":       result.Polls,
		"finalStatus": result.FinalStatus,
	}
	if result.OutputFile == "" {
		delete(payload, "outputFile")
	}
	if result.NextOffset <= 0 {
		delete(payload, "nextOffset")
	}
	if errorCode != "" {
		payload["errorCode"] = errorCode
	}
	writeJSON(payload)
}

func writeMonitorStreamPoll(status sessionStatus, poll int, jsonMin bool) {
	if jsonMin {
		minPayload := monitorStreamPollMin{
			Type:         "poll",
			Poll:         poll,
			Session:      status.Session,
			Status:       normalizeMonitorFinalStatus(status.SessionState, status.Status),
			SessionState: status.SessionState,
			TodosDone:    status.TodosDone,
			TodosTotal:   status.TodosTotal,
			WaitEstimate: status.WaitEstimate,
		}
		writeJSON(minPayload)
		return
	}
	writeJSON(map[string]any{
		"type":                 "poll",
		"poll":                 poll,
		"session":              status.Session,
		"status":               normalizeMonitorFinalStatus(status.SessionState, status.Status),
		"sessionState":         status.SessionState,
		"activeTask":           status.ActiveTask,
		"waitEstimate":         status.WaitEstimate,
		"todosDone":            status.TodosDone,
		"todosTotal":           status.TodosTotal,
		"classificationReason": status.ClassificationReason,
	})
}

func normalizeMonitorFinalStatus(finalState, finalStatus string) string {
	switch finalState {
	case "completed", "crashed", "stuck", "not_found":
		return finalState
	default:
		return finalStatus
	}
}

func validateMonitorExpectationConfig(expect, untilMarker string) error {
	if expect == "marker" && strings.TrimSpace(untilMarker) == "" {
		return fmt.Errorf("--expect marker requires --until-marker")
	}
	return nil
}

func cmdSessionStatus(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	full := false
	failNotFound := false
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session status")
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = args[i+1]
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
		case "--full":
			full = true
		case "--fail-not-found":
			failNotFound = true
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
	agentHint, err := parseAgentHint(agentHint)
	if err != nil {
		return commandError(jsonOut, "invalid_agent_hint", err.Error())
	}
	modeHint, err = parseModeHint(modeHint)
	if err != nil {
		return commandError(jsonOut, "invalid_mode_hint", err.Error())
	}

	status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, full, 0)
	if err != nil {
		return commandError(jsonOut, "status_compute_failed", err.Error())
	}
	status = normalizeStatusForSessionStatusOutput(status)

	if jsonOut {
		errorCode := ""
		if failNotFound && status.SessionState == "not_found" {
			errorCode = "session_not_found"
		}
		writeSessionStatusJSON(status, errorCode, jsonMin)
		if failNotFound && status.SessionState == "not_found" {
			return 1
		}
		return 0
	}

	if full {
		if err := writeCSVRecord(
			"status_full_v1",
			status.Status,
			strconv.Itoa(status.TodosDone),
			strconv.Itoa(status.TodosTotal),
			status.ActiveTask,
			strconv.Itoa(status.WaitEstimate),
			status.SessionState,
			status.ClassificationReason,
			status.PaneStatus,
			strconv.Itoa(status.AgentPID),
			fmt.Sprintf("%.2f", status.AgentCPU),
			strconv.Itoa(status.OutputAgeSeconds),
			strconv.Itoa(status.HeartbeatAge),
			strconv.FormatBool(status.Signals.PromptWaiting),
			strconv.FormatBool(status.Signals.HeartbeatFresh),
			strconv.FormatBool(status.Signals.StateLockTimedOut),
			strconv.Itoa(status.Signals.StateLockWaitMS),
			status.Signals.AgentScanError,
			status.Signals.TMUXReadError,
			status.Signals.StateReadError,
			status.Signals.MetaReadError,
			status.Signals.DoneFileReadError,
		); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write status output: %v\n", err)
			return 1
		}
		if failNotFound && status.SessionState == "not_found" {
			return 1
		}
		return 0
	}

	if err := writeCSVRecord(
		status.Status,
		strconv.Itoa(status.TodosDone),
		strconv.Itoa(status.TodosTotal),
		status.ActiveTask,
		strconv.Itoa(status.WaitEstimate),
		status.SessionState,
	); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write status output: %v\n", err)
		return 1
	}
	if failNotFound && status.SessionState == "not_found" {
		return 1
	}
	return 0
}

func cmdSessionMonitor(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	expect := "any"
	pollInterval := defaultPollIntervalSeconds
	maxPolls := defaultMaxPolls
	stopOnWaiting := true
	waitingRequiresTurnComplete := false
	untilMarker := ""
	untilMarkerSet := false
	jsonOut := hasJSONFlag(args)
	jsonMin := false
	streamJSON := false
	verbose := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session monitor")
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = args[i+1]
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
		case "--expect":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --expect")
			}
			parsedExpect, err := parseMonitorExpect(args[i+1])
			if err != nil {
				return commandError(jsonOut, "invalid_expect", err.Error())
			}
			expect = parsedExpect
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
		case "--stop-on-waiting":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --stop-on-waiting")
			}
			parsed, err := parseBoolFlag(args[i+1])
			if err != nil {
				return commandErrorf(jsonOut, "invalid_stop_on_waiting", "invalid --stop-on-waiting: %s (expected true|false)", args[i+1])
			}
			stopOnWaiting = parsed
			i++
		case "--waiting-requires-turn-complete":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --waiting-requires-turn-complete")
			}
			parsed, err := parseBoolFlag(args[i+1])
			if err != nil {
				return commandErrorf(jsonOut, "invalid_waiting_requires_turn_complete", "invalid --waiting-requires-turn-complete: %s (expected true|false)", args[i+1])
			}
			waitingRequiresTurnComplete = parsed
			i++
		case "--until-marker":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --until-marker")
			}
			untilMarkerSet = true
			untilMarker = strings.TrimSpace(args[i+1])
			i++
		case "--json":
			jsonOut = true
		case "--json-min":
			jsonMin = true
			jsonOut = true
		case "--stream-json":
			streamJSON = true
			jsonOut = true
		case "--verbose":
			verbose = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--session is required")
	}
	if untilMarkerSet && untilMarker == "" {
		return commandError(jsonOut, "invalid_until_marker", "invalid --until-marker: cannot be empty")
	}
	if err := validateMonitorExpectationConfig(expect, untilMarker); err != nil {
		return commandError(jsonOut, "expect_marker_requires_until_marker", err.Error())
	}
	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	agentHint, err := parseAgentHint(agentHint)
	if err != nil {
		return commandError(jsonOut, "invalid_agent_hint", err.Error())
	}
	modeHint, err = parseModeHint(modeHint)
	if err != nil {
		return commandError(jsonOut, "invalid_mode_hint", err.Error())
	}

	last := sessionStatus{}
	degradedPolls := 0
	for poll := 1; poll <= maxPolls; poll++ {
		status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, true, poll)
		if err != nil {
			return commandError(jsonOut, "status_compute_failed", err.Error())
		}
		last = status
		if status.SessionState == "degraded" {
			degradedPolls++
		}

		if verbose {
			displayStatus := normalizeMonitorFinalStatus(status.SessionState, status.Status)
			fmt.Fprintf(os.Stderr, "[%s] poll=%d state=%s status=%s active=%q wait=%ds\n",
				time.Now().Format("15:04:05"), poll, status.SessionState, displayStatus, status.ActiveTask, status.WaitEstimate)
		}
		if streamJSON {
			writeMonitorStreamPoll(status, poll, jsonMin)
		}

		reason := ""
		if untilMarker != "" {
			capture, captureErr := tmuxCapturePaneFn(session, 320)
			if captureErr == nil && strings.Contains(capture, untilMarker) {
				reason = "marker_found"
			}
		}
		switch status.SessionState {
		case "completed":
			if reason == "" {
				reason = "completed"
			}
		case "crashed":
			if reason == "" {
				reason = "crashed"
			}
		case "not_found":
			if reason == "" {
				reason = "not_found"
			}
		case "stuck":
			if reason == "" {
				reason = "stuck"
			}
		case "waiting_input":
			if reason == "" && stopOnWaiting {
				if waitingRequiresTurnComplete {
					waitingTurn := monitorWaitingTurnCompleteFn(session, projectRoot, status)
					if waitingTurn.Ready {
						status.Signals.TranscriptTurnComplete = true
						status.Signals.TranscriptFileAge = waitingTurn.FileAge
						if err := recordSessionTurnComplete(projectRoot, session, waitingTurn.InputAtNanos, waitingTurn.FileAge); err != nil {
							fmt.Fprintf(os.Stderr, "observability warning: failed to persist turn-complete marker: %v\n", err)
						}
						reason = "waiting_input_turn_complete"
					}
				} else {
					reason = "waiting_input"
				}
			}
		}
		if reason != "" {
			expectationMet := monitorExpectationSatisfied(expect, reason)
			finalReason := reason
			if !expectationMet {
				finalReason = monitorExpectationMismatchReason(expect, reason)
			}
			result := monitorResult{
				FinalState:  status.SessionState,
				Session:     status.Session,
				TodosDone:   status.TodosDone,
				TodosTotal:  status.TodosTotal,
				OutputFile:  status.OutputFile,
				NextOffset:  computeSessionCaptureNextOffset(session),
				ExitReason:  finalReason,
				Polls:       poll,
				FinalStatus: normalizeMonitorFinalStatus(status.SessionState, status.Status),
			}
			if jsonOut {
				errorCode := ""
				if !expectationMet {
					errorCode = "monitor_expectation_mismatch"
				} else if !(reason == "completed" || reason == "marker_found" || strings.HasPrefix(reason, "waiting_input")) {
					errorCode = "monitor_" + reason
				}
				writeMonitorJSON(result, jsonMin, errorCode)
			} else {
				if err := writeCSVRecord(
					result.FinalState,
					strconv.Itoa(result.TodosDone),
					strconv.Itoa(result.TodosTotal),
					result.OutputFile,
					result.ExitReason,
					strconv.Itoa(result.Polls),
					result.FinalStatus,
				); err != nil {
					fmt.Fprintf(os.Stderr, "failed to write monitor output: %v\n", err)
					return 1
				}
			}
			if err := appendLifecycleEvent(projectRoot, session, "lifecycle", result.FinalState, result.FinalStatus, "monitor_"+finalReason); err != nil {
				fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
			}
			if !expectationMet {
				return 2
			}
			if reason == "completed" || reason == "marker_found" || strings.HasPrefix(reason, "waiting_input") {
				return 0
			}
			return 2
		}

		if poll < maxPolls {
			time.Sleep(time.Duration(pollInterval) * time.Second)
		}
	}

	result := monitorResult{
		FinalState:  "timeout",
		Session:     session,
		TodosDone:   last.TodosDone,
		TodosTotal:  last.TodosTotal,
		OutputFile:  last.OutputFile,
		NextOffset:  computeSessionCaptureNextOffset(session),
		ExitReason:  "max_polls_exceeded",
		Polls:       maxPolls,
		FinalStatus: "timeout",
	}
	if degradedPolls == maxPolls && maxPolls > 0 {
		result.ExitReason = "degraded_max_polls_exceeded"
	}
	if jsonOut {
		writeMonitorJSON(result, jsonMin, "monitor_timeout")
	} else {
		if err := writeCSVRecord(
			result.FinalState,
			strconv.Itoa(result.TodosDone),
			strconv.Itoa(result.TodosTotal),
			result.OutputFile,
			result.ExitReason,
			strconv.Itoa(result.Polls),
			result.FinalStatus,
		); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write monitor output: %v\n", err)
			return 1
		}
	}
	if err := appendLifecycleEvent(projectRoot, session, "lifecycle", result.FinalState, result.FinalStatus, "monitor_"+result.ExitReason); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}
	return 2
}

func computeSessionCaptureNextOffset(session string) int {
	if strings.TrimSpace(session) == "" || !tmuxHasSessionFn(session) {
		return 0
	}
	capture, err := tmuxCapturePaneFn(session, 320)
	if err != nil {
		return 0
	}
	capture = strings.Join(trimLines(capture), "\n")
	capture = filterCaptureNoise(capture)
	return len(capture)
}

func monitorWaitingTurnComplete(session, projectRoot string, status sessionStatus) waitingTurnCompleteResult {
	result := waitingTurnCompleteResult{}
	if status.SessionState != "waiting_input" {
		return result
	}

	meta, err := loadSessionMeta(projectRoot, session)
	if err != nil {
		return result
	}
	agent := normalizeAgent(meta.Agent)
	if agent == "" {
		agent = normalizeAgent(status.Agent)
	}
	mode := strings.ToLower(strings.TrimSpace(meta.Mode))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(status.Mode))
	}
	if mode != "interactive" {
		return result
	}

	prompt := strings.TrimSpace(meta.Prompt)
	createdAt := strings.TrimSpace(meta.CreatedAt)
	if prompt == "" || createdAt == "" {
		return result
	}

	state, _ := loadSessionStateWithError(sessionStateFile(projectRoot, session))
	lastInputAtNanos := state.LastInputAtNanos
	if lastInputAtNanos <= 0 && state.LastInputAt > 0 {
		lastInputAtNanos = state.LastInputAt * int64(time.Second)
	}

	switch agent {
	case "claude":
		turnComplete, fileAge, sessionID, err := checkTranscriptTurnCompleteFn(meta.ProjectRoot, prompt, createdAt, state.ClaudeSessionID)
		if err != nil {
			return result
		}
		cacheTranscriptSessionID(projectRoot, session, "claude", sessionID)
		if !turnComplete {
			return result
		}
		if !transcriptLikelyIncludesLatestInput(lastInputAtNanos, fileAge) {
			return result
		}
		result.Ready = true
		result.InputAtNanos = lastInputAtNanos
		result.FileAge = fileAge
		return result
	case "codex":
		turnComplete, fileAge, sessionID, err := checkCodexTranscriptTurnCompleteFn(prompt, createdAt, state.CodexSessionID)
		if err != nil {
			return result
		}
		cacheTranscriptSessionID(projectRoot, session, "codex", sessionID)
		if !turnComplete {
			return result
		}
		if !transcriptLikelyIncludesLatestInput(lastInputAtNanos, fileAge) {
			return result
		}
		if lastInputAtNanos > 0 {
			hasAssistantTurn, err := codexHasAssistantTurnSinceFn(sessionID, lastInputAtNanos)
			if err != nil || !hasAssistantTurn {
				return result
			}
		}
		result.Ready = true
		result.InputAtNanos = lastInputAtNanos
		result.FileAge = fileAge
		return result
	default:
		return result
	}
}

func transcriptLikelyIncludesLatestInput(lastInputAtNanos int64, fileAge int) bool {
	if lastInputAtNanos <= 0 {
		return true
	}
	if fileAge < 0 {
		return false
	}
	latestPossibleWrite := nowFn().Add(-time.Duration(fileAge) * time.Second).UnixNano()
	const transcriptClockSkewTolerance = int64(2 * time.Second)
	return latestPossibleWrite+transcriptClockSkewTolerance >= lastInputAtNanos
}

func cacheTranscriptSessionID(projectRoot, session, agent, sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	statePath := sessionStateFile(projectRoot, session)
	_, err := withStateFileLockFn(statePath, func() error {
		state, loadErr := loadSessionStateWithError(statePath)
		if loadErr != nil {
			state = sessionState{}
		}
		switch strings.ToLower(strings.TrimSpace(agent)) {
		case "claude":
			if state.ClaudeSessionID == sessionID {
				return nil
			}
			state.ClaudeSessionID = sessionID
		case "codex":
			if state.CodexSessionID == sessionID {
				return nil
			}
			state.CodexSessionID = sessionID
		default:
			return nil
		}
		return saveSessionState(statePath, state)
	})
	if err != nil {
		// Best-effort cache update; monitor semantics should not fail on this.
		return
	}
}

func recordSessionTurnComplete(projectRoot, session string, inputAtNanos int64, fileAge int) error {
	statePath := sessionStateFile(projectRoot, session)
	_, err := withStateFileLockFn(statePath, func() error {
		state, loadErr := loadSessionStateWithError(statePath)
		if loadErr != nil {
			state = sessionState{}
		}
		state.LastTurnCompleteAtNanos = nowFn().UnixNano()
		state.LastTurnCompleteInputNanos = inputAtNanos
		state.LastTurnCompleteFileAge = fileAge
		return saveSessionState(statePath, state)
	})
	return err
}

func cmdSessionCapture(args []string) int {
	session := ""
	lines := 200
	jsonOut := hasJSONFlag(args)
	jsonMin := false
	raw := false
	deltaFrom := ""
	markersRaw := ""
	stripNoise := true
	projectRoot := getPWD()
	projectRootExplicit := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session capture")
		case "--session":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --session")
			}
			session = args[i+1]
			i++
		case "--lines":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --lines")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return commandError(jsonOut, "invalid_lines", "invalid --lines")
			}
			lines = n
			i++
		case "--raw":
			raw = true
		case "--delta-from":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --delta-from")
			}
			deltaFrom = strings.TrimSpace(args[i+1])
			i++
		case "--markers":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --markers")
			}
			markersRaw = args[i+1]
			i++
		case "--keep-noise":
			stripNoise = false
		case "--strip-noise":
			stripNoise = true
		case "--project-root":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --project-root")
			}
			projectRoot = args[i+1]
			projectRootExplicit = true
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
	if deltaFrom != "" && !raw {
		return commandError(jsonOut, "delta_requires_raw_capture", "--delta-from requires --raw")
	}
	markers, err := parseCaptureMarkersFlag(markersRaw)
	if err != nil {
		return commandError(jsonOut, "invalid_markers", err.Error())
	}

	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	transcriptProjectRoot := projectRoot
	if !projectRootExplicit {
		// Prefer the current project by default (USAGE contract). If no local
		// metadata exists, fall back to global metadata lookup for compatibility.
		if _, err := loadSessionMeta(projectRoot, session); err != nil {
			transcriptProjectRoot = ""
		}
	}

	if !raw && shouldUseTranscriptCaptureFn(session, transcriptProjectRoot) {
		sessionID, messages, err := captureSessionTranscriptFn(session, transcriptProjectRoot)
		if err == nil {
			return writeTranscriptCapture(session, sessionID, messages, jsonOut, jsonMin)
		}
	}

	if !tmuxHasSessionFn(session) {
		if jsonOut {
			writeJSONError("session_not_found", "session not found", map[string]any{
				"session":     session,
				"projectRoot": projectRoot,
			})
			return 1
		}
		fmt.Fprintln(os.Stderr, "session not found")
		return 1
	}
	capture, err := tmuxCapturePaneFn(session, lines)
	if err != nil {
		return commandErrorf(jsonOut, "capture_failed", "failed to capture pane: %v", err)
	}
	capture = strings.Join(trimLines(capture), "\n")
	if stripNoise {
		capture = filterCaptureNoise(capture)
	}
	if err := updateCaptureState(projectRoot, session, capture); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: failed to update capture state: %v\n", err)
	}

	deltaMode := ""
	nextOffset := 0
	if deltaFrom != "" {
		capture, deltaMode, nextOffset, err = applyCaptureDelta(projectRoot, session, capture, deltaFrom)
		if err != nil {
			return commandError(jsonOut, "invalid_delta_from", err.Error())
		}
	}

	markerSummary := captureMarkerSummary{}
	if len(markers) > 0 {
		markerSummary = buildCaptureMarkerSummary(capture, markers)
	}

	if jsonOut {
		payload := map[string]any{"session": session, "capture": capture}
		if len(markers) > 0 {
			delete(payload, "capture")
			payload["markers"] = markerSummary.Markers
			payload["markerMatches"] = markerSummary.Matches
			payload["foundMarkers"] = markerSummary.Found
			payload["missingMarkers"] = markerSummary.Missing
			if !jsonMin {
				payload["markerCounts"] = markerSummary.Counts
			}
		}
		if deltaFrom != "" {
			if !jsonMin {
				payload["deltaFrom"] = deltaFrom
				payload["deltaMode"] = deltaMode
			}
			payload["nextOffset"] = nextOffset
		}
		writeJSON(payload)
		return 0
	}
	if len(markers) > 0 {
		for _, marker := range markerSummary.Markers {
			fmt.Printf("%s=%t\n", marker, markerSummary.Matches[marker])
		}
		return 0
	}
	fmt.Print(capture)
	return 0
}

func updateCaptureState(projectRoot, session, capture string) error {
	statePath := sessionStateFile(projectRoot, session)
	_, err := withStateFileLockFn(statePath, func() error {
		state, loadErr := loadSessionStateWithError(statePath)
		if loadErr != nil {
			state = sessionState{}
		}
		hash := md5Hex8(capture)
		if state.LastOutputHash == hash {
			return nil
		}
		now := nowFn()
		state.LastOutputHash = hash
		state.LastOutputAt = now.Unix()
		state.LastOutputAtNanos = now.UnixNano()
		return saveSessionState(statePath, state)
	})
	return err
}

func applyCaptureDelta(projectRoot, session, capture, deltaFrom string) (string, string, int, error) {
	fullLen := len(capture)
	if deltaFrom == "" {
		return capture, "", fullLen, nil
	}

	if offset, ok := parseCaptureOffset(deltaFrom); ok {
		if offset < 0 {
			offset = 0
		}
		if offset > fullLen {
			offset = fullLen
		}
		return capture[offset:], "offset", fullLen, nil
	}

	cutoffNanos, err := parseCaptureTimestamp(deltaFrom)
	if err != nil {
		return "", "", 0, err
	}
	lastOutputAtNanos, err := loadCaptureLastOutputAtNanos(projectRoot, session)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to load capture state: %w", err)
	}
	if lastOutputAtNanos <= 0 || lastOutputAtNanos > cutoffNanos {
		return capture, "timestamp", fullLen, nil
	}
	return "", "timestamp", fullLen, nil
}

func parseCaptureOffset(deltaFrom string) (int, bool) {
	if strings.HasPrefix(deltaFrom, "@") {
		return 0, false
	}
	if strings.Contains(deltaFrom, "T") || strings.Contains(deltaFrom, "-") || strings.Contains(deltaFrom, ":") {
		return 0, false
	}
	n, err := strconv.Atoi(deltaFrom)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseCaptureTimestamp(deltaFrom string) (int64, error) {
	value := strings.TrimSpace(deltaFrom)
	if value == "" {
		return 0, fmt.Errorf("invalid --delta-from: empty value")
	}
	if strings.HasPrefix(value, "@") {
		raw := strings.TrimSpace(strings.TrimPrefix(value, "@"))
		if raw == "" {
			return 0, fmt.Errorf("invalid --delta-from timestamp: missing unix value after @")
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid --delta-from timestamp: %s", deltaFrom)
		}
		switch {
		case n > 1_000_000_000_000_000_000:
			return n, nil
		case n > 1_000_000_000_000:
			if n > math.MaxInt64/int64(time.Millisecond) {
				return math.MaxInt64, nil
			}
			return n * int64(time.Millisecond), nil
		default:
			if n > math.MaxInt64/int64(time.Second) {
				return math.MaxInt64, nil
			}
			return n * int64(time.Second), nil
		}
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts.UnixNano(), nil
	}
	return 0, fmt.Errorf("invalid --delta-from: expected offset integer, @unix timestamp, or RFC3339 timestamp")
}

func loadCaptureLastOutputAtNanos(projectRoot, session string) (int64, error) {
	state, err := loadSessionStateWithError(sessionStateFile(projectRoot, session))
	if err != nil {
		return 0, err
	}
	if state.LastOutputAtNanos > 0 {
		return state.LastOutputAtNanos, nil
	}
	if state.LastOutputAt > 0 {
		return state.LastOutputAt * int64(time.Second), nil
	}
	return 0, nil
}

func shouldUseTranscriptCapture(session, projectRoot string) bool {
	root := strings.TrimSpace(projectRoot)
	if root != "" {
		meta, err := loadSessionMeta(canonicalProjectRoot(root), session)
		if err != nil {
			return false
		}
		return normalizeAgent(meta.Agent) == "claude"
	}

	meta, err := loadSessionMetaByGlobFn(session)
	if err != nil {
		return false
	}
	return normalizeAgent(meta.Agent) == "claude"
}

func parseAgentHint(agent string) (string, error) {
	a := strings.ToLower(strings.TrimSpace(agent))
	if a == "" || a == "auto" {
		return "auto", nil
	}
	parsed, err := parseAgent(a)
	if err != nil {
		return "", fmt.Errorf("invalid --agent: %s (expected auto|claude|codex)", agent)
	}
	return parsed, nil
}

func parseModeHint(mode string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "" || m == "auto" {
		return "auto", nil
	}
	parsed, err := parseMode(m)
	if err != nil {
		return "", fmt.Errorf("invalid --mode: %s (expected auto|interactive|exec)", mode)
	}
	return parsed, nil
}

func parseMonitorExpect(expect string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(expect))
	switch value {
	case "", "any":
		return "any", nil
	case "terminal", "marker":
		return value, nil
	default:
		return "", fmt.Errorf("invalid --expect: %s (expected any|terminal|marker)", expect)
	}
}

func monitorExpectationSatisfied(expect, reason string) bool {
	switch expect {
	case "marker":
		return reason == "marker_found"
	case "terminal":
		return reason == "completed" || reason == "crashed" || reason == "stuck" || reason == "not_found"
	default:
		return true
	}
}

func monitorExpectationMismatchReason(expect, reason string) string {
	switch expect {
	case "marker":
		return "expected_marker_got_" + reason
	case "terminal":
		return "expected_terminal_got_" + reason
	default:
		return reason
	}
}

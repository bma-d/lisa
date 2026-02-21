package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var computeSessionStatusFn = computeSessionStatus
var monitorWaitingTurnCompleteFn = monitorWaitingTurnComplete

type waitingTurnCompleteResult struct {
	Ready        bool
	InputAtNanos int64
	FileAge      int
}

func normalizeStatusForSessionStatusOutput(status sessionStatus) sessionStatus {
	normalized := status
	switch normalized.SessionState {
	case "completed", "crashed", "stuck", "not_found":
		normalized.Status = normalized.SessionState
	}
	return normalized
}

func normalizeMonitorFinalStatus(finalState, finalStatus string) string {
	switch finalState {
	case "completed", "crashed", "stuck", "not_found":
		return finalState
	default:
		return finalStatus
	}
}

func cmdSessionStatus(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	full := false
	failNotFound := false
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session status")
		case "--session":
			if i+1 >= len(args) {
				return flagValueError("--session")
			}
			session = args[i+1]
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return flagValueError("--project-root")
			}
			projectRoot = args[i+1]
			projectRootExplicit = true
			i++
		case "--agent":
			if i+1 >= len(args) {
				return flagValueError("--agent")
			}
			agentHint = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return flagValueError("--mode")
			}
			modeHint = args[i+1]
			i++
		case "--full":
			full = true
		case "--fail-not-found":
			failNotFound = true
		case "--json":
			jsonOut = true
		default:
			return unknownFlagError(args[i])
		}
	}

	if session == "" {
		fmt.Fprintln(os.Stderr, "--session is required")
		return 1
	}
	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	agentHint, err := parseAgentHint(agentHint)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	modeHint, err = parseModeHint(modeHint)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, full, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	status = normalizeStatusForSessionStatusOutput(status)

	if jsonOut {
		writeJSON(status)
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
	jsonOut := false
	verbose := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session monitor")
		case "--session":
			if i+1 >= len(args) {
				return flagValueError("--session")
			}
			session = args[i+1]
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return flagValueError("--project-root")
			}
			projectRoot = args[i+1]
			projectRootExplicit = true
			i++
		case "--agent":
			if i+1 >= len(args) {
				return flagValueError("--agent")
			}
			agentHint = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return flagValueError("--mode")
			}
			modeHint = args[i+1]
			i++
		case "--expect":
			if i+1 >= len(args) {
				return flagValueError("--expect")
			}
			parsedExpect, err := parseMonitorExpect(args[i+1])
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				return 1
			}
			expect = parsedExpect
			i++
		case "--poll-interval":
			if i+1 >= len(args) {
				return flagValueError("--poll-interval")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "invalid --poll-interval")
				return 1
			}
			pollInterval = n
			i++
		case "--max-polls":
			if i+1 >= len(args) {
				return flagValueError("--max-polls")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "invalid --max-polls")
				return 1
			}
			maxPolls = n
			i++
		case "--stop-on-waiting":
			if i+1 >= len(args) {
				return flagValueError("--stop-on-waiting")
			}
			parsed, err := parseBoolFlag(args[i+1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid --stop-on-waiting: %s (expected true|false)\n", args[i+1])
				return 1
			}
			stopOnWaiting = parsed
			i++
		case "--waiting-requires-turn-complete":
			if i+1 >= len(args) {
				return flagValueError("--waiting-requires-turn-complete")
			}
			parsed, err := parseBoolFlag(args[i+1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid --waiting-requires-turn-complete: %s (expected true|false)\n", args[i+1])
				return 1
			}
			waitingRequiresTurnComplete = parsed
			i++
		case "--until-marker":
			if i+1 >= len(args) {
				return flagValueError("--until-marker")
			}
			untilMarkerSet = true
			untilMarker = strings.TrimSpace(args[i+1])
			i++
		case "--json":
			jsonOut = true
		case "--verbose":
			verbose = true
		default:
			return unknownFlagError(args[i])
		}
	}

	if session == "" {
		fmt.Fprintln(os.Stderr, "--session is required")
		return 1
	}
	if untilMarkerSet && untilMarker == "" {
		fmt.Fprintln(os.Stderr, "invalid --until-marker: cannot be empty")
		return 1
	}
	if expect == "marker" && untilMarker == "" {
		fmt.Fprintln(os.Stderr, "--expect marker requires --until-marker")
		return 1
	}
	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	agentHint, err := parseAgentHint(agentHint)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	modeHint, err = parseModeHint(modeHint)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	last := sessionStatus{}
	degradedPolls := 0
	for poll := 1; poll <= maxPolls; poll++ {
		status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, true, poll)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
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
				ExitReason:  finalReason,
				Polls:       poll,
				FinalStatus: normalizeMonitorFinalStatus(status.SessionState, status.Status),
			}
			if jsonOut {
				writeJSON(result)
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
		ExitReason:  "max_polls_exceeded",
		Polls:       maxPolls,
		FinalStatus: normalizeMonitorFinalStatus("timeout", last.Status),
	}
	if degradedPolls == maxPolls && maxPolls > 0 {
		result.ExitReason = "degraded_max_polls_exceeded"
	}
	if jsonOut {
		writeJSON(result)
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
	jsonOut := false
	raw := false
	stripNoise := true
	projectRoot := getPWD()
	projectRootExplicit := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session capture")
		case "--session":
			if i+1 >= len(args) {
				return flagValueError("--session")
			}
			session = args[i+1]
			i++
		case "--lines":
			if i+1 >= len(args) {
				return flagValueError("--lines")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "invalid --lines")
				return 1
			}
			lines = n
			i++
		case "--raw":
			raw = true
		case "--keep-noise":
			stripNoise = false
		case "--strip-noise":
			stripNoise = true
		case "--project-root":
			if i+1 >= len(args) {
				return flagValueError("--project-root")
			}
			projectRoot = args[i+1]
			projectRootExplicit = true
			i++
		case "--json":
			jsonOut = true
		default:
			return unknownFlagError(args[i])
		}
	}
	if session == "" {
		fmt.Fprintln(os.Stderr, "--session is required")
		return 1
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

	if !raw && shouldUseTranscriptCapture(session, transcriptProjectRoot) {
		sessionID, messages, err := captureSessionTranscript(session, transcriptProjectRoot)
		if err == nil {
			return writeTranscriptCapture(session, sessionID, messages, jsonOut)
		}
	}

	if !tmuxHasSessionFn(session) {
		fmt.Fprintln(os.Stderr, "session not found")
		return 1
	}
	capture, err := tmuxCapturePaneFn(session, lines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to capture pane: %v\n", err)
		return 1
	}
	capture = strings.Join(trimLines(capture), "\n")
	if stripNoise {
		capture = filterCaptureNoise(capture)
	}
	if jsonOut {
		writeJSON(map[string]any{
			"session": session,
			"capture": capture,
		})
		return 0
	}
	fmt.Print(capture)
	return 0
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

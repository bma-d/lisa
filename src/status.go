package app

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ansiEscapeRe             = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	todoCheckboxRe           = regexp.MustCompile(`(?i)\[( |x)\]`)
	activeTaskKeywordRe      = regexp.MustCompile(`(?i)(working|running|checking|planning|writing|editing|creating|fixing|executing|reviewing)\b`)
	activeTaskCurrentRe      = regexp.MustCompile(`(?i)^current task[:\s]+(.+)$`)
	activeTaskActiveRe       = regexp.MustCompile(`(?i)^active task[:\s]+(.+)$`)
	promptBusyKeywordRe      = regexp.MustCompile(`(?i)\b(working|running|checking|planning|writing|editing|creating|fixing|executing|reviewing|loading|reading|searching|parsing|building|compiling|installing)\b`)
	codexPromptRe            = regexp.MustCompile(`❯\s*([0-9]+[smh]\s*)?[0-9]{1,2}:[0-9]{2}:[0-9]{2}\s*$`)
	shellPromptTrailerRe     = regexp.MustCompile(`[❯›]\s*(?:[0-9]+[smh]\s*)?[0-9]{1,2}:[0-9]{2}:[0-9]{2}\s*$`)
	waitReadLikeRe           = regexp.MustCompile(`loading|reading|searching|parsing`)
	waitBuildLikeRe          = regexp.MustCompile(`running tests|testing|building|compiling|installing`)
	waitWriteLikeRe          = regexp.MustCompile(`writing|editing|updating|creating|fixing`)
	sessionCompletionLineRe  = regexp.MustCompile(`^` + regexp.QuoteMeta(sessionDonePrefix) + `(?:([A-Za-z0-9._-]+):)?(-?\d+)\s*$`)
	execCompletionLineRe     = regexp.MustCompile(`^` + regexp.QuoteMeta(execDonePrefix) + `(-?\d+)\s*$`)
	activeTaskIgnorePrefixes = []string{"$", ">", "To get started", "Usage:", "Error:", "warning:", "note:"}
	activeTaskPatterns       = []*regexp.Regexp{activeTaskKeywordRe, activeTaskCurrentRe, activeTaskActiveRe}
)

type sessionCompletionMarker struct {
	Seen     bool
	RunID    string
	ExitCode int
}

func readPaneSnapshot(session string) (string, string, string, error) {
	combined, err := tmuxDisplayFn(session, "#{pane_dead}\t#{pane_dead_status}\t#{pane_current_command}\t#{pane_pid}")
	if err == nil {
		parts := strings.SplitN(combined, "\t", 4)
		if len(parts) == 4 {
			return formatPaneStatus(parts[0], parts[1]), strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3]), nil
		}
	}

	paneStatus, err := tmuxPaneStatusFn(session)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read tmux pane status: %w", err)
	}
	paneCommand, err := tmuxDisplayFn(session, "#{pane_current_command}")
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read tmux pane command: %w", err)
	}
	panePIDRaw, err := tmuxDisplayFn(session, "#{pane_pid}")
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read tmux pane pid: %w", err)
	}
	return paneStatus, paneCommand, strings.TrimSpace(panePIDRaw), nil
}

func formatPaneStatus(deadRaw, exitRaw string) string {
	dead := strings.TrimSpace(deadRaw)
	exit := strings.TrimSpace(exitRaw)
	if dead == "1" {
		if exit != "" {
			return "exited:" + exit
		}
		return "exited:0"
	}
	if exit != "" && exit != "0" {
		return "crashed:" + exit
	}
	return "alive"
}

func computeSessionStatus(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
	status := sessionStatus{
		Session:              session,
		Status:               "error",
		SessionState:         "error",
		WaitEstimate:         30,
		HeartbeatAge:         -1,
		HeartbeatFreshSecs:   getIntEnv("LISA_HEARTBEAT_STALE_SECONDS", defaultHeartbeatStaleSecs),
		ClassificationReason: "initializing",
	}

	if session == "" {
		status.ActiveTask = "no_session"
		status.ClassificationReason = "no_session"
		return status, nil
	}
	if !tmuxHasSessionFn(session) {
		status.Status = "not_found"
		status.SessionState = "not_found"
		status.WaitEstimate = 0
		status.ClassificationReason = "session_not_found"
		return status, nil
	}

	meta, metaErr := loadSessionMeta(projectRoot, session)
	metaReadable := metaErr == nil || errors.Is(metaErr, os.ErrNotExist)
	if metaErr != nil && !errors.Is(metaErr, os.ErrNotExist) {
		status.Signals.MetaReadError = metaErr.Error()
	}
	agent := resolveAgent(agentHint, meta, session)
	mode := resolveMode(modeHint, meta, session)
	status.Agent = agent
	status.Mode = mode
	if metaErr == nil {
		status.Signals.RunID = strings.TrimSpace(meta.RunID)
	}

	capture, err := tmuxCapturePaneFn(session, 220)
	if err != nil {
		return status, fmt.Errorf("failed to capture tmux pane: %w", err)
	}
	capture = filterInputBox(capture)
	capture = strings.Join(trimLines(capture), "\n")

	paneStatus, paneCommand, panePIDText, err := readPaneSnapshot(session)
	if err != nil {
		return status, err
	}
	status.PaneStatus = paneStatus
	status.PaneCommand = paneCommand

	statePath := sessionStateFile(projectRoot, session)
	stateHint, stateHintErr := loadSessionStateWithError(statePath)
	if stateHintErr != nil {
		status.Signals.StateReadError = stateHintErr.Error()
		stateHint = sessionState{}
	}

	todoDone, todoTotal := parseTodos(capture)
	status.TodosDone = todoDone
	status.TodosTotal = todoTotal
	status.ActiveTask = extractActiveTask(capture)
	status.WaitEstimate = estimateWait(status.ActiveTask, todoDone, todoTotal)

	execDone, execExitCode := parseExecCompletion(capture)
	sessionDone, sessionExitCode, markerRunID, markerRunMismatch := parseSessionCompletionForRun(capture, status.Signals.RunID)
	sessionMarkerSeen := markerRunID != "" || sessionDone || markerRunMismatch
	if !metaReadable {
		sessionDone = false
	}
	status.Signals.ExecMarkerSeen = execDone
	status.Signals.ExecExitCode = execExitCode
	status.Signals.SessionMarkerSeen = sessionMarkerSeen
	status.Signals.SessionMarkerRunID = markerRunID
	status.Signals.SessionMarkerRunMismatch = markerRunMismatch
	status.Signals.SessionExitCode = sessionExitCode

	panePID := 0
	if panePIDText != "" {
		panePID, err = strconv.Atoi(panePIDText)
		if err != nil {
			return status, fmt.Errorf("failed to parse tmux pane pid %q: %w", panePIDText, err)
		}
	}
	now := time.Now().Unix()
	processScanInterval := getIntEnv("LISA_PROCESS_SCAN_INTERVAL_SECONDS", defaultProcessScanInterval)
	if processScanInterval <= 0 {
		processScanInterval = defaultProcessScanInterval
	}
	useCachedProcessScan := stateHint.LastAgentProbeAt > 0 &&
		(now-stateHint.LastAgentProbeAt) < int64(processScanInterval)
	agentPID := stateHint.LastAgentPID
	agentCPU := stateHint.LastAgentCPU
	var agentScanErr error
	if !useCachedProcessScan {
		agentPID, agentCPU, agentScanErr = detectAgentProcessFn(panePID, agent)
	} else {
		status.Signals.AgentScanCached = true
	}
	status.AgentPID = agentPID
	status.AgentCPU = agentCPU
	status.Signals.AgentProcessDetected = agentPID > 0
	if agentScanErr != nil {
		status.Signals.AgentScanError = agentScanErr.Error()
	}

	hbAge, hbSeen := sessionHeartbeatAge(projectRoot, session, now)
	if hbSeen {
		status.HeartbeatAge = hbAge
	}
	heartbeatFresh := hbSeen && isAgeFresh(hbAge, status.HeartbeatFreshSecs)
	status.Signals.HeartbeatSeen = hbSeen
	status.Signals.HeartbeatFresh = heartbeatFresh

	paneIsShell := isShellCommand(paneCommand)
	status.Signals.PaneIsShell = paneIsShell

	lockMeta, err := withStateFileLockFn(statePath, func() error {
		state, stateErr := loadSessionStateWithError(statePath)
		if stateErr != nil {
			status.Signals.StateReadError = stateErr.Error()
			state = sessionState{}
		}
		state.LastAgentPID = agentPID
		state.LastAgentCPU = agentCPU
		if !useCachedProcessScan {
			state.LastAgentProbeAt = now
		}

		hash := md5Hex8(capture)
		if hash != "" && hash != state.LastOutputHash {
			state.LastOutputHash = hash
			state.LastOutputAt = now
		}
		if state.LastOutputAt == 0 {
			state.LastOutputAt = now
		}
		outputAge := int(now - state.LastOutputAt)
		if outputAge < 0 {
			outputAge = 0
		}
		staleAfter := getIntEnv("LISA_OUTPUT_STALE_SECONDS", defaultOutputStaleSeconds)
		outputFresh := isAgeFresh(outputAge, staleAfter)
		status.OutputAgeSeconds = outputAge
		status.OutputFreshSeconds = staleAfter
		status.Signals.OutputFresh = outputFresh

		interactiveWaiting := mode == "interactive" && agentPID > 0 && agentCPU < 0.2 && !outputFresh
		promptWaiting := mode == "interactive" && looksLikePromptWaiting(agent, capture)
		activeProcessBusy := agentPID > 0 && agentCPU >= 0.2
		status.Signals.InteractiveWaiting = interactiveWaiting
		status.Signals.PromptWaiting = promptWaiting
		status.Signals.ActiveProcessBusy = activeProcessBusy

		switch {
		case strings.HasPrefix(paneStatus, "crashed:"):
			status.Status = "idle"
			status.SessionState = "crashed"
			status.WaitEstimate = 0
			status.ClassificationReason = "pane_crashed"
		case strings.HasPrefix(paneStatus, "exited:"):
			exitCode := strings.TrimPrefix(paneStatus, "exited:")
			status.Status = "idle"
			status.WaitEstimate = 0
			if exitCode == "0" {
				status.SessionState = "completed"
				status.ClassificationReason = "pane_exited_zero"
			} else {
				status.SessionState = "crashed"
				status.ClassificationReason = "pane_exited_nonzero"
			}
		default:
			status.Status = "active"
			status.SessionState = "in_progress"

			switch {
			case sessionDone:
				status.Status = "idle"
				status.WaitEstimate = 0
				status.ClassificationReason = "session_done_marker"
				if sessionExitCode == 0 {
					status.SessionState = "completed"
				} else {
					status.SessionState = "crashed"
				}
			case execDone:
				status.Status = "idle"
				status.WaitEstimate = 0
				status.ClassificationReason = "exec_done_marker"
				if execExitCode == 0 {
					status.SessionState = "completed"
				} else {
					status.SessionState = "crashed"
				}
			case interactiveWaiting:
				status.Status = "idle"
				status.SessionState = "waiting_input"
				status.WaitEstimate = 0
				status.ClassificationReason = "interactive_waiting_idle"
			case promptWaiting && !activeProcessBusy:
				status.Status = "idle"
				status.SessionState = "waiting_input"
				status.WaitEstimate = 0
				status.ClassificationReason = "prompt_waiting"
			case agentPID > 0:
				status.Status = "active"
				status.SessionState = "in_progress"
				status.ClassificationReason = "agent_pid_alive"
			case outputFresh:
				status.Status = "active"
				status.SessionState = "in_progress"
				status.ClassificationReason = "output_fresh"
			case heartbeatFresh:
				status.Status = "active"
				status.SessionState = "in_progress"
				status.ClassificationReason = "heartbeat_fresh"
			case !paneIsShell:
				status.Status = "active"
				status.SessionState = "in_progress"
				status.ClassificationReason = "non_shell_command"
			case agentScanErr != nil:
				status.Status = "idle"
				status.SessionState = "degraded"
				status.WaitEstimate = 0
				status.ClassificationReason = "agent_scan_error"
			default:
				status.Status = "idle"
				status.WaitEstimate = 0
				if pollCount > 0 && pollCount <= 3 {
					status.SessionState = "just_started"
					status.ClassificationReason = "grace_period_just_started"
				} else {
					status.SessionState = "stuck"
					if markerRunMismatch {
						status.ClassificationReason = "stuck_marker_run_mismatch"
					} else {
						status.ClassificationReason = "stuck_no_signals"
					}
				}
			}
			if status.Status == "active" && status.ActiveTask == "" {
				status.ActiveTask = fmt.Sprintf("%s running", strings.Title(agent))
			}
		}

		if status.Status == "active" {
			state.HasEverBeenActive = true
		}
		if pollCount > 0 {
			state.PollCount = pollCount
		} else {
			state.PollCount++
		}
		currentPoll := state.PollCount

		eventType := "snapshot"
		if state.LastSessionState != status.SessionState ||
			state.LastStatus != status.Status ||
			state.LastClassificationReason != status.ClassificationReason {
			eventType = "transition"
		}
		if eventErr := appendSessionEventFn(projectRoot, session, sessionEvent{
			At:      time.Now().UTC().Format(time.RFC3339Nano),
			Type:    eventType,
			Session: session,
			State:   status.SessionState,
			Status:  status.Status,
			Reason:  status.ClassificationReason,
			Poll:    currentPoll,
			Signals: status.Signals,
		}); eventErr != nil {
			status.Signals.EventsWriteError = eventErr.Error()
		}

		state.LastSessionState = status.SessionState
		state.LastStatus = status.Status
		state.LastClassificationReason = status.ClassificationReason
		state.LastClassificationPollRef = currentPoll

		return saveSessionState(statePath, state)
	})
	status.Signals.StateLockWaitMS = lockMeta.WaitMS
	if err != nil {
		if waitMS, timedOut := stateLockTimeoutWaitMS(err); timedOut {
			status.Status = "idle"
			status.SessionState = "degraded"
			status.WaitEstimate = 0
			status.ClassificationReason = "state_lock_timeout"
			status.Signals.StateLockTimedOut = true
			status.Signals.StateLockWaitMS = waitMS
			if eventErr := appendSessionEventFn(projectRoot, session, sessionEvent{
				At:      time.Now().UTC().Format(time.RFC3339Nano),
				Type:    "transition",
				Session: session,
				State:   status.SessionState,
				Status:  status.Status,
				Reason:  status.ClassificationReason,
				Poll:    pollCount,
				Signals: status.Signals,
			}); eventErr != nil {
				status.Signals.EventsWriteError = eventErr.Error()
			}
			return status, nil
		}
		return status, fmt.Errorf("failed to update session state: %w", err)
	}

	if full && (status.SessionState == "completed" || status.SessionState == "crashed" || status.SessionState == "stuck" || status.SessionState == "degraded") {
		outputFile, err := writeSessionOutputFile(projectRoot, session)
		if err == nil {
			status.OutputFile = outputFile
			if status.ActiveTask == "" {
				status.ActiveTask = outputFile
			}
		}
	}

	return status, nil
}

func resolveAgent(agentHint string, meta sessionMeta, session string) string {
	agentHint = strings.ToLower(strings.TrimSpace(agentHint))
	if agentHint == "claude" || agentHint == "codex" {
		return agentHint
	}
	if v := strings.ToLower(strings.TrimSpace(meta.Agent)); v == "claude" || v == "codex" {
		return v
	}
	if envAgent, err := tmuxShowEnvironmentFn(session, "LISA_AGENT"); err == nil {
		envAgent = strings.ToLower(strings.TrimSpace(envAgent))
		if envAgent == "claude" || envAgent == "codex" {
			return envAgent
		}
	}
	if envAgent, err := tmuxShowEnvironmentFn(session, "AI_AGENT"); err == nil {
		envAgent = strings.ToLower(strings.TrimSpace(envAgent))
		if envAgent == "claude" || envAgent == "codex" {
			return envAgent
		}
	}
	return "claude"
}

func resolveMode(modeHint string, meta sessionMeta, session string) string {
	modeHint = strings.ToLower(strings.TrimSpace(modeHint))
	if modeHint == "interactive" || modeHint == "exec" {
		return modeHint
	}
	if v := strings.ToLower(strings.TrimSpace(meta.Mode)); v == "interactive" || v == "exec" {
		return v
	}
	if v, err := tmuxShowEnvironmentFn(session, "LISA_MODE"); err == nil {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "interactive" || v == "exec" {
			return v
		}
	}
	return "interactive"
}

func parseTodos(capture string) (int, int) {
	lines := trimLines(capture)
	done := 0
	total := 0
	for _, line := range lines {
		matches := todoCheckboxRe.FindAllString(line, -1)
		for _, m := range matches {
			total++
			if strings.EqualFold(m, "[x]") {
				done++
			}
		}
	}
	return done, total
}

func extractActiveTask(capture string) string {
	lines := trimLines(capture)
	if len(lines) == 0 {
		return ""
	}

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if containsAnyPrefix(line, activeTaskIgnorePrefixes) {
			continue
		}
		for _, p := range activeTaskPatterns {
			if p.MatchString(line) {
				if len(line) > 140 {
					return strings.TrimSpace(line[:140])
				}
				return line
			}
		}
	}
	return ""
}

func looksLikePromptWaiting(agent, capture string) bool {
	lines := trimLines(capture)
	if len(lines) == 0 {
		return false
	}

	tail := make([]string, 0, 8)
	for i := len(lines) - 1; i >= 0 && len(tail) < 8; i-- {
		line := strings.TrimSpace(stripANSIEscape(lines[i]))
		if line == "" {
			continue
		}
		tail = append(tail, line)
	}
	if len(tail) == 0 {
		return false
	}
	last := tail[0]
	lowerTail := strings.ToLower(strings.Join(tail, "\n"))

	if promptBusyKeywordRe.MatchString(last) {
		return false
	}

	if agent == "codex" {
		if codexPromptRe.MatchString(last) {
			return true
		}
		if strings.Contains(lowerTail, "tokens used") && strings.Contains(last, "❯") {
			return true
		}
	}

	if agent == "claude" {
		if strings.HasSuffix(last, "›") {
			return true
		}
		if strings.TrimSpace(last) == ">" {
			return true
		}
		if strings.Contains(lowerTail, "press enter to send") && (strings.TrimSpace(last) == ">" || strings.HasSuffix(last, "›")) {
			return true
		}
	}
	return false
}

func estimateWait(task string, done, total int) int {
	lower := strings.ToLower(task)
	switch {
	case waitReadLikeRe.MatchString(lower):
		return 30
	case waitBuildLikeRe.MatchString(lower):
		return 120
	case waitWriteLikeRe.MatchString(lower):
		return 60
	}
	if total > 0 {
		progress := 100 * done / total
		switch {
		case progress < 25:
			return 90
		case progress < 50:
			return 75
		case progress < 75:
			return 60
		default:
			return 30
		}
	}
	return 60
}

func parseExecCompletion(capture string) (bool, int) {
	return parseCompletionMarker(capture, execCompletionLineRe)
}

func parseSessionCompletion(capture string) (bool, int) {
	done, code, _, _ := parseSessionCompletionForRun(capture, "")
	return done, code
}

func parseSessionCompletionForRun(capture, runID string) (bool, int, string, bool) {
	marker := parseSessionCompletionMarker(capture)
	if !marker.Seen {
		return false, 0, "", false
	}
	runMismatch := runID != "" && marker.RunID != "" && marker.RunID != runID
	if runMismatch {
		return false, marker.ExitCode, marker.RunID, true
	}
	return true, marker.ExitCode, marker.RunID, false
}

func parseSessionCompletionMarker(capture string) sessionCompletionMarker {
	tail := nonEmptyTailLines(capture, 24)
	if len(tail) == 0 {
		return sessionCompletionMarker{}
	}

	markerIdx := -1
	markerRunID := ""
	markerCode := ""
	for i := len(tail) - 1; i >= 0; i-- {
		match := sessionCompletionLineRe.FindStringSubmatch(tail[i])
		if len(match) == 3 {
			markerIdx = i
			markerRunID = strings.TrimSpace(match[1])
			markerCode = match[2]
			break
		}
	}
	if markerIdx < 0 {
		return sessionCompletionMarker{}
	}
	for i := markerIdx + 1; i < len(tail); i++ {
		if !isLikelyShellPromptLine(tail[i]) {
			return sessionCompletionMarker{}
		}
	}
	code, err := strconv.Atoi(markerCode)
	if err != nil {
		code = 1
	}
	return sessionCompletionMarker{Seen: true, RunID: markerRunID, ExitCode: code}
}

func parseCompletionMarker(capture string, markerRe *regexp.Regexp) (bool, int) {
	tail := nonEmptyTailLines(capture, 24)
	if len(tail) == 0 {
		return false, 0
	}

	markerIdx := -1
	markerCode := ""
	for i := len(tail) - 1; i >= 0; i-- {
		match := markerRe.FindStringSubmatch(tail[i])
		if len(match) == 2 {
			markerIdx = i
			markerCode = match[1]
			break
		}
	}
	if markerIdx < 0 {
		return false, 0
	}
	for i := markerIdx + 1; i < len(tail); i++ {
		if !isLikelyShellPromptLine(tail[i]) {
			return false, 0
		}
	}

	code, err := strconv.Atoi(markerCode)
	if err != nil {
		return true, 1
	}
	return true, code
}

func nonEmptyTailLines(capture string, max int) []string {
	lines := trimLines(capture)
	tail := make([]string, 0, max)
	for i := len(lines) - 1; i >= 0 && len(tail) < max; i-- {
		line := strings.TrimSpace(stripANSIEscape(lines[i]))
		if line == "" {
			continue
		}
		tail = append([]string{line}, tail...)
	}
	return tail
}

func stripANSIEscape(line string) string {
	return ansiEscapeRe.ReplaceAllString(line, "")
}

func sessionHeartbeatAge(projectRoot, session string, now int64) (int, bool) {
	info, err := os.Stat(sessionHeartbeatFile(projectRoot, session))
	if err != nil {
		return 0, false
	}
	age := int(now - info.ModTime().Unix())
	if age < 0 {
		age = 0
	}
	return age, true
}

func isAgeFresh(age, staleAfter int) bool {
	if age < 0 {
		return false
	}
	return age <= staleAfter
}

func isLikelyShellPromptLine(line string) bool {
	line = strings.TrimSpace(stripANSIEscape(line))
	if line == "" {
		return true
	}
	// Starship and similar prompts often include duration/time segments after the prompt token.
	if shellPromptTrailerRe.MatchString(line) {
		return true
	}
	if strings.HasSuffix(line, "$") ||
		strings.HasSuffix(line, "#") ||
		strings.HasSuffix(line, "%") ||
		strings.HasSuffix(line, "❯") {
		return true
	}

	if strings.HasSuffix(line, ">") {
		if strings.Contains(line, "<") {
			return false
		}
		return strings.Contains(line, "/") ||
			strings.Contains(line, "~") ||
			strings.Contains(line, `\\`)
	}
	return false
}

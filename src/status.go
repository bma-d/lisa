package app

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
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
var nowFn = time.Now

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

func degradedSessionStatus(status sessionStatus, reason string, err error) sessionStatus {
	status.Status = "idle"
	status.SessionState = "degraded"
	status.WaitEstimate = 0
	status.ClassificationReason = reason
	if err != nil {
		status.Signals.TMUXReadError = err.Error()
	}
	return status
}

func applyTerminalPaneStatus(status sessionStatus, paneStatus string) (sessionStatus, bool) {
	switch {
	case strings.HasPrefix(paneStatus, "crashed:"):
		status.Status = "idle"
		status.SessionState = "crashed"
		status.WaitEstimate = 0
		status.ClassificationReason = "pane_crashed"
		return status, true
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
		return status, true
	default:
		return status, false
	}
}

func appendStatusEvent(projectRoot, session, eventType string, poll int, status sessionStatus) error {
	return appendSessionEventFn(projectRoot, session, sessionEvent{
		At:      nowFn().UTC().Format(time.RFC3339Nano),
		Type:    eventType,
		Session: session,
		State:   status.SessionState,
		Status:  status.Status,
		Reason:  status.ClassificationReason,
		Poll:    poll,
		Signals: status.Signals,
	})
}

func shouldAppendImmediateReadErrorEvent(pollCount int) bool {
	return pollCount > 0
}

func agentDisplayName(agent string) string {
	agent = strings.TrimSpace(strings.ToLower(agent))
	if agent == "" {
		return "Agent"
	}
	runes := []rune(agent)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
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
	statePath := sessionStateFile(projectRoot, session)
	stateHint, stateHintErr := loadSessionStateWithError(statePath)
	if stateHintErr != nil {
		status.Signals.StateReadError = stateHintErr.Error()
		stateHint = sessionState{}
	}

	agent := resolveAgent(agentHint, meta, session, strings.ToLower(strings.TrimSpace(stateHint.LastResolvedAgent)))
	mode := resolveMode(modeHint, meta, session, strings.ToLower(strings.TrimSpace(stateHint.LastResolvedMode)))
	status.Agent = agent
	status.Mode = mode
	if metaErr == nil {
		status.Signals.RunID = strings.TrimSpace(meta.RunID)
	}
	doneFileDone, doneFileExitCode, doneFileRunID, doneFileRunMismatch, doneFileErr := readSessionDoneFile(projectRoot, session, status.Signals.RunID)
	if doneFileErr != nil {
		status.Signals.DoneFileReadError = doneFileErr.Error()
	}
	status.Signals.DoneFileSeen = doneFileDone || doneFileRunMismatch || doneFileRunID != ""
	status.Signals.DoneFileRunID = doneFileRunID
	status.Signals.DoneFileRunMismatch = doneFileRunMismatch
	status.Signals.DoneFileExitCode = doneFileExitCode
	if !metaReadable {
		doneFileDone = false
	}

	paneStatus, paneCommand, panePIDText, err := readPaneSnapshot(session)
	if err != nil {
		status = degradedSessionStatus(status, "tmux_snapshot_error", err)
		if shouldAppendImmediateReadErrorEvent(pollCount) {
			if eventErr := appendStatusEvent(projectRoot, session, "transition", pollCount, status); eventErr != nil {
				status.Signals.EventsWriteError = eventErr.Error()
			}
		}
		return status, nil
	}
	status.PaneStatus = paneStatus
	status.PaneCommand = paneCommand

	capture, err := tmuxCapturePaneFn(session, 220)
	if err != nil {
		status.Signals.TMUXReadError = err.Error()
		if doneFileDone {
			status.Status = "idle"
			status.WaitEstimate = 0
			status.ClassificationReason = "done_file"
			if doneFileExitCode == 0 {
				status.SessionState = "completed"
			} else {
				status.SessionState = "crashed"
			}
		} else if terminalStatus, terminal := applyTerminalPaneStatus(status, paneStatus); terminal {
			status = terminalStatus
		} else {
			status = degradedSessionStatus(status, "tmux_capture_error", err)
		}
		if shouldAppendImmediateReadErrorEvent(pollCount) {
			if eventErr := appendStatusEvent(projectRoot, session, "transition", pollCount, status); eventErr != nil {
				status.Signals.EventsWriteError = eventErr.Error()
			}
		}
		return status, nil
	}
	capture = filterInputBox(capture)
	capture = strings.Join(trimLines(capture), "\n")
	captureObservedAtNanos := nowFn().UnixNano()

	todoDone, todoTotal := parseTodos(capture)
	status.TodosDone = todoDone
	status.TodosTotal = todoTotal
	status.ActiveTask = extractActiveTask(capture)
	status.WaitEstimate = estimateWait(status.ActiveTask, todoDone, todoTotal)

	execDone, execExitCode := parseExecCompletion(capture)
	sessionDoneMarker, sessionExitCode, markerRunID, markerRunMismatch := parseSessionCompletionForRun(capture, status.Signals.RunID)
	sessionDone := sessionDoneMarker || doneFileDone
	if doneFileDone {
		sessionExitCode = doneFileExitCode
	}
	sessionMarkerSeen := markerRunID != "" || sessionDoneMarker || markerRunMismatch
	if !metaReadable {
		sessionDone = false
		sessionDoneMarker = false
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
			status = degradedSessionStatus(status, "tmux_pane_pid_parse_error", fmt.Errorf("failed to parse tmux pane pid %q: %w", panePIDText, err))
			if shouldAppendImmediateReadErrorEvent(pollCount) {
				if eventErr := appendStatusEvent(projectRoot, session, "transition", pollCount, status); eventErr != nil {
					status.Signals.EventsWriteError = eventErr.Error()
				}
			}
			return status, nil
		}
	}
	now := nowFn().Unix()
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

	var pendingEvent sessionEvent
	pendingEventReady := false

	lockMeta, err := withStateFileLockFn(statePath, func() error {
		state, stateErr := loadSessionStateWithError(statePath)
		if stateErr != nil {
			status.Signals.StateReadError = stateErr.Error()
			state = sessionState{}
		}
		state.LastAgentPID = agentPID
		state.LastAgentCPU = agentCPU
		state.LastResolvedAgent = agent
		state.LastResolvedMode = mode
		if stateHint.ClaudeSessionID != "" {
			state.ClaudeSessionID = stateHint.ClaudeSessionID
		}
		if stateHint.CodexSessionID != "" {
			state.CodexSessionID = stateHint.CodexSessionID
		}
		if !useCachedProcessScan {
			state.LastAgentProbeAt = now
		}

		stateOutputAtNanos := state.LastOutputAtNanos
		if stateOutputAtNanos == 0 && state.LastOutputAt > 0 {
			stateOutputAtNanos = state.LastOutputAt * int64(time.Second)
		}

		hash := md5Hex8(capture)
		if hash != "" && hash != state.LastOutputHash {
			if captureObservedAtNanos > stateOutputAtNanos {
				state.LastOutputHash = hash
				state.LastOutputAtNanos = captureObservedAtNanos
				state.LastOutputAt = captureObservedAtNanos / int64(time.Second)
				stateOutputAtNanos = captureObservedAtNanos
			}
		}
		if stateOutputAtNanos == 0 {
			stateOutputAtNanos = captureObservedAtNanos
			state.LastOutputAtNanos = stateOutputAtNanos
			state.LastOutputAt = stateOutputAtNanos / int64(time.Second)
		}
		outputAge := int((nowFn().UnixNano() - stateOutputAtNanos) / int64(time.Second))
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

		var transcriptTurnComplete bool
		if mode == "interactive" && metaErr == nil {
			switch agent {
			case "claude":
				cached := stateHint.ClaudeSessionID
				tc, tAge, sid, tErr := checkTranscriptTurnCompleteFn(meta.ProjectRoot, meta.Prompt, meta.CreatedAt, cached)
				transcriptTurnComplete = tc
				status.Signals.TranscriptTurnComplete = tc
				status.Signals.TranscriptFileAge = tAge
				if tErr != nil {
					status.Signals.TranscriptError = tErr.Error()
				}
				if sid != "" && sid != stateHint.ClaudeSessionID {
					state.ClaudeSessionID = sid
				}
			case "codex":
				cached := stateHint.CodexSessionID
				tc, tAge, sid, tErr := checkCodexTranscriptTurnCompleteFn(meta.Prompt, meta.CreatedAt, cached)
				transcriptTurnComplete = tc
				status.Signals.TranscriptTurnComplete = tc
				status.Signals.TranscriptFileAge = tAge
				if tErr != nil {
					status.Signals.TranscriptError = tErr.Error()
				}
				if sid != "" && sid != stateHint.CodexSessionID {
					state.CodexSessionID = sid
				}
			}
		}

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
				if doneFileDone {
					status.ClassificationReason = "done_file"
				} else {
					status.ClassificationReason = "session_done_marker"
				}
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
			case transcriptTurnComplete:
				status.Status = "idle"
				status.SessionState = "waiting_input"
				status.WaitEstimate = 0
				status.ClassificationReason = "transcript_turn_complete"
			case promptWaiting:
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
			case status.Signals.DoneFileReadError != "":
				status.Status = "idle"
				status.SessionState = "degraded"
				status.WaitEstimate = 0
				status.ClassificationReason = "done_file_read_error"
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
				status.ActiveTask = fmt.Sprintf("%s running", agentDisplayName(agent))
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
		pendingEvent = sessionEvent{
			At:      nowFn().UTC().Format(time.RFC3339Nano),
			Type:    eventType,
			Session: session,
			State:   status.SessionState,
			Status:  status.Status,
			Reason:  status.ClassificationReason,
			Poll:    currentPoll,
			Signals: status.Signals,
		}
		pendingEventReady = true

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
			if eventErr := appendStatusEvent(projectRoot, session, "transition", pollCount, status); eventErr != nil {
				status.Signals.EventsWriteError = eventErr.Error()
			}
			return status, nil
		}
		return status, fmt.Errorf("failed to update session state: %w", err)
	}
	if pendingEventReady {
		// lock wait is known only after withStateFileLockFn returns; propagate to event payload.
		pendingEvent.Signals.StateLockWaitMS = status.Signals.StateLockWaitMS
		pendingEvent.Signals.StateLockTimedOut = status.Signals.StateLockTimedOut
		if eventErr := appendSessionEventFn(projectRoot, session, pendingEvent); eventErr != nil {
			status.Signals.EventsWriteError = eventErr.Error()
		}
	}

	if full && (status.SessionState == "completed" || status.SessionState == "crashed" || status.SessionState == "stuck" || status.SessionState == "degraded") {
		outputFile, err := writeSessionOutputFileFromCapture(projectRoot, session, capture)
		if err == nil {
			status.OutputFile = outputFile
			if status.ActiveTask == "" {
				status.ActiveTask = outputFile
			}
		}
	}

	return status, nil
}

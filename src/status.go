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
	ansiEscapeRe            = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	shellPromptTrailerRe    = regexp.MustCompile(`[❯›]\s*(?:[0-9]+[smh]\s*)?[0-9]{1,2}:[0-9]{2}:[0-9]{2}\s*$`)
	sessionCompletionLineRe = regexp.MustCompile(`^` + regexp.QuoteMeta(sessionDonePrefix) + `(?:([A-Za-z0-9._-]+):)?(-?\d+)\s*$`)
	execCompletionLineRe    = regexp.MustCompile(`^` + regexp.QuoteMeta(execDonePrefix) + `(-?\d+)\s*$`)
)

const agentCPUBusyThreshold = 0.2

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

func stateLastInputAtNanos(state sessionState) int64 {
	lastInputAtNanos := state.LastInputAtNanos
	if lastInputAtNanos <= 0 && state.LastInputAt > 0 {
		lastInputAtNanos = state.LastInputAt * int64(time.Second)
	}
	return lastInputAtNanos
}

func applyCachedTurnCompleteSignals(status *sessionStatus, state sessionState) {
	turnCompleteMarker := state.LastTurnCompleteInputNanos
	if turnCompleteMarker <= 0 {
		turnCompleteMarker = state.LastTurnCompleteAtNanos
	}
	if turnCompleteMarker <= 0 {
		return
	}
	lastInputAtNanos := stateLastInputAtNanos(state)
	if lastInputAtNanos > 0 && turnCompleteMarker < lastInputAtNanos {
		return
	}
	status.Signals.TranscriptTurnComplete = true
	status.Signals.TranscriptFileAge = state.LastTurnCompleteFileAge
}

func computeSessionStatus(session, projectRoot, agentHint, modeHint string, full bool, pollCount int) (sessionStatus, error) {
	status := sessionStatus{
		Session:              session,
		Status:               "error",
		SessionState:         "error",
		WaitEstimate:         0,
		HeartbeatAge:         -1,
		HeartbeatFreshSecs:   getIntEnv("LISA_HEARTBEAT_STALE_SECONDS", defaultHeartbeatStaleSecs),
		ClassificationReason: "initializing",
		OutputFreshSeconds:   getIntEnv("LISA_OUTPUT_STALE_SECONDS", defaultOutputStaleSeconds),
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
	effectivePollCount := pollCount
	if effectivePollCount <= 0 {
		effectivePollCount = stateHint.PollCount + 1
		if effectivePollCount <= 0 {
			effectivePollCount = 1
		}
	}
	applyCachedTurnCompleteSignals(&status, stateHint)

	agent := resolveAgent(agentHint, meta, session, strings.ToLower(strings.TrimSpace(stateHint.LastResolvedAgent)))
	mode := resolveMode(modeHint, meta, session, strings.ToLower(strings.TrimSpace(stateHint.LastResolvedMode)))
	sessionLower := strings.ToLower(strings.TrimSpace(session))
	interactiveModeKnown := mode == "interactive" && (strings.EqualFold(strings.TrimSpace(modeHint), "interactive") ||
		strings.EqualFold(strings.TrimSpace(meta.Mode), "interactive") ||
		strings.HasSuffix(sessionLower, "-interactive") ||
		strings.Contains(sessionLower, "-interactive-"))
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
	paneTreeReady := false
	paneHasNonShellDescendant := false
	paneCPU := 0.0
	if mode == "interactive" && paneIsShell && agentPID == 0 {
		cpu, hasNonShellDescendant, paneTreeErr := inspectPaneProcessTreeFn(panePID)
		if paneTreeErr == nil {
			paneCPU = cpu
			paneHasNonShellDescendant = hasNonShellDescendant
			paneTreeReady = true
		}
	}

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
			case doneFileDone:
				status.Status = "idle"
				status.WaitEstimate = 0
				status.ClassificationReason = "done_file"
				if doneFileExitCode == 0 {
					status.SessionState = "completed"
				} else {
					status.SessionState = "crashed"
				}
			case agentPID > 0:
				activeProcessBusy := agentCPU >= agentCPUBusyThreshold
				status.Signals.ActiveProcessBusy = activeProcessBusy
				if mode == "interactive" && !activeProcessBusy && effectivePollCount > 3 {
					status.Status = "idle"
					status.SessionState = "waiting_input"
					status.WaitEstimate = 0
					status.ClassificationReason = "interactive_idle_cpu"
					status.Signals.InteractiveWaiting = true
				} else {
					status.Status = "active"
					status.SessionState = "in_progress"
					status.ClassificationReason = "agent_pid_alive"
				}
			case interactiveModeKnown && paneIsShell && heartbeatFresh && effectivePollCount > 3 && paneTreeReady:
				activeProcessBusy := paneCPU >= agentCPUBusyThreshold
				status.Signals.ActiveProcessBusy = activeProcessBusy
				switch {
				case paneHasNonShellDescendant:
					status.Status = "active"
					status.SessionState = "in_progress"
					status.ClassificationReason = "interactive_child_process"
				case activeProcessBusy:
					status.Status = "active"
					status.SessionState = "in_progress"
					status.ClassificationReason = "interactive_shell_busy"
				default:
					status.Status = "idle"
					status.SessionState = "waiting_input"
					status.WaitEstimate = 0
					status.ClassificationReason = "interactive_shell_idle"
					status.Signals.InteractiveWaiting = true
				}
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
				if effectivePollCount > 0 && effectivePollCount <= 3 {
					status.SessionState = "just_started"
					status.ClassificationReason = "grace_period_just_started"
				} else {
					status.SessionState = "stuck"
					if doneFileRunMismatch {
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
		state.PollCount = effectivePollCount
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
		capture, captureErr := tmuxCapturePaneFn(session, 220)
		if captureErr != nil {
			if status.Signals.TMUXReadError == "" {
				status.Signals.TMUXReadError = captureErr.Error()
			}
			return status, nil
		}
		capture = filterInputBox(capture)
		capture = strings.Join(trimLines(capture), "\n")
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

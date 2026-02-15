package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

var computeSessionStatusFn = computeSessionStatus

func cmdSessionStatus(args []string) int {
	session := ""
	projectRoot := getPWD()
	agentHint := "auto"
	modeHint := "auto"
	full := false
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
	projectRoot = canonicalProjectRoot(projectRoot)
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

	if jsonOut {
		writeJSON(status)
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
	return 0
}

func cmdSessionMonitor(args []string) int {
	session := ""
	projectRoot := getPWD()
	agentHint := "auto"
	modeHint := "auto"
	pollInterval := defaultPollIntervalSeconds
	maxPolls := defaultMaxPolls
	stopOnWaiting := true
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
	projectRoot = canonicalProjectRoot(projectRoot)
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
			fmt.Fprintf(os.Stderr, "[%s] poll=%d state=%s status=%s active=%q wait=%ds\n",
				time.Now().Format("15:04:05"), poll, status.SessionState, status.Status, status.ActiveTask, status.WaitEstimate)
		}

		reason := ""
		switch status.SessionState {
		case "completed":
			reason = "completed"
		case "crashed":
			reason = "crashed"
		case "not_found":
			reason = "not_found"
		case "stuck":
			reason = "stuck"
		case "waiting_input":
			if stopOnWaiting {
				reason = "waiting_input"
			}
		}
		if reason != "" {
			result := monitorResult{
				FinalState:  status.SessionState,
				Session:     status.Session,
				TodosDone:   status.TodosDone,
				TodosTotal:  status.TodosTotal,
				OutputFile:  status.OutputFile,
				ExitReason:  reason,
				Polls:       poll,
				FinalStatus: status.Status,
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
			if err := appendLifecycleEvent(projectRoot, session, "lifecycle", result.FinalState, result.FinalStatus, "monitor_"+reason); err != nil {
				fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
			}
			if reason == "completed" || reason == "waiting_input" {
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
		FinalStatus: last.Status,
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

func cmdSessionCapture(args []string) int {
	session := ""
	lines := 200
	jsonOut := false
	raw := false
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

	projectRoot = canonicalProjectRoot(projectRoot)
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

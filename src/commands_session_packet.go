package app

import (
	"fmt"
	"os"
	"strings"
)

func cmdSessionPacket(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	lines := 120
	events := 8
	tokenBudget := 320
	summaryStyle := "ops"
	cursorFile := ""
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session packet")
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
		case "--summary-style":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --summary-style")
			}
			summaryStyle = args[i+1]
			i++
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

	if session == "" {
		return commandError(jsonOut, "missing_required_flag", "--session is required")
	}
	var err error
	summaryStyle, err = parseCaptureSummaryStyle(summaryStyle)
	if err != nil {
		return commandError(jsonOut, "invalid_summary_style", err.Error())
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

	captureText := ""
	if tmuxHasSessionFn(session) {
		captureText, err = tmuxCapturePaneFn(session, lines)
		if err != nil {
			return commandErrorf(jsonOut, "capture_failed", "failed to capture pane: %v", err)
		}
		captureText = strings.Join(trimLines(filterCaptureNoise(captureText)), "\n")
	} else if strings.TrimSpace(status.OutputFile) != "" {
		if raw, readErr := os.ReadFile(status.OutputFile); readErr == nil {
			captureText = strings.Join(trimLines(filterCaptureNoise(string(raw))), "\n")
		}
	}
	if strings.TrimSpace(captureText) == "" {
		captureText = "(no live capture)"
	}
	summaryText, truncated := summarizeCaptureTextByStyle(session, projectRoot, captureText, tokenBudget, summaryStyle)

	items := make([]sessionHandoffItem, 0)
	droppedRecent := 0
	deltaFrom := -1
	nextDeltaOffset := -1
	if cursorFile != "" {
		cursorFile, err = expandAndCleanPath(cursorFile)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", err)
		}
		deltaFrom, err = loadCursorOffset(cursorFile)
		if err != nil {
			return commandErrorf(jsonOut, "invalid_cursor_file", "invalid --cursor-file: %v", err)
		}
		items, droppedRecent, nextDeltaOffset, err = readSessionHandoffDelta(projectRoot, session, deltaFrom, events)
		if err != nil {
			return commandErrorf(jsonOut, "handoff_delta_read_failed", "failed to read handoff delta: %v", err)
		}
		if err := writeCursorOffset(cursorFile, nextDeltaOffset); err != nil {
			return commandErrorf(jsonOut, "cursor_file_write_failed", "failed writing --cursor-file: %v", err)
		}
	} else {
		tail, _ := readSessionEventTailFn(projectRoot, session, events)
		droppedRecent = tail.DroppedLines
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
	nextAction := nextActionForState(status.SessionState)
	nextOffset := computeSessionCaptureNextOffset(session)

	if jsonOut {
		payload := map[string]any{
			"session":      session,
			"status":       status.Status,
			"sessionState": status.SessionState,
			"reason":       status.ClassificationReason,
			"nextAction":   nextAction,
			"nextOffset":   nextOffset,
			"summary":      summaryText,
			"summaryStyle": summaryStyle,
			"tokenBudget":  tokenBudget,
			"truncated":    truncated,
		}
		if !jsonMin {
			payload["projectRoot"] = projectRoot
			payload["capture"] = map[string]any{
				"lines": lines,
			}
			payload["handoff"] = map[string]any{
				"events":        items,
				"droppedRecent": droppedRecent,
			}
		} else {
			payload["recent"] = items
		}
		if cursorFile != "" {
			payload["deltaFrom"] = deltaFrom
			payload["nextDeltaOffset"] = nextDeltaOffset
			payload["deltaCount"] = len(items)
			if !jsonMin {
				payload["cursorFile"] = cursorFile
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

	fmt.Printf("state=%s status=%s reason=%s next=%s\n", status.SessionState, status.Status, status.ClassificationReason, nextAction)
	fmt.Println(summaryText)
	return boolExit(status.SessionState != "not_found")
}

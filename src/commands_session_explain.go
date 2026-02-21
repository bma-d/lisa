package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

func cmdSessionExplain(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	agentHint := "auto"
	modeHint := "auto"
	eventLimit := 10
	jsonOut := hasJSONFlag(args)
	jsonMin := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session explain")
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
		case "--events":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --events")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return commandError(jsonOut, "invalid_events", "invalid --events")
			}
			eventLimit = n
			i++
		case "--recent":
			if i+1 >= len(args) {
				return commandErrorf(jsonOut, "missing_flag_value", "missing value for --recent")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return commandError(jsonOut, "invalid_recent", "invalid --recent")
			}
			eventLimit = n
			i++
		case "--json":
			jsonOut = true
		case "--json-min":
			jsonOut = true
			jsonMin = true
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

	status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, true, 0)
	if err != nil {
		return commandError(jsonOut, "status_compute_failed", err.Error())
	}
	status = normalizeStatusForSessionStatusOutput(status)

	eventTail, err := readSessionEventTailFn(projectRoot, session, eventLimit)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return commandErrorf(jsonOut, "event_tail_read_failed", "failed reading session events: %v", err)
	}
	events := eventTail.Events

	if jsonOut {
		if jsonMin {
			recent := make([]map[string]any, 0, len(events))
			for _, event := range events {
				recent = append(recent, map[string]any{
					"at":     event.At,
					"type":   event.Type,
					"state":  event.State,
					"status": event.Status,
					"reason": event.Reason,
				})
			}
			payload := map[string]any{
				"session":      status.Session,
				"status":       status.Status,
				"sessionState": status.SessionState,
				"reason":       status.ClassificationReason,
				"recent":       recent,
			}
			if status.SessionState == "not_found" {
				payload["errorCode"] = "session_not_found"
			}
			writeJSON(payload)
			return 0
		}
		payload := map[string]any{
			"status":            status,
			"eventFile":         sessionEventsFile(projectRoot, session),
			"events":            events,
			"droppedEventLines": eventTail.DroppedLines,
		}
		if status.SessionState == "not_found" {
			payload["errorCode"] = "session_not_found"
		}
		writeJSON(payload)
		return 0
	}

	fmt.Printf("session: %s\n", status.Session)
	fmt.Printf("state: %s (%s)\n", status.SessionState, status.Status)
	fmt.Printf("reason: %s\n", status.ClassificationReason)
	fmt.Printf("agent: %s mode: %s\n", status.Agent, status.Mode)
	fmt.Printf("output_age: %ds (fresh<=%ds)\n", status.OutputAgeSeconds, status.OutputFreshSeconds)
	fmt.Printf("heartbeat_age: %ds (fresh<=%ds)\n", status.HeartbeatAge, status.HeartbeatFreshSecs)
	fmt.Printf("signals: done_file=%t session_marker=%t exec_marker=%t prompt_waiting=%t heartbeat_fresh=%t agent_pid=%d\n",
		status.Signals.DoneFileSeen,
		status.Signals.SessionMarkerSeen,
		status.Signals.ExecMarkerSeen,
		status.Signals.PromptWaiting,
		status.Signals.HeartbeatFresh,
		status.AgentPID,
	)
	if status.Signals.DoneFileReadError != "" {
		fmt.Printf("done_file_read_error: %s\n", status.Signals.DoneFileReadError)
	}
	if status.Signals.MetaReadError != "" {
		fmt.Printf("meta_read_error: %s\n", status.Signals.MetaReadError)
	}
	if status.Signals.StateReadError != "" {
		fmt.Printf("state_read_error: %s\n", status.Signals.StateReadError)
	}
	if status.Signals.EventsWriteError != "" {
		fmt.Printf("events_write_error: %s\n", status.Signals.EventsWriteError)
	}
	if status.Signals.TMUXReadError != "" {
		fmt.Printf("tmux_read_error: %s\n", status.Signals.TMUXReadError)
	}
	if len(events) == 0 {
		fmt.Println("events: none")
		if eventTail.DroppedLines > 0 {
			fmt.Printf("events_dropped: %d\n", eventTail.DroppedLines)
		}
		return 0
	}
	fmt.Println("events:")
	for _, event := range events {
		fmt.Printf("- %s %s state=%s status=%s reason=%s\n",
			event.At,
			event.Type,
			event.State,
			event.Status,
			event.Reason,
		)
	}
	if eventTail.DroppedLines > 0 {
		fmt.Printf("events_dropped: %d\n", eventTail.DroppedLines)
	}
	return 0
}

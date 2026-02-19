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
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session explain")
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
		case "--events":
			if i+1 >= len(args) {
				return flagValueError("--events")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "invalid --events")
				return 1
			}
			eventLimit = n
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

	status, err := computeSessionStatusFn(session, projectRoot, agentHint, modeHint, true, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	eventTail, err := readSessionEventTailFn(projectRoot, session, eventLimit)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "failed reading session events: %v\n", err)
		return 1
	}
	events := eventTail.Events

	if jsonOut {
		writeJSON(map[string]any{
			"status":            status,
			"eventFile":         sessionEventsFile(projectRoot, session),
			"events":            events,
			"droppedEventLines": eventTail.DroppedLines,
		})
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

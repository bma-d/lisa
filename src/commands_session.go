package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func cmdSession(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lisa session <subcommand>")
		return 1
	}

	switch args[0] {
	case "name":
		return cmdSessionName(args[1:])
	case "spawn":
		return cmdSessionSpawn(args[1:])
	case "send":
		return cmdSessionSend(args[1:])
	case "status":
		return cmdSessionStatus(args[1:])
	case "monitor":
		return cmdSessionMonitor(args[1:])
	case "capture":
		return cmdSessionCapture(args[1:])
	case "list":
		return cmdSessionList(args[1:])
	case "exists":
		return cmdSessionExists(args[1:])
	case "kill":
		return cmdSessionKill(args[1:])
	case "kill-all":
		return cmdSessionKillAll(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown session subcommand: %s\n", args[0])
		return 1
	}
}

func cmdSessionName(args []string) int {
	agent := "claude"
	mode := "interactive"
	projectRoot := getPWD()
	tag := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 >= len(args) {
				return flagValueError("--agent")
			}
			agent = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return flagValueError("--mode")
			}
			mode = args[i+1]
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return flagValueError("--project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--tag":
			if i+1 >= len(args) {
				return flagValueError("--tag")
			}
			tag = args[i+1]
			i++
		default:
			return unknownFlagError(args[i])
		}
	}

	name := generateSessionName(projectRoot, agent, mode, tag)
	fmt.Println(name)
	return 0
}

func cmdSessionSpawn(args []string) int {
	agent := "claude"
	mode := "interactive"
	projectRoot := getPWD()
	session := ""
	prompt := ""
	command := ""
	agentArgs := ""
	width := defaultTmuxWidth
	height := defaultTmuxHeight
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 >= len(args) {
				return flagValueError("--agent")
			}
			agent = args[i+1]
			i++
		case "--mode":
			if i+1 >= len(args) {
				return flagValueError("--mode")
			}
			mode = args[i+1]
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return flagValueError("--project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--session":
			if i+1 >= len(args) {
				return flagValueError("--session")
			}
			session = args[i+1]
			i++
		case "--prompt":
			if i+1 >= len(args) {
				return flagValueError("--prompt")
			}
			prompt = args[i+1]
			i++
		case "--command":
			if i+1 >= len(args) {
				return flagValueError("--command")
			}
			command = args[i+1]
			i++
		case "--agent-args":
			if i+1 >= len(args) {
				return flagValueError("--agent-args")
			}
			agentArgs = args[i+1]
			i++
		case "--width":
			if i+1 >= len(args) {
				return flagValueError("--width")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "invalid --width")
				return 1
			}
			width = n
			i++
		case "--height":
			if i+1 >= len(args) {
				return flagValueError("--height")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "invalid --height")
				return 1
			}
			height = n
			i++
		case "--json":
			jsonOut = true
		default:
			return unknownFlagError(args[i])
		}
	}

	agent = normalizeAgent(agent)
	mode = normalizeMode(mode)
	if session == "" {
		session = generateSessionName(projectRoot, agent, mode, "")
	}
	if tmuxHasSession(session) {
		fmt.Fprintf(os.Stderr, "session already exists: %s\n", session)
		return 1
	}

	if command == "" {
		var err error
		command, err = buildAgentCommand(agent, mode, prompt, agentArgs)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
	}

	if err := tmuxNewSession(session, projectRoot, agent, mode, width, height); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create tmux session: %v\n", err)
		return 1
	}

	commandToSend := command
	if mode == "exec" && command != "" {
		commandToSend = wrapExecCommand(command)
	}

	if commandToSend != "" {
		if err := tmuxSendCommandWithFallback(session, commandToSend, true); err != nil {
			_ = tmuxKillSession(session)
			fmt.Fprintf(os.Stderr, "failed to send startup command: %v\n", err)
			return 1
		}
	}

	meta := sessionMeta{
		Session:     session,
		Agent:       agent,
		Mode:        mode,
		ProjectRoot: projectRoot,
		StartCmd:    command,
		Prompt:      prompt,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := saveSessionMeta(projectRoot, session, meta); err != nil {
		fmt.Fprintf(os.Stderr, "failed to persist metadata: %v\n", err)
	}
	_ = os.Remove(sessionStateFile(projectRoot, session))

	if jsonOut {
		writeJSON(map[string]any{
			"session":     session,
			"agent":       agent,
			"mode":        mode,
			"projectRoot": projectRoot,
			"command":     command,
		})
		return 0
	}
	fmt.Println(session)
	return 0
}

func cmdSessionSend(args []string) int {
	session := ""
	text := ""
	keys := ""
	enter := false
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 >= len(args) {
				return flagValueError("--session")
			}
			session = args[i+1]
			i++
		case "--text":
			if i+1 >= len(args) {
				return flagValueError("--text")
			}
			text = args[i+1]
			i++
		case "--keys":
			if i+1 >= len(args) {
				return flagValueError("--keys")
			}
			keys = args[i+1]
			i++
		case "--enter":
			enter = true
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
	if !tmuxHasSession(session) {
		fmt.Fprintln(os.Stderr, "session not found")
		return 1
	}
	if text == "" && keys == "" {
		fmt.Fprintln(os.Stderr, "provide --text or --keys")
		return 1
	}
	if text != "" && keys != "" {
		fmt.Fprintln(os.Stderr, "use either --text or --keys, not both")
		return 1
	}

	if text != "" {
		if err := tmuxSendText(session, text, enter); err != nil {
			fmt.Fprintf(os.Stderr, "failed sending text: %v\n", err)
			return 1
		}
	} else {
		keyList := strings.Fields(keys)
		if len(keyList) == 0 {
			fmt.Fprintln(os.Stderr, "empty --keys")
			return 1
		}
		if err := tmuxSendKeys(session, keyList, enter); err != nil {
			fmt.Fprintf(os.Stderr, "failed sending keys: %v\n", err)
			return 1
		}
	}

	if jsonOut {
		writeJSON(map[string]any{
			"session": session,
			"ok":      true,
			"enter":   enter,
		})
		return 0
	}
	fmt.Println("ok")
	return 0
}

func cmdSessionStatus(args []string) int {
	session := ""
	projectRoot := getPWD()
	agentHint := "auto"
	modeHint := "auto"
	full := false
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
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

	status, err := computeSessionStatus(session, projectRoot, agentHint, modeHint, full, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	if jsonOut {
		writeJSON(status)
		return 0
	}

	fmt.Printf("%s,%d,%d,%s,%d,%s\n", status.Status, status.TodosDone, status.TodosTotal, status.ActiveTask, status.WaitEstimate, status.SessionState)
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
			val := strings.ToLower(args[i+1])
			stopOnWaiting = val != "false" && val != "0" && val != "no"
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

	last := sessionStatus{}
	for poll := 1; poll <= maxPolls; poll++ {
		status, err := computeSessionStatus(session, projectRoot, agentHint, modeHint, true, poll)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		last = status

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
				fmt.Printf("%s,%d,%d,%s,%s,%d,%s\n", result.FinalState, result.TodosDone, result.TodosTotal, result.OutputFile, result.ExitReason, result.Polls, result.FinalStatus)
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
	if jsonOut {
		writeJSON(result)
	} else {
		fmt.Printf("%s,%d,%d,%s,%s,%d,%s\n", result.FinalState, result.TodosDone, result.TodosTotal, result.OutputFile, result.ExitReason, result.Polls, result.FinalStatus)
	}
	return 2
}

func cmdSessionCapture(args []string) int {
	session := ""
	lines := 200
	jsonOut := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
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
	if !tmuxHasSession(session) {
		fmt.Fprintln(os.Stderr, "session not found")
		return 1
	}
	capture, err := tmuxCapturePane(session, lines)
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

func cmdSessionList(args []string) int {
	projectOnly := false
	projectRoot := getPWD()

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project-only":
			projectOnly = true
		case "--project-root":
			if i+1 >= len(args) {
				return flagValueError("--project-root")
			}
			projectRoot = args[i+1]
			i++
		default:
			return unknownFlagError(args[i])
		}
	}

	list, err := tmuxListSessions(projectOnly, projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list sessions: %v\n", err)
		return 1
	}
	fmt.Println(strings.Join(list, "\n"))
	return 0
}

func cmdSessionExists(args []string) int {
	session := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 >= len(args) {
				return flagValueError("--session")
			}
			session = args[i+1]
			i++
		default:
			return unknownFlagError(args[i])
		}
	}
	if session == "" {
		fmt.Fprintln(os.Stderr, "--session is required")
		return 1
	}
	if tmuxHasSession(session) {
		fmt.Println("true")
		return 0
	}
	fmt.Println("false")
	return 1
}

func cmdSessionKill(args []string) int {
	session := ""
	projectRoot := getPWD()
	for i := 0; i < len(args); i++ {
		switch args[i] {
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
		default:
			return unknownFlagError(args[i])
		}
	}
	if session == "" {
		fmt.Fprintln(os.Stderr, "--session is required")
		return 1
	}
	if err := cleanupSessionArtifacts(projectRoot, session); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", err)
	}
	_ = tmuxKillSession(session)
	fmt.Println("ok")
	return 0
}

func cmdSessionKillAll(args []string) int {
	projectOnly := false
	projectRoot := getPWD()
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project-only":
			projectOnly = true
		case "--project-root":
			if i+1 >= len(args) {
				return flagValueError("--project-root")
			}
			projectRoot = args[i+1]
			i++
		default:
			return unknownFlagError(args[i])
		}
	}

	sessions, err := tmuxListSessions(projectOnly, projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list sessions: %v\n", err)
		return 1
	}
	for _, s := range sessions {
		_ = cleanupSessionArtifacts(projectRoot, s)
		_ = tmuxKillSession(s)
	}
	fmt.Printf("killed %d sessions\n", len(sessions))
	return 0
}

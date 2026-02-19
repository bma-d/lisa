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

	if args[0] == "--help" || args[0] == "-h" {
		return showHelp("session")
	}
	if args[0] == "help" {
		if len(args) > 1 {
			return showHelp("session " + args[1])
		}
		return showHelp("session")
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
	case "explain":
		return cmdSessionExplain(args[1:])
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
		case "--help", "-h":
			return showHelp("session name")
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

	var err error
	agent, err = parseAgent(agent)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	mode, err = parseMode(mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	projectRoot = canonicalProjectRoot(projectRoot)

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
	cleanupAllHashes := false
	skipPermissions := true
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session spawn")
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
		case "--cleanup-all-hashes":
			cleanupAllHashes = true
		case "--no-dangerously-skip-permissions":
			skipPermissions = false
		case "--json":
			jsonOut = true
		default:
			return unknownFlagError(args[i])
		}
	}

	var err error
	agent, err = parseAgent(agent)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	mode, err = parseMode(mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	projectRoot = canonicalProjectRoot(projectRoot)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	session = strings.TrimSpace(session)
	if session != "" && !strings.HasPrefix(session, "lisa-") {
		fmt.Fprintln(os.Stderr, `invalid --session: must start with "lisa-"`)
		return 1
	}

	if session == "" {
		session = generateSessionName(projectRoot, agent, mode, "")
	}
	if tmuxHasSessionFn(session) {
		fmt.Fprintf(os.Stderr, "session already exists: %s\n", session)
		return 1
	}
	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	emitSpawnFailureEvent := func(reason string) {
		if err := appendLifecycleEvent(projectRoot, session, "lifecycle", "degraded", "idle", reason); err != nil {
			fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
		}
	}
	if err := pruneStaleSessionEventArtifactsFn(); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}

	cleanupOpts := cleanupOptions{AllHashes: cleanupAllHashes}
	if err := cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOpts); err != nil {
		fmt.Fprintf(os.Stderr, "failed to reset previous session artifacts: %v\n", err)
		emitSpawnFailureEvent("spawn_cleanup_error")
		return 1
	}
	if err := ensureHeartbeatWritableFn(sessionHeartbeatFile(projectRoot, session)); err != nil {
		fmt.Fprintf(os.Stderr, "failed to prepare heartbeat file: %v\n", err)
		emitSpawnFailureEvent("spawn_heartbeat_prepare_error")
		return 1
	}

	if command == "" {
		command, err = buildAgentCommandWithOptions(agent, mode, prompt, agentArgs, skipPermissions)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			emitSpawnFailureEvent("spawn_command_build_error")
			return 1
		}
	}
	commandToSend := command
	if mode == "exec" && command != "" {
		commandToSend = wrapExecCommand(command)
	}
	if strings.TrimSpace(commandToSend) != "" {
		commandToSend = wrapSessionCommand(commandToSend, runID)
	}

	if err := tmuxNewSessionWithStartupFn(session, projectRoot, agent, mode, width, height, commandToSend); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create tmux session: %v\n", err)
		if shouldPrintCodexExecNestedTmuxHint(agent, mode, err) {
			fmt.Fprintln(os.Stderr, "hint: codex exec --full-auto sandbox blocks tmux sockets for nested lisa runs; use --mode interactive (then session send) or pass --agent-args '--dangerously-bypass-approvals-and-sandbox'")
		}
		if cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOpts); cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", cleanupErr)
		}
		emitSpawnFailureEvent("spawn_tmux_new_error")
		return 1
	}

	meta := sessionMeta{
		Session:     session,
		Agent:       agent,
		Mode:        mode,
		RunID:       runID,
		ProjectRoot: projectRoot,
		StartCmd:    command,
		Prompt:      prompt,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := saveSessionMetaFn(projectRoot, session, meta); err != nil {
		killErr := tmuxKillSessionFn(session)
		cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOpts)
		fmt.Fprintf(os.Stderr, "failed to persist metadata: %v\n", err)
		if killErr != nil {
			fmt.Fprintf(os.Stderr, "failed to kill session after metadata error: %v\n", killErr)
		}
		if cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", cleanupErr)
		}
		emitSpawnFailureEvent("spawn_meta_persist_error")
		return 1
	}
	_ = os.Remove(sessionStateFile(projectRoot, session))
	if err := appendLifecycleEvent(projectRoot, session, "lifecycle", "spawned", "active", "spawn_success"); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}

	if jsonOut {
		writeJSON(map[string]any{
			"session":     session,
			"agent":       agent,
			"mode":        mode,
			"runId":       runID,
			"projectRoot": projectRoot,
			"command":     command,
		})
		return 0
	}
	fmt.Println(session)
	return 0
}

func shouldPrintCodexExecNestedTmuxHint(agent, mode string, err error) bool {
	if err == nil || agent != "codex" || mode != "exec" {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "error creating /tmp/") ||
		strings.Contains(msg, "error connecting to /private/tmp/")
}

func cmdSessionSend(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	text := ""
	keys := ""
	enter := false
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session send")
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
	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	if !tmuxHasSessionFn(session) {
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
		if err := tmuxSendTextFn(session, text, enter); err != nil {
			fmt.Fprintf(os.Stderr, "failed sending text: %v\n", err)
			return 1
		}
		if err := appendLifecycleEvent(projectRoot, session, "lifecycle", "in_progress", "active", "send_text"); err != nil {
			fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
		}
	} else {
		keyList := strings.Fields(keys)
		if len(keyList) == 0 {
			fmt.Fprintln(os.Stderr, "empty --keys")
			return 1
		}
		if err := tmuxSendKeysFn(session, keyList, enter); err != nil {
			fmt.Fprintf(os.Stderr, "failed sending keys: %v\n", err)
			return 1
		}
		if err := appendLifecycleEvent(projectRoot, session, "lifecycle", "in_progress", "active", "send_keys"); err != nil {
			fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
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

package app

import (
	"fmt"
	"os"
	"strings"
)

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

	projectRoot = canonicalProjectRoot(projectRoot)
	list, err := tmuxListSessionsFn(projectOnly, projectRoot)
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
	if tmuxHasSessionFn(session) {
		fmt.Println("true")
		return 0
	}
	fmt.Println("false")
	return 1
}

func cmdSessionKill(args []string) int {
	session := ""
	projectRoot := getPWD()
	cleanupAllHashes := false
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
		case "--cleanup-all-hashes":
			cleanupAllHashes = true
		default:
			return unknownFlagError(args[i])
		}
	}
	if session == "" {
		fmt.Fprintln(os.Stderr, "--session is required")
		return 1
	}
	projectRoot = canonicalProjectRoot(projectRoot)
	cleanupOpts := cleanupOptions{
		AllHashes:  cleanupAllHashes,
		KeepEvents: true,
	}
	if !tmuxHasSessionFn(session) {
		fmt.Fprintln(os.Stderr, "session not found")
		if err := cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOpts); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", err)
		}
		if err := appendLifecycleEvent(projectRoot, session, "lifecycle", "not_found", "idle", "kill_not_found"); err != nil {
			fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
		}
		return 1
	}
	killErr := tmuxKillSessionFn(session)
	cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, session, cleanupOpts)
	eventState := "terminated"
	eventReason := "kill_success"
	if killErr != nil {
		eventState = "degraded"
		eventReason = "kill_error"
	}
	if err := appendLifecycleEvent(projectRoot, session, "lifecycle", eventState, "idle", eventReason); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}
	if err := pruneStaleSessionEventArtifactsFn(); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}
	if killErr != nil || cleanupErr != nil {
		if killErr != nil {
			fmt.Fprintf(os.Stderr, "failed to kill session: %v\n", killErr)
		}
		if cleanupErr != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", cleanupErr)
		}
		return 1
	}
	fmt.Println("ok")
	return 0
}

func cmdSessionKillAll(args []string) int {
	projectOnly := false
	projectRoot := getPWD()
	cleanupAllHashes := false
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
		case "--cleanup-all-hashes":
			cleanupAllHashes = true
		default:
			return unknownFlagError(args[i])
		}
	}

	projectRoot = canonicalProjectRoot(projectRoot)
	allHashes := cleanupAllHashes || !projectOnly
	cleanupOpts := cleanupOptions{
		AllHashes:  allHashes,
		KeepEvents: true,
	}
	sessions, err := tmuxListSessionsFn(projectOnly, projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list sessions: %v\n", err)
		return 1
	}
	var errs []string
	killed := 0
	for _, s := range sessions {
		killErr := tmuxKillSessionFn(s)
		if killErr != nil {
			errs = append(errs, fmt.Sprintf("%s kill: %v", s, killErr))
		} else {
			killed++
		}
		cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, s, cleanupOpts)
		eventState := "terminated"
		eventReason := "kill_all_success"
		if killErr != nil {
			eventState = "degraded"
			eventReason = "kill_all_error"
		}
		if eventErr := appendLifecycleEvent(projectRoot, s, "lifecycle", eventState, "idle", eventReason); eventErr != nil {
			errs = append(errs, fmt.Sprintf("%s observability: %v", s, eventErr))
		}
		if cleanupErr != nil {
			errs = append(errs, fmt.Sprintf("%s cleanup: %v", s, cleanupErr))
		}
	}
	if pruneErr := pruneStaleSessionEventArtifactsFn(); pruneErr != nil {
		errs = append(errs, fmt.Sprintf("event retention cleanup: %v", pruneErr))
	}
	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "killed %d/%d sessions\n", killed, len(sessions))
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		return 1
	}
	fmt.Printf("killed %d sessions\n", killed)
	return 0
}

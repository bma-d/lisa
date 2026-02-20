package app

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func cmdSessionList(args []string) int {
	projectOnly := false
	allSockets := false
	projectRoot := getPWD()

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session list")
		case "--project-only":
			projectOnly = true
		case "--all-sockets":
			allSockets = true
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
	list := []string{}
	var err error
	if allSockets {
		list, err = listSessionsAcrossSockets(projectRoot, projectOnly)
	} else {
		restoreRuntime := withProjectRuntimeEnv(projectRoot)
		defer restoreRuntime()
		list, err = tmuxListSessionsFn(projectOnly, projectRoot)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list sessions: %v\n", err)
		return 1
	}
	fmt.Println(strings.Join(list, "\n"))
	return 0
}

func listSessionsAcrossSockets(projectRoot string, projectOnly bool) ([]string, error) {
	projectRoot = canonicalProjectRoot(projectRoot)
	rootHash := projectHash(projectRoot)

	outSet := map[string]struct{}{}

	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	currentSessions, err := tmuxListSessionsFn(projectOnly, projectRoot)
	restoreRuntime()
	if err != nil {
		return nil, err
	}
	for _, session := range currentSessions {
		outSet[session] = struct{}{}
	}

	metas, err := loadSessionMetasForProject(projectRoot, true)
	if err != nil {
		return nil, err
	}
	for _, meta := range metas {
		session := strings.TrimSpace(meta.Session)
		if session == "" {
			continue
		}
		metaRoot := canonicalProjectRoot(meta.ProjectRoot)
		if metaRoot == "" {
			metaRoot = projectRoot
		}
		if projectOnly && projectHash(metaRoot) != rootHash {
			continue
		}
		restore := withProjectRuntimeEnv(metaRoot)
		active := tmuxHasSessionFn(session)
		restore()
		if active {
			outSet[session] = struct{}{}
		}
	}

	out := make([]string, 0, len(outSet))
	for session := range outSet {
		out = append(out, session)
	}
	sort.Strings(out)
	return out, nil
}

func cmdSessionExists(args []string) int {
	session := ""
	projectRoot := getPWD()
	projectRootExplicit := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session exists")
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
	projectRootExplicit := false
	cleanupAllHashes := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session kill")
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
	projectRoot = resolveSessionProjectRoot(session, projectRoot, projectRootExplicit)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
	cleanupOpts := cleanupOptions{
		AllHashes:  cleanupAllHashes,
		KeepEvents: true,
	}
	descendants, descendantsErr := listSessionDescendants(projectRoot, session, cleanupOpts.AllHashes)
	if descendantsErr != nil {
		fmt.Fprintf(os.Stderr, "descendant lookup warning: %v\n", descendantsErr)
	}

	targets := make([]string, 0, len(descendants)+1)
	targets = append(targets, descendants...)
	targets = append(targets, session)

	rootFound := false
	var errs []string
	for _, target := range targets {
		isRoot := target == session
		reasonPrefix := "kill_descendant"
		if isRoot {
			reasonPrefix = "kill"
		}

		if !tmuxHasSessionFn(target) {
			if isRoot {
				fmt.Fprintln(os.Stderr, "session not found")
			}
			if err := cleanupSessionArtifactsWithOptions(projectRoot, target, cleanupOpts); err != nil {
				errs = append(errs, fmt.Sprintf("%s cleanup: %v", target, err))
			}
			if err := appendLifecycleEvent(projectRoot, target, "lifecycle", "not_found", "idle", reasonPrefix+"_not_found"); err != nil {
				errs = append(errs, fmt.Sprintf("%s observability: %v", target, err))
			}
			continue
		}
		if isRoot {
			rootFound = true
		}

		killErr := tmuxKillSessionFn(target)
		cleanupErr := cleanupSessionArtifactsWithOptions(projectRoot, target, cleanupOpts)
		eventState := "terminated"
		eventReason := reasonPrefix + "_success"
		if killErr != nil {
			eventState = "degraded"
			eventReason = reasonPrefix + "_error"
		}
		if err := appendLifecycleEvent(projectRoot, target, "lifecycle", eventState, "idle", eventReason); err != nil {
			errs = append(errs, fmt.Sprintf("%s observability: %v", target, err))
		}
		if killErr != nil {
			errs = append(errs, fmt.Sprintf("%s kill: %v", target, killErr))
		}
		if cleanupErr != nil {
			errs = append(errs, fmt.Sprintf("%s cleanup: %v", target, cleanupErr))
		}
	}

	if err := pruneStaleSessionEventArtifactsFn(); err != nil {
		fmt.Fprintf(os.Stderr, "observability warning: %v\n", err)
	}

	if !rootFound {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		return 1
	}

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
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
		case "--help", "-h":
			return showHelp("session kill-all")
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
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()
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

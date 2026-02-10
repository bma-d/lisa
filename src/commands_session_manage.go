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
	projectRoot = canonicalProjectRoot(projectRoot)
	if !tmuxHasSessionFn(session) {
		fmt.Fprintln(os.Stderr, "session not found")
		if err := cleanupSessionArtifacts(projectRoot, session); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", err)
		}
		return 1
	}
	if err := tmuxKillSessionFn(session); err != nil {
		fmt.Fprintf(os.Stderr, "failed to kill session: %v\n", err)
		return 1
	}
	if err := cleanupSessionArtifacts(projectRoot, session); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", err)
		return 1
	}
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

	projectRoot = canonicalProjectRoot(projectRoot)
	sessions, err := tmuxListSessionsFn(projectOnly, projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list sessions: %v\n", err)
		return 1
	}
	var errs []string
	killed := 0
	for _, s := range sessions {
		if err := tmuxKillSessionFn(s); err != nil {
			errs = append(errs, fmt.Sprintf("%s kill: %v", s, err))
			continue
		}
		killed++
		if err := cleanupSessionArtifacts(projectRoot, s); err != nil {
			errs = append(errs, fmt.Sprintf("%s cleanup: %v", s, err))
		}
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

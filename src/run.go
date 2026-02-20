package app

import (
	"fmt"
	"os"
	"strings"
)

func Run(args []string) int {
	if len(args) < 1 {
		usage()
		return 1
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "doctor":
		return cmdDoctor(rest)
	case "cleanup":
		return cmdCleanup(rest)
	case "version", "--version", "-version", "-v":
		fmt.Printf("lisa %s (commit %s, built %s)\n", BuildVersion, BuildCommit, BuildDate)
		return 0
	case "session":
		return cmdSession(rest)
	case "agent":
		return cmdAgent(rest)
	case "help", "--help", "-h":
		return showHelp(strings.Join(rest, " "))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		return 1
	}
}

func usage() {
	helpTop()
}

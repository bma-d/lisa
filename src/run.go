package app

import (
	"fmt"
	"os"
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
	case "version", "--version", "-version", "-v":
		fmt.Printf("lisa %s (commit %s, built %s)\n", BuildVersion, BuildCommit, BuildDate)
		return 0
	case "session":
		return cmdSession(rest)
	case "agent":
		return cmdAgent(rest)
	case "help", "--help", "-h":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		return 1
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "lisa <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  doctor")
	fmt.Fprintln(os.Stderr, "  version (also: --version, -version, -v)")
	fmt.Fprintln(os.Stderr, "  session name [--agent claude|codex] [--mode interactive|exec] [--project-root PATH] [--tag TEXT]")
	fmt.Fprintln(os.Stderr, "  session spawn --agent claude|codex --mode interactive|exec [--session lisa-NAME] [--prompt TEXT] [--command TEXT]")
	fmt.Fprintln(os.Stderr, "               [--agent-args TEXT] [--project-root PATH] [--width N] [--height N] [--cleanup-all-hashes] [--json]")
	fmt.Fprintln(os.Stderr, "  session send --session NAME [--project-root PATH] [--text TEXT | --keys \"KEYS...\"] [--enter] [--json]")
	fmt.Fprintln(os.Stderr, "  session status --session NAME [--agent auto|claude|codex] [--mode auto|interactive|exec] [--project-root PATH] [--full] [--json]")
	fmt.Fprintln(os.Stderr, "               --full adds classification/signal columns in text output (prefixed with schema tag)")
	fmt.Fprintln(os.Stderr, "  session explain --session NAME [--agent auto|claude|codex] [--mode auto|interactive|exec] [--project-root PATH] [--events N] [--json]")
	fmt.Fprintln(os.Stderr, "  session monitor --session NAME [--agent auto|claude|codex] [--mode auto|interactive|exec] [--project-root PATH]")
	fmt.Fprintln(os.Stderr, "                  [--poll-interval N] [--max-polls N] [--stop-on-waiting true|false] [--json] [--verbose]")
	fmt.Fprintln(os.Stderr, "  session capture --session NAME [--lines N] [--json]")
	fmt.Fprintln(os.Stderr, "  session list [--project-only] [--project-root PATH]")
	fmt.Fprintln(os.Stderr, "  session exists --session NAME")
	fmt.Fprintln(os.Stderr, "  session kill --session NAME [--project-root PATH] [--cleanup-all-hashes]")
	fmt.Fprintln(os.Stderr, "  session kill-all [--project-only] [--project-root PATH] [--cleanup-all-hashes]")
	fmt.Fprintln(os.Stderr, "  agent build-cmd --agent claude|codex --mode interactive|exec [--prompt TEXT] [--agent-args TEXT] [--json]")
}

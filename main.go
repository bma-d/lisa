package main

import (
	"fmt"
	"os"

	app "github.com/bma-d/lisa/src"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func shouldPrintVersion(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch args[0] {
	case "version", "--version", "-version", "-v":
		return true
	default:
		return false
	}
}

func main() {
	args := os.Args[1:]
	if shouldPrintVersion(args) {
		fmt.Printf("lisa %s (commit %s, built %s)\n", version, commit, date)
		return
	}
	os.Exit(app.Run(args))
}

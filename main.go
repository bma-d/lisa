package main

import (
	"os"

	app "github.com/bma-d/lisa/src"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	app.SetBuildInfo(version, commit, date)
	os.Exit(app.Run(os.Args[1:]))
}

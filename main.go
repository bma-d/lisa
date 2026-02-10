package main

import (
	"os"

	app "lisa/src"
)

func main() {
	os.Exit(app.Run(os.Args[1:]))
}

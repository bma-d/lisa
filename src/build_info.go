package app

import "strings"

var (
	BuildVersion = "dev"
	BuildCommit  = "none"
	BuildDate    = "unknown"
)

func SetBuildInfo(version, commit, date string) {
	if v := strings.TrimSpace(version); v != "" {
		BuildVersion = v
	}
	if c := strings.TrimSpace(commit); c != "" {
		BuildCommit = c
	}
	if d := strings.TrimSpace(date); d != "" {
		BuildDate = d
	}
}

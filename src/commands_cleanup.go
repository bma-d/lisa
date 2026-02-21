package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type tmuxSocketProbe struct {
	Reachable bool
	Sessions  int
	Clients   int
}

type cleanupSummary struct {
	DryRun       bool     `json:"dryRun"`
	Scanned      int      `json:"scanned"`
	Removed      int      `json:"removed"`
	WouldRemove  int      `json:"wouldRemove"`
	Killed       int      `json:"killedServers"`
	WouldKill    int      `json:"wouldKillServers"`
	KeptActive   int      `json:"keptActive"`
	SocketErrors []string `json:"errors,omitempty"`
	ErrorCode    string   `json:"errorCode,omitempty"`
}

var cleanupSocketCandidatesFn = cleanupSocketCandidates
var probeTmuxSocketFn = probeTmuxSocket
var killTmuxSocketServerFn = killTmuxSocketServer
var removeSocketPathFn = removeSocketPath
var listTmuxSocketPathsFromProcessTableFn = listTmuxSocketPathsFromProcessTable

func cmdCleanup(args []string) int {
	includeTmuxDefault := false
	dryRun := false
	jsonOut := hasJSONFlag(args)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("cleanup")
		case "--include-tmux-default":
			includeTmuxDefault = true
		case "--dry-run":
			dryRun = true
		case "--json":
			jsonOut = true
		default:
			return commandErrorf(jsonOut, "unknown_flag", "unknown flag: %s", args[i])
		}
	}

	socketPaths, err := cleanupSocketCandidatesFn(includeTmuxDefault)
	if err != nil {
		return commandErrorf(jsonOut, "cleanup_probe_failed", "cleanup failed: %v", err)
	}

	summary := cleanupSummary{
		DryRun:  dryRun,
		Scanned: len(socketPaths),
	}
	for _, socketPath := range socketPaths {
		probe, err := probeTmuxSocketFn(socketPath)
		if err != nil {
			summary.SocketErrors = append(summary.SocketErrors, fmt.Sprintf("%s probe: %v", socketPath, err))
			continue
		}

		if !probe.Reachable {
			if dryRun {
				summary.WouldRemove++
				continue
			}
			if err := removeSocketPathFn(socketPath); err != nil {
				summary.SocketErrors = append(summary.SocketErrors, fmt.Sprintf("%s remove: %v", socketPath, err))
				continue
			}
			summary.Removed++
			continue
		}

		if probe.Clients > 0 {
			summary.KeptActive++
			continue
		}

		if dryRun {
			summary.WouldKill++
			// Most detached/no-client servers should leave a stale socket after kill.
			summary.WouldRemove++
			continue
		}

		if err := killTmuxSocketServerFn(socketPath); err != nil {
			summary.SocketErrors = append(summary.SocketErrors, fmt.Sprintf("%s kill-server: %v", socketPath, err))
			continue
		}
		summary.Killed++

		postProbe, postErr := probeTmuxSocketFn(socketPath)
		if postErr != nil {
			summary.SocketErrors = append(summary.SocketErrors, fmt.Sprintf("%s post-kill probe: %v", socketPath, postErr))
			continue
		}
		if !postProbe.Reachable {
			if err := removeSocketPathFn(socketPath); err != nil {
				summary.SocketErrors = append(summary.SocketErrors, fmt.Sprintf("%s post-kill remove: %v", socketPath, err))
			} else {
				summary.Removed++
			}
			continue
		}
		summary.KeptActive++
	}

	if len(summary.SocketErrors) > 0 {
		summary.ErrorCode = "cleanup_socket_errors"
		if jsonOut {
			writeJSON(summary)
			return 1
		}
		for _, e := range summary.SocketErrors {
			fmt.Fprintln(os.Stderr, e)
		}
		return 1
	}
	if jsonOut {
		writeJSON(summary)
	} else if dryRun {
		fmt.Printf("cleanup (dry-run): scanned %d sockets, would remove %d stale sockets, would kill %d detached servers, kept %d active\n",
			summary.Scanned, summary.WouldRemove, summary.WouldKill, summary.KeptActive)
	} else {
		fmt.Printf("cleanup: scanned %d sockets, removed %d stale sockets, killed %d detached servers, kept %d active\n",
			summary.Scanned, summary.Removed, summary.Killed, summary.KeptActive)
	}
	return 0
}

func cleanupSocketCandidates(includeTmuxDefault bool) ([]string, error) {
	patterns := []string{
		filepath.Join(preferredTmuxSocketDir(), "lisa-tmux-*-*.sock"),
		filepath.Join(os.TempDir(), "lisa-tmux-*-*.sock"),
		"/tmp/lisa-tmux-*-*.sock",
		"/private/tmp/lisa-tmux-*-*.sock",
		"/tmp/lisa-codex-nosb.sock",
	}
	if includeTmuxDefault {
		patterns = append(patterns,
			"/tmp/tmux-*.sock",
			"/tmp/tmux-*/default",
		)
	}

	paths := make([]string, 0, 32)
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		paths = append(paths, matches...)
	}

	processSocketPaths, err := listTmuxSocketPathsFromProcessTableFn()
	if err == nil {
		for _, p := range processSocketPaths {
			if includeTmuxDefault || isLikelyLisaSocketPath(p) {
				paths = append(paths, p)
			}
		}
	}

	paths = dedupeSocketPaths(paths)
	existing := make([]string, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		existing = append(existing, p)
	}
	sort.Strings(existing)
	return existing, nil
}

func listTmuxSocketPathsFromProcessTable() ([]string, error) {
	psOut, err := runCmd("ps", "axo", "command=")
	if err != nil {
		return nil, err
	}
	return extractTmuxSocketPathsFromPS(psOut), nil
}

func extractTmuxSocketPathsFromPS(psOutput string) []string {
	var out []string
	for _, line := range trimLines(psOutput) {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		execName := strings.ToLower(filepath.Base(fields[0]))
		if execName != "tmux" {
			continue
		}
		for i := 1; i < len(fields)-1; i++ {
			if fields[i] != "-S" {
				continue
			}
			candidate := strings.TrimSpace(fields[i+1])
			if candidate == "" {
				continue
			}
			out = append(out, filepath.Clean(candidate))
			break
		}
	}
	return dedupeSocketPaths(out)
}

func isLikelyLisaSocketPath(path string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(path)))
	if base == "lisa-codex-nosb.sock" {
		return true
	}
	return strings.HasPrefix(base, "lisa-") && strings.HasSuffix(base, ".sock")
}

func probeTmuxSocket(socketPath string) (tmuxSocketProbe, error) {
	sessionOut, sessionErr := runTmuxWithSocket("", socketPath, "list-sessions")
	if sessionErr != nil {
		if isTmuxNoSessionsOutput(sessionOut) || isTmuxNoSessionsOutput(sessionErr.Error()) {
			return tmuxSocketProbe{Reachable: false}, nil
		}
		return tmuxSocketProbe{Reachable: false}, nil
	}

	sessionLines := 0
	for _, line := range trimLines(sessionOut) {
		if strings.TrimSpace(line) != "" {
			sessionLines++
		}
	}

	clientOut, clientErr := runTmuxWithSocket("", socketPath, "list-clients")
	if clientErr != nil {
		if isTmuxNoSessionsOutput(clientOut) || isTmuxNoSessionsOutput(clientErr.Error()) {
			return tmuxSocketProbe{Reachable: false}, nil
		}
		// tmux can return non-zero for empty client list on some versions; treat as detached.
		return tmuxSocketProbe{Reachable: true, Sessions: sessionLines, Clients: 0}, nil
	}

	clientLines := 0
	for _, line := range trimLines(clientOut) {
		if strings.TrimSpace(line) != "" {
			clientLines++
		}
	}
	return tmuxSocketProbe{Reachable: true, Sessions: sessionLines, Clients: clientLines}, nil
}

func killTmuxSocketServer(socketPath string) error {
	out, err := runTmuxWithSocket("", socketPath, "kill-server")
	if err != nil {
		return wrapTmuxCommandError(err, out)
	}
	return nil
}

func removeSocketPath(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

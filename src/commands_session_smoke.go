package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var runLisaSubcommandFn = runLisaSubcommand
var osExecutableFn = os.Executable

type sessionSmokeSummary struct {
	OK             bool               `json:"ok"`
	ProjectRoot    string             `json:"projectRoot"`
	Levels         int                `json:"levels"`
	WorkDir        string             `json:"workDir"`
	KeepSessions   bool               `json:"keepSessions"`
	Sessions       []string           `json:"sessions"`
	Markers        []string           `json:"markers"`
	MissingMarkers []string           `json:"missingMarkers,omitempty"`
	Monitor        monitorResult      `json:"monitor"`
	Tree           *sessionTreeResult `json:"tree,omitempty"`
	Error          string             `json:"error,omitempty"`
	CleanupErrors  []string           `json:"cleanupErrors,omitempty"`
}

func cmdSessionSmoke(args []string) int {
	projectRoot := getPWD()
	levels := 3
	maxPolls := 180
	pollInterval := 1
	keepSessions := false
	jsonOut := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			return showHelp("session smoke")
		case "--project-root":
			if i+1 >= len(args) {
				return flagValueError("--project-root")
			}
			projectRoot = args[i+1]
			i++
		case "--levels":
			if i+1 >= len(args) {
				return flagValueError("--levels")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "invalid --levels")
				return 1
			}
			levels = n
			i++
		case "--max-polls":
			if i+1 >= len(args) {
				return flagValueError("--max-polls")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "invalid --max-polls")
				return 1
			}
			maxPolls = n
			i++
		case "--poll-interval":
			if i+1 >= len(args) {
				return flagValueError("--poll-interval")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "invalid --poll-interval")
				return 1
			}
			pollInterval = n
			i++
		case "--keep-sessions":
			keepSessions = true
		case "--json":
			jsonOut = true
		default:
			return unknownFlagError(args[i])
		}
	}

	if levels > 4 {
		fmt.Fprintln(os.Stderr, "invalid --levels: max supported is 4")
		return 1
	}

	projectRoot = canonicalProjectRoot(projectRoot)
	restoreRuntime := withProjectRuntimeEnv(projectRoot)
	defer restoreRuntime()

	if _, err := exec.LookPath("bash"); err != nil {
		fmt.Fprintf(os.Stderr, "error: required command not found: bash (%v)\n", err)
		return 1
	}

	binPath, err := osExecutableFn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve lisa binary path: %v\n", err)
		return 1
	}
	binPath = strings.TrimSpace(binPath)
	if binPath == "" {
		fmt.Fprintln(os.Stderr, "failed to resolve lisa binary path")
		return 1
	}

	runID := fmt.Sprintf("%s-%d", time.Now().Format("20060102_150405"), os.Getpid())
	workDir := filepath.Join(os.TempDir(), "lisa-smoke-"+runID)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create smoke workdir: %v\n", err)
		return 1
	}

	sessions := make([]string, levels)
	markers := make([]string, levels)
	scripts := make([]string, levels)
	for i := 0; i < levels; i++ {
		level := i + 1
		sessions[i] = fmt.Sprintf("lisa-smoke-l%d-%s", level, runID)
		markers[i] = fmt.Sprintf("LISA_SMOKE_L%d_DONE=1", level)
		scripts[i] = filepath.Join(workDir, fmt.Sprintf("l%d.sh", level))
	}

	summary := sessionSmokeSummary{
		OK:           false,
		ProjectRoot:  projectRoot,
		Levels:       levels,
		WorkDir:      workDir,
		KeepSessions: keepSessions,
		Sessions:     sessions,
		Markers:      markers,
	}

	if !keepSessions {
		defer func() {
			summary.CleanupErrors = append(summary.CleanupErrors, cleanupSmokeSessions(binPath, projectRoot, sessions)...)
		}()
	}

	for idx := levels - 1; idx >= 0; idx-- {
		lines := []string{
			"#!/usr/bin/env bash",
			"set -euo pipefail",
			"BIN=" + shellQuote(binPath),
			"ROOT=" + shellQuote(projectRoot),
		}

		if idx < levels-1 {
			child := sessions[idx+1]
			childScript := scripts[idx+1]
			lines = append(lines,
				fmt.Sprintf(`"$BIN" session spawn --agent codex --mode interactive --project-root "$ROOT" --session %s --command %s --json`,
					shellQuote(child),
					shellQuote("/bin/bash "+childScript),
				),
				fmt.Sprintf(`"$BIN" session monitor --session %s --project-root "$ROOT" --poll-interval %d --max-polls %d --expect terminal --json`,
					shellQuote(child), pollInterval, maxPolls),
				fmt.Sprintf(`"$BIN" session capture --session %s --project-root "$ROOT" --raw --lines %d`,
					shellQuote(child), 120+idx*80),
			)
		}

		lines = append(lines,
			"echo "+markers[idx],
			"echo LISA_SMOKE_SESSION="+sessions[idx],
		)

		body := strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(scripts[idx], []byte(body), 0o700); err != nil {
			return emitSmokeFailure(jsonOut, &summary, fmt.Sprintf("failed to write smoke script %s: %v", scripts[idx], err))
		}
	}

	rootSession := sessions[0]
	if _, err := runLisaSubcommandFn(binPath,
		"session", "spawn",
		"--agent", "codex",
		"--mode", "interactive",
		"--project-root", projectRoot,
		"--session", rootSession,
		"--command", "/bin/bash "+scripts[0],
		"--json",
	); err != nil {
		return emitSmokeFailure(jsonOut, &summary, fmt.Sprintf("failed to spawn L1 smoke session: %v", err))
	}

	monitorOut, err := runLisaSubcommandFn(binPath,
		"session", "monitor",
		"--session", rootSession,
		"--project-root", projectRoot,
		"--poll-interval", strconv.Itoa(pollInterval),
		"--max-polls", strconv.Itoa(maxPolls),
		"--expect", "terminal",
		"--json",
	)
	if err != nil {
		return emitSmokeFailure(jsonOut, &summary, fmt.Sprintf("failed to monitor L1 smoke session: %v", err))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(monitorOut)), &summary.Monitor); err != nil {
		return emitSmokeFailure(jsonOut, &summary, fmt.Sprintf("failed to parse monitor output: %v", err))
	}
	if summary.Monitor.FinalState != "completed" {
		return emitSmokeFailure(jsonOut, &summary, fmt.Sprintf("unexpected smoke final state: %s", summary.Monitor.FinalState))
	}

	captureOut, err := runLisaSubcommandFn(binPath,
		"session", "capture",
		"--session", rootSession,
		"--project-root", projectRoot,
		"--raw",
		"--lines", strconv.Itoa(220+levels*120),
		"--json",
	)
	if err != nil {
		return emitSmokeFailure(jsonOut, &summary, fmt.Sprintf("failed to capture smoke output: %v", err))
	}
	var capturePayload struct {
		Capture string `json:"capture"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(captureOut)), &capturePayload); err != nil {
		return emitSmokeFailure(jsonOut, &summary, fmt.Sprintf("failed to parse capture output: %v", err))
	}
	for _, marker := range markers {
		if !strings.Contains(capturePayload.Capture, marker) {
			summary.MissingMarkers = append(summary.MissingMarkers, marker)
		}
	}
	if len(summary.MissingMarkers) > 0 {
		return emitSmokeFailure(jsonOut, &summary, "smoke marker assertions failed")
	}

	treeOut, err := runLisaSubcommandFn(binPath,
		"session", "tree",
		"--session", rootSession,
		"--project-root", projectRoot,
		"--json",
	)
	if err == nil {
		tree := sessionTreeResult{}
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(treeOut)), &tree); jsonErr == nil {
			summary.Tree = &tree
		}
	}

	summary.OK = true
	if jsonOut {
		writeJSON(summary)
		return 0
	}
	fmt.Printf("PASS: nested smoke %d-level\n", levels)
	for i, session := range sessions {
		fmt.Printf("L%d=%s\n", i+1, session)
	}
	fmt.Printf("Artifacts: %s\n", workDir)
	return 0
}

func emitSmokeFailure(jsonOut bool, summary *sessionSmokeSummary, message string) int {
	summary.OK = false
	summary.Error = message
	if jsonOut {
		writeJSON(summary)
		return 1
	}
	fmt.Fprintln(os.Stderr, message)
	return 1
}

func cleanupSmokeSessions(binPath, projectRoot string, sessions []string) []string {
	errs := []string{}
	for i := len(sessions) - 1; i >= 0; i-- {
		session := sessions[i]
		if _, err := runLisaSubcommandFn(binPath,
			"session", "kill",
			"--session", session,
			"--project-root", projectRoot,
		); err != nil {
			errs = append(errs, fmt.Sprintf("%s cleanup: %v", session, err))
		}
	}
	return errs
}

func runLisaSubcommand(binPath string, args ...string) (string, error) {
	cmd := exec.Command(binPath, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}
